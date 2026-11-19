package pieverse

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	gconfig "github.com/Laisky/go-config/v2"
	"github.com/stretchr/testify/require"
)

const productionHost = "blacknova-enclave-cipherforge-wallet-relay.pieverse.io"

// newMasterTLSServer starts a TLS server, signed by httptest's private CA,
// that always answers as a healthy master.
//
// Parameters:
//   - t: test handle; the server is closed on cleanup.
//
// Returns:
//   - *httptest.Server: the running TLS server.
func newMasterTLSServer(t *testing.T) *httptest.Server {
	t.Helper()

	srv := httptest.NewTLSServer(masterHandler(t))
	t.Cleanup(srv.Close)
	return srv
}

// masterHandler answers every request as a healthy master.
//
// Parameters:
//   - t: test handle used to report write failures.
//
// Returns:
//   - http.Handler: the handler.
func masterHandler(t *testing.T) http.Handler {
	t.Helper()

	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, err := w.Write([]byte(`{"state":"master"}`)); err != nil {
			t.Errorf("write health body: %v", err)
		}
	})
}

// newNamedTLSServer starts a TLS server whose certificate, issued by a fresh
// private CA, lists only dnsName (no IP address).
//
// Parameters:
//   - t: test handle; the server is closed on cleanup.
//   - dnsName: the only subject alternative name of the leaf.
//
// Returns:
//   - *httptest.Server: the running TLS server (URL uses 127.0.0.1).
//   - string: path of a PEM file holding the issuing CA.
func newNamedTLSServer(t *testing.T, dnsName string) (*httptest.Server, string) {
	t.Helper()

	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	now := time.Now().UTC()
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "pieverse-test-root"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	require.NoError(t, err)
	caCert, err := x509.ParseCertificate(caDER)
	require.NoError(t, err)

	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	leafTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: dnsName},
		DNSNames:     []string{dnsName},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTmpl, caCert, &leafKey.PublicKey, caKey)
	require.NoError(t, err)

	srv := httptest.NewUnstartedServer(masterHandler(t))
	srv.TLS = &tls.Config{
		Certificates: []tls.Certificate{{Certificate: [][]byte{leafDER}, PrivateKey: leafKey}},
		MinVersion:   tls.VersionTLS12,
	}
	srv.StartTLS()
	t.Cleanup(srv.Close)

	caFile := filepath.Join(t.TempDir(), "ca.pem")
	require.NoError(t, os.WriteFile(caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}), 0o600))
	return srv, caFile
}

// TestServerNameOverridesVerifiedHostname verifies tasks.pieverse_alert.server_name
// lets a url with an IP literal, or with a host the leaf does not list, verify
// against the name the certificate does carry, without weakening any other
// check.
func TestServerNameOverridesVerifiedHostname(t *testing.T) {
	t.Parallel()

	const certName = "health.internal"
	srv, caFile := newNamedTLSServer(t, certName)
	ipURL := srv.URL
	hostURL := strings.Replace(srv.URL, "127.0.0.1", "localhost", 1)

	for _, target := range []struct{ name, url string }{
		{name: "ip literal", url: ipURL},
		{name: "host not in certificate", url: hostURL},
	} {
		t.Run(target.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			var hostnameErr x509.HostnameError
			var unknownAuthority x509.UnknownAuthorityError

			err := newHealthChecker(caFile, "").check(ctx, target.url)
			require.ErrorAs(t, err, &hostnameErr, "without server_name the URL host must be verified")

			require.NoError(t, newHealthChecker(caFile, certName).check(ctx, target.url))

			err = newHealthChecker(caFile, "other.internal").check(ctx, target.url)
			require.ErrorAs(t, err, &hostnameErr, "a wrong server_name must still fail")

			err = newHealthChecker("", certName).check(ctx, target.url)
			require.ErrorAs(t, err, &unknownAuthority, "server_name must not bypass chain verification")
		})
	}
}

