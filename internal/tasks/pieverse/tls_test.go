package pieverse

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

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

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"state":"master"}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestCheckHealthRejectsUntrustedCertificate reproduces CodeQL
// go/disabled-certificate-check #105: with InsecureSkipVerify any TLS
// endpoint, including a man-in-the-middle, was reported healthy. A certificate
// that does not chain to a trusted root must now fail the check.
func TestCheckHealthRejectsUntrustedCertificate(t *testing.T) {
	t.Parallel()

	srv := newMasterTLSServer(t)
	client, err := newHTTPClient("")
	require.NoError(t, err)

	err = checkHealth(context.Background(), client, srv.URL)
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

	client, err := newHTTPClient(caFile)
	require.NoError(t, err)
	require.NoError(t, checkHealth(context.Background(), client, srv.URL))
}

// TestNewHTTPClientRejectsBadRootCAFile verifies a misconfigured root_ca_file
// is reported instead of being silently ignored.
func TestNewHTTPClientRejectsBadRootCAFile(t *testing.T) {
	t.Parallel()

	_, err := newHTTPClient(filepath.Join(t.TempDir(), "missing.pem"))
	require.Error(t, err)

	garbage := filepath.Join(t.TempDir(), "garbage.pem")
	require.NoError(t, os.WriteFile(garbage, []byte("not a certificate"), 0o600))
	_, err = newHTTPClient(garbage)
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
