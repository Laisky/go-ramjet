package ario

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/library/egress"
)

// redirectingSubdomain is the ArNS label whose gateway subdomain resolves to a
// public address and redirects to the link-local metadata address.
const redirectingSubdomain = "hop"

// redirectingSubdomainAddress is the public address the test resolver answers
// for the redirecting subdomain.
const redirectingSubdomainAddress = "93.184.216.34"

// selfSignedCertificate returns a certificate for dnsNames that is its own
// root, and a pool trusting it, so pinned TLS hops verify against a loopback
// fixture without disabling verification.
func selfSignedCertificate(t *testing.T, dnsNames ...string) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	now := time.Now().UTC()
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: dnsNames[0]},
		DNSNames:              dnsNames,
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	leaf, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	roots := x509.NewCertPool()
	roots.AddCert(leaf)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}, roots
}

// pinnedPeerConn reports the pinned public peer for a connection that a test
// routed to a loopback fixture, as egress checks the peer it reached.
type pinnedPeerConn struct {
	net.Conn
	peer net.Addr
}

// RemoteAddr returns the pinned public peer instead of the fixture's loopback address.
func (c *pinnedPeerConn) RemoteAddr() net.Addr { return c.peer }

// TestGatewayHandlerRefusesSubdomainsResolvingInternally covers the
// go/request-forgery sink in GatewayHandler (alerts #120 and #55): the caller's
// domain selects an ArNS subdomain of a configured gateway, and that name may
// resolve to a loopback, link-local metadata or private address, or resolve
// publicly and redirect to the metadata address. The handler must refuse each
// by admission and never connect to the internal target. A plain client in
// place of the gateway client, the server transport in place of egress, or a
// transport that admits only the first hop connects to the stand-in and fails
// this test.
func TestGatewayHandlerRefusesSubdomainsResolvingInternally(t *testing.T) {
	var dials, hits, hopHits atomic.Int32
	standIn := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("X-Ar-Io-Hops", "1")
		_, _ = w.Write([]byte("internal-secret"))
	}))
	t.Cleanup(standIn.Close)
	hopHost := redirectingSubdomain + ".gateway.example"
	certificate, roots := selfSignedCertificate(t, hopHost)
	hop := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hopHits.Add(1)
		http.Redirect(w, r, "https://169.254.169.254/latest/meta-data/", http.StatusFound)
	}))
	hop.TLS = &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{certificate}}
	hop.StartTLS()
	t.Cleanup(hop.Close)
	dialHop := func(ctx context.Context, network string) (net.Conn, error) {
		var dialer net.Dialer
		conn, err := dialer.DialContext(ctx, network, hop.Listener.Addr().String())
		if err != nil {
			return nil, errors.WithStack(err)
		}
		return conn, nil
	}
	// Any client that skips admission reaches the redirecting subdomain's
	// server for its name and the stand-in, counted, for everything else.
	routed := &http.Transport{
		Proxy:           nil,
		TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: true}, //nolint:gosec // test stand-in only
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			if address == net.JoinHostPort(hopHost, "443") {
				return dialHop(ctx, network)
			}
			dials.Add(1)
			var dialer net.Dialer
			conn, err := dialer.DialContext(ctx, network, standIn.Listener.Addr().String())
			if err != nil {
				return nil, errors.WithStack(err)
			}
			return conn, nil
		},
	}
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	http.DefaultTransport = routed
	useGateways(t, &http.Client{Transport: routed}, "https://gateway.example/")

	for _, tc := range []struct {
		name, domain, address string
		redirect              bool
	}{
		{name: "loopback", domain: "evil", address: "127.0.0.1"},
		{name: "link-local metadata", domain: "evil", address: "169.254.169.254"},
		{name: "private DNS", domain: "evil", address: "10.0.0.7"},
		{name: "redirect to metadata", domain: redirectingSubdomain, address: redirectingSubdomainAddress, redirect: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dials.Store(0)
			hits.Store(0)
			hopHits.Store(0)
			useGatewayEgress(t, egress.PinnedTransport{Deps: egress.Dependencies{
				Resolve: func(_ context.Context, host string) ([]net.IPAddr, error) {
					if host != tc.domain+".gateway.example" {
						return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
					}
					return []net.IPAddr{{IP: net.ParseIP(tc.address)}}, nil
				},
				Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
					if tc.redirect && address == net.JoinHostPort(redirectingSubdomainAddress, "443") {
						conn, err := dialHop(ctx, network)
						if err != nil {
							return nil, err
						}
						return &pinnedPeerConn{Conn: conn, peer: &net.TCPAddr{IP: net.ParseIP(redirectingSubdomainAddress), Port: 443}}, nil
					}
					return routed.DialContext(ctx, network, address)
				},
				RootCAs: roots,
			}})

			recorder := serveGateway(httptest.NewRequest(http.MethodGet,
				"/gateway/"+testFileKey+"?domain="+url.QueryEscape(tc.domain), nil))
			require.Zero(t, dials.Load(), "the internal target must not be contacted")
			require.Zero(t, hits.Load())
			require.Equal(t, http.StatusBadRequest, recorder.Code)
			require.Contains(t, recorder.Body.String(), string(egress.ReasonDestinationDenied))
			require.NotContains(t, recorder.Body.String(), "internal-secret")

			// The same client the handler builds reports the refusal as a typed error.
			target, err := gatewayTarget("https://gateway.example/", tc.domain, testFileKey)
			require.NoError(t, err)
			response, err := newGatewayClient(httpcli, ArweaveGateways, gatewayEgress).Get(target)
			if response != nil {
				require.NoError(t, response.Body.Close())
			}
			reason, ok := egress.ReasonOf(err)
			require.True(t, ok, "expected an admission refusal, got: %v", err)
			require.Equal(t, egress.ReasonDestinationDenied, reason)
			require.Zero(t, dials.Load())
			if tc.redirect {
				require.EqualValues(t, 2, hopHits.Load(), "the public subdomain must be admitted and contacted")
			}
		})
	}
}