// TestBindTaskReportsBadRootCAFileEveryRun verifies an unusable root_ca_file
// keeps the monitor scheduled and fails closed on every run, so the operator
// sees an ERROR each interval instead of a monitor that silently never runs.
func TestBindTaskReportsBadRootCAFileEveryRun(t *testing.T) {
	const caKey = "tasks.pieverse_alert.root_ca_file"
	t.Setenv("TASKS", taskName)
	gconfig.Shared.Set(caKey, filepath.Join(t.TempDir(), "missing.pem"))
	t.Cleanup(func() { gconfig.Shared.Set(caKey, "") })

	var scheduled []*healthChecker
	origSchedule := scheduleHealthCheck
	scheduleHealthCheck = func(_ time.Duration, checker *healthChecker) {
		scheduled = append(scheduled, checker)
	}
	t.Cleanup(func() { scheduleHealthCheck = origSchedule })

	// A healthy endpoint proves the failure comes from the configuration.
	srv := httptest.NewServer(masterHandler(t))
	t.Cleanup(srv.Close)

	bindTask()
	require.Len(t, scheduled, 1, "a bad root_ca_file must not silently disable the monitor")
	checker := scheduled[0]
	for range 3 {
		// Fails closed: a plain-HTTP healthy endpoint is never reached.
		err := checker.check(context.Background(), srv.URL)
		require.ErrorContains(t, err, "read root ca file")
	}
}

// TestCheckHealthRejectsUntrustedCertificate reproduces CodeQL
// go/disabled-certificate-check #105: with InsecureSkipVerify any TLS
// endpoint, including a man-in-the-middle, was reported healthy. A certificate
// that does not chain to a trusted root must now fail the check.
func TestCheckHealthRejectsUntrustedCertificate(t *testing.T) {
	t.Parallel()

	srv := newMasterTLSServer(t)
	checker := newHealthChecker("", "")

	err := checker.check(context.Background(), srv.URL)
	require.Error(t, err, "an impostor TLS endpoint must not be reported healthy")
	var unknownAuthority x509.UnknownAuthorityError
	require.ErrorAs(t, err, &unknownAuthority)
}

// TestCheckHealthTrustsConfiguredRootCAFile verifies root_ca_file adds trust
// anchors, so an operator can trust another private CA without disabling
// verification.
func TestCheckHealthTrustsConfiguredRootCAFile(t *testing.T) {
	t.Parallel()

	srv := newMasterTLSServer(t)
	caFile := filepath.Join(t.TempDir(), "roots.pem")
	require.NoError(t, os.WriteFile(caFile, pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: srv.Certificate().Raw,
	}), 0o600))

	checker := newHealthChecker(caFile, "")
	require.NoError(t, checker.check(context.Background(), srv.URL))
}

// TestNewHTTPClientRejectsBadRootCAFile verifies a misconfigured root_ca_file
// is reported instead of being silently ignored.
func TestNewHTTPClientRejectsBadRootCAFile(t *testing.T) {
	t.Parallel()

	_, err := newHTTPClient(filepath.Join(t.TempDir(), "missing.pem"), "")
	require.Error(t, err)

	garbage := filepath.Join(t.TempDir(), "garbage.pem")
	require.NoError(t, os.WriteFile(garbage, []byte("not a certificate"), 0o600))
	_, err = newHTTPClient(garbage, "")
	require.ErrorContains(t, err, "no PEM certificate")
}

