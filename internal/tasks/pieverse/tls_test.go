package pieverse

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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
	require.Nil(t, checker.client, "must fail closed, never fall back to an unverified client")
	for range 3 {
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
	require.NoError(t, checker.configErr)

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
	require.NoError(t, checker.configErr)
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

	roots := x509.NewCertPool()
	require.True(t, roots.AppendCertsFromPEM(purrfectTEERootCAPEM))
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
