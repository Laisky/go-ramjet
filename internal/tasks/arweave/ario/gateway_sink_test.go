package ario

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/library/egress"
)

// TestGatewayHandlerRefusesSubdomainsResolvingInternally covers the
// go/request-forgery sink in GatewayHandler (alerts #120 and #55): the caller's
// domain selects an ArNS subdomain of a configured gateway, and that name may
// resolve to a loopback, link-local metadata or private address. The handler
// must refuse it by admission and never connect to it. A plain client in place
// of the gateway client, or the server transport in place of egress, connects
// to the stand-in and fails this test.
func TestGatewayHandlerRefusesSubdomainsResolvingInternally(t *testing.T) {
	var dials, hits atomic.Int32
	standIn := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("X-Ar-Io-Hops", "1")
		_, _ = w.Write([]byte("internal-secret"))
	}))
	t.Cleanup(standIn.Close)
	// Any client that skips admission is routed to the stand-in and counted.
	routed := &http.Transport{
		Proxy:           nil,
		TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: true}, //nolint:gosec // test stand-in only
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
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

	for name, address := range map[string]string{
		"loopback":            "127.0.0.1",
		"link-local metadata": "169.254.169.254",
		"private DNS":         "10.0.0.7",
	} {
		t.Run(name, func(t *testing.T) {
			dials.Store(0)
			hits.Store(0)
			useGatewayEgress(t, egress.PinnedTransport{Deps: egress.Dependencies{
				Resolve: func(_ context.Context, host string) ([]net.IPAddr, error) {
					if host != "evil.gateway.example" {
						return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
					}
					return []net.IPAddr{{IP: net.ParseIP(address)}}, nil
				},
				Dial: routed.DialContext,
			}})

			recorder := serveGateway(httptest.NewRequest(http.MethodGet,
				"/gateway/"+testFileKey+"?domain="+url.QueryEscape("evil"), nil))
			require.Zero(t, dials.Load(), "the internal target must not be contacted")
			require.Zero(t, hits.Load())
			require.Equal(t, http.StatusBadRequest, recorder.Code)
			require.Contains(t, recorder.Body.String(), string(egress.ReasonDestinationDenied))
			require.NotContains(t, recorder.Body.String(), "internal-secret")

			// The same client the handler builds reports the refusal as a typed error.
			target, err := gatewayTarget("https://gateway.example/", "evil", testFileKey)
			require.NoError(t, err)
			response, err := newGatewayClient(httpcli, ArweaveGateways, gatewayEgress).Get(target)
			if response != nil {
				require.NoError(t, response.Body.Close())
			}
			reason, ok := egress.ReasonOf(err)
			require.True(t, ok, "expected an admission refusal, got: %v", err)
			require.Equal(t, egress.ReasonDestinationDenied, reason)
			require.Zero(t, dials.Load())
		})
	}
}