// TestEmbeddedRootVerifiesProductionChain proves the default endpoint keeps
// working with verification on: the leaf and issuing CA it served on
// 2026-11-19 chain to the embedded purrfect-tee Root CA for its hostname.
func TestEmbeddedRootVerifiesProductionChain(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile(filepath.Join("testdata", "served_chain_20261119.pem"))
	require.NoError(t, err)

	var certs []*x509.Certificate
	for block, rest := pem.Decode(raw); block != nil; block, rest = pem.Decode(rest) {
		cert, err := x509.ParseCertificate(block.Bytes)
		require.NoError(t, err)
		certs = append(certs, cert)
	}
	require.Len(t, certs, 2, "fixture holds leaf + issuing CA")

	// The production pool: system roots plus the embedded purrfect-tee Root.
	roots, err := newRootPool("")
	require.NoError(t, err)
	intermediates := x509.NewCertPool()
	intermediates.AddCert(certs[1])

	opts := x509.VerifyOptions{
		DNSName:       productionHost,
		Roots:         roots,
		Intermediates: intermediates,
		CurrentTime:   time.Date(2026, 11, 19, 6, 0, 0, 0, time.UTC),
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	chains, err := certs[0].Verify(opts)
	require.NoError(t, err)
	require.NotEmpty(t, chains)
	require.Equal(t, "purrfect-tee-root-ca", chains[0][len(chains[0])-1].Subject.CommonName)

	opts.DNSName = "impostor.example.com"
	_, err = certs[0].Verify(opts)
	require.Error(t, err, "hostname verification must stay on")
}

// TestCheckReverifiesTLSEachRun verifies every health check performs its own
// TLS handshake. With a kept-alive connection only the first run verified the
// certificate, so once the endpoint started serving an untrusted or expired
// leaf to new clients (its leaves live about 24h) the monitor kept reporting
// healthy over the old connection.
func TestCheckReverifiesTLSEachRun(t *testing.T) {
	t.Parallel()

	untrustedKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	now := time.Now().UTC()
	untrustedTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1)},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(time.Hour),
	}
	untrustedDER, err := x509.CreateCertificate(rand.Reader, untrustedTmpl, untrustedTmpl, &untrustedKey.PublicKey, untrustedKey)
	require.NoError(t, err)

	var rotated atomic.Bool
	srv := httptest.NewUnstartedServer(masterHandler(t))
	srv.TLS = &tls.Config{
		// Called for every handshake, with or without SNI.
		GetConfigForClient: func(*tls.ClientHelloInfo) (*tls.Config, error) {
			if rotated.Load() {
				return &tls.Config{
					Certificates: []tls.Certificate{{Certificate: [][]byte{untrustedDER}, PrivateKey: untrustedKey}},
					MinVersion:   tls.VersionTLS12,
				}, nil
			}
			return nil, nil // keep httptest's certificate, trusted below
		},
		MinVersion: tls.VersionTLS12,
	}
	srv.StartTLS()
	t.Cleanup(srv.Close)

	caFile := filepath.Join(t.TempDir(), "ca.pem")
	require.NoError(t, os.WriteFile(caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600))

	checker := newHealthChecker(caFile, "")
	require.NoError(t, checker.check(context.Background(), srv.URL))

	rotated.Store(true) // new handshakes now get an untrusted certificate
	var unknownAuthority x509.UnknownAuthorityError
	require.ErrorAs(t, checker.check(context.Background(), srv.URL), &unknownAuthority,
		"the next run must re-verify the certificate, not reuse a verified connection")
}

// TestCheckRecoversAfterRootCAFileIsFixed verifies root_ca_file is read on
// every run, so fixing or rotating the bundle takes effect at the next run
// (as settings-example.yml promises) instead of only after a restart; until
// then each run still fails closed.
func TestCheckRecoversAfterRootCAFileIsFixed(t *testing.T) {
	t.Parallel()

	srv := newMasterTLSServer(t)
	caFile := filepath.Join(t.TempDir(), "roots.pem") // not mounted yet
	checker := newHealthChecker(caFile, "")

	for range 2 {
		require.ErrorContains(t, checker.check(context.Background(), srv.URL), "read root ca file")
	}

	require.NoError(t, os.WriteFile(caFile, pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: srv.Certificate().Raw,
	}), 0o600))
	require.NoError(t, checker.check(context.Background(), srv.URL), "a fixed root_ca_file must apply without a restart")

	require.NoError(t, os.WriteFile(caFile, []byte("not a certificate"), 0o600))
	require.ErrorContains(t, checker.check(context.Background(), srv.URL), "no PEM certificate",
		"a broken bundle must fail closed again, never fall back to an unverified client")
}

// purrfectTEERootCASHA256 is the SHA-256 of the purrfect-tee Root CA
// certificate (DER) in the production Root pin published by
// purrfect-claw-platform (infra/tee-agent-shared/kms-pins.prod.json).
const purrfectTEERootCASHA256 = "d4e928312d8b6e4b1a0be8995a0ffab5b24b2adb0597d90be890e32796c1f074"

// TestEmbeddedRootCAIsPinned freezes the embedded trust anchor: exactly one
// self-signed CA certificate whose SHA-256 equals the production Root pin.
// Without it, replacing or appending a certificate to the PEM (which would
// silently widen what the monitor trusts) would still pass the chain test.
func TestEmbeddedRootCAIsPinned(t *testing.T) {
	t.Parallel()

	block, rest := pem.Decode(purrfectTEERootCAPEM)
	require.NotNil(t, block)
	require.Equal(t, "CERTIFICATE", block.Type)
	require.Empty(t, strings.TrimSpace(string(rest)), "the embedded PEM must hold exactly one certificate")

	sum := sha256.Sum256(block.Bytes)
	require.Equal(t, purrfectTEERootCASHA256, hex.EncodeToString(sum[:]))

	root, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)
	require.Equal(t, "purrfect-tee-root-ca", root.Subject.CommonName)
	require.True(t, root.IsCA)
	require.NotZero(t, root.KeyUsage&x509.KeyUsageCertSign)
	require.NoError(t, root.CheckSignatureFrom(root), "the trust anchor must be self-signed")
	require.Equal(t, time.Date(2036, time.May, 19, 19, 16, 22, 0, time.UTC), root.NotAfter.UTC())
}

// TestCheckLeavesNoOpenConnections verifies the per-run client never keeps a
// connection alive: after N runs the server must see no open connection. With
// keep-alives on, every run's fresh client would leave one idle connection
// (and its goroutines) behind until the server times it out.
func TestCheckLeavesNoOpenConnections(t *testing.T) {
	t.Parallel()

	var open atomic.Int64
	srv := httptest.NewUnstartedServer(masterHandler(t))
	srv.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		switch state {
		case http.StateNew:
			open.Add(1)
		case http.StateClosed, http.StateHijacked:
			open.Add(-1)
		default:
		}
	}
	srv.StartTLS()
	t.Cleanup(srv.Close)

	caFile := filepath.Join(t.TempDir(), "ca.pem")
	require.NoError(t, os.WriteFile(caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600))
	checker := newHealthChecker(caFile, "")

	const runs = 20
	for range runs {
		require.NoError(t, checker.check(context.Background(), srv.URL))
	}
	require.Eventually(t, func() bool { return open.Load() == 0 }, 5*time.Second, 10*time.Millisecond,
		"every run must close its connection")
}

// TestNewRootPoolRejectsDamagedBundle verifies a root_ca_file that is only
// partly usable fails closed. root_ca_file is re-read on every run, so an
// in-place edit can be read half-written; AppendCertsFromPEM accepted such a
// bundle as long as one certificate parsed, silently dropping the rest.
func TestNewRootPoolRejectsDamagedBundle(t *testing.T) {
	t.Parallel()

	first := newMasterTLSServer(t).Certificate()
	second := newMasterTLSServer(t).Certificate()
	good := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: first.Raw})
	full := append(append([]byte{}, good...), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: second.Raw})...)

	for _, tc := range []struct {
		name string
		data []byte
		want string
	}{
		{name: "truncated last block", data: full[:len(good)+len(full[len(good):])/2], want: "incomplete PEM block"},
		{name: "damaged middle block", data: append(append([]byte{}, full[:len(good)-30]...), full[len(good):]...),
			want: "incomplete PEM block"},
		{name: "unparsable certificate", data: append(append([]byte{}, good...),
			pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("not DER")})...), want: "parse certificate 2"},
		{name: "empty file", data: nil, want: "no PEM certificate"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			caFile := filepath.Join(t.TempDir(), "roots.pem")
			require.NoError(t, os.WriteFile(caFile, tc.data, 0o600))
			_, err := newRootPool(caFile)
			require.ErrorContains(t, err, tc.want)
		})
	}

	// A complete bundle, with comment lines and a non-certificate block like
	// those in system CA bundles, still loads.
	caFile := filepath.Join(t.TempDir(), "roots.pem")
	bundle := append([]byte("# old root\n"), full...)
	bundle = append(bundle, pem.EncodeToMemory(&pem.Block{Type: "X509 CRL", Bytes: []byte("ignored")})...)
	require.NoError(t, os.WriteFile(caFile, bundle, 0o600))
	_, err := newRootPool(caFile)
	require.NoError(t, err)
}

// TestCheckExplainsUntrustedChain verifies a chain that does not lead to a
// trusted root, which is what a purrfect-tee Root CA rotation looks like until
// the new root is configured, still fails closed but names the remedy. The
// raw x509 error only said "certificate signed by unknown authority", which
// does not tell the operator that root_ca_file exists or that the new root
// must be checked against the published pin before it is trusted.
func TestCheckExplainsUntrustedChain(t *testing.T) {
	t.Parallel()

	srv := newMasterTLSServer(t) // signed by a CA the checker does not trust
	err := newHealthChecker("", "").check(context.Background(), srv.URL)

	var unknownAuthority x509.UnknownAuthorityError
	require.ErrorAs(t, err, &unknownAuthority, "the x509 cause must stay inspectable")
	require.ErrorContains(t, err, "tasks.pieverse_alert.root_ca_file")
	require.ErrorContains(t, err, "pin")
}
