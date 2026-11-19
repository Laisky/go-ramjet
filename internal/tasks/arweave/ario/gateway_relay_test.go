package ario

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/library/web"
)

// loopbackForwarder answers requests for host itself through respond and sends
// every other request to its loopback destination, refusing anything else, so
// the server client behaves like a normal client without touching the network.
func loopbackForwarder(host string, respond func(*http.Request) *http.Response) http.RoundTripper {
	forward := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			hostPart, _, err := net.SplitHostPort(address)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			if ip := net.ParseIP(hostPart); ip == nil || !ip.IsLoopback() {
				return nil, errors.Errorf("unexpected dial to %s", address)
			}
			var dialer net.Dialer
			conn, err := dialer.DialContext(ctx, network, address)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			return conn, nil
		},
	}
	return gatewayRoundTripper(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host == host {
			return respond(request), nil
		}
		return forward.RoundTrip(request)
	})
}

// gatewayRedirect answers request with a 302 to location.
func gatewayRedirect(request *http.Request, location string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusFound,
		Header:     http.Header{"Location": []string{location}},
		Body:       http.NoBody,
		Request:    request,
	}
}

// TestGatewayHandlerRefusesRedirectIntoInternalNetwork proves a configured
// gateway cannot bounce this server to an internal address: every hop that
// leaves the configured gateway hosts must pass the public egress policy.
func TestGatewayHandlerRefusesRedirectIntoInternalNetwork(t *testing.T) {
	var internalHits atomic.Int32
	internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		internalHits.Add(1)
		w.Header().Set("X-Ar-Io-Hops", "1")
		_, _ = io.WriteString(w, "internal-secret")
	}))
	t.Cleanup(internal.Close)
	useGateways(t, &http.Client{Transport: loopbackForwarder("gateway.example", func(request *http.Request) *http.Response {
		return gatewayRedirect(request, internal.URL+"/latest/meta-data/")
	})}, "https://gateway.example/")

	recorder := serveGateway(httptest.NewRequest(http.MethodGet, "/gateway/"+testFileKey, nil))
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.NotContains(t, recorder.Body.String(), "internal-secret")
	require.Zero(t, internalHits.Load(), "a gateway redirect must not reach an internal address")
}

// TestGatewayHandlerTrustsCacheOnlyOnFinalURL accepts a response without
// X-Ar-Io-Hops only when the final hop, not the first, is the operator cache.
func TestGatewayHandlerTrustsCacheOnlyOnFinalURL(t *testing.T) {
	const cache = "https://s3.laisky.com/public/arweave/uploadcache/"
	useGateways(t, &http.Client{Transport: gatewayRoundTripper(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host == "s3.laisky.com" {
			return gatewayRedirect(request, "https://gateway.example/elsewhere"), nil
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Request: request,
			Body: io.NopCloser(strings.NewReader("unverified"))}, nil
	})}, cache, "https://gateway.example/")

	recorder := serveGateway(httptest.NewRequest(http.MethodGet, "/gateway/"+testFileKey, nil))
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.NotContains(t, recorder.Body.String(), "unverified")
}

// TestGatewayHandlerServesOperatorCacheDirectly keeps a direct answer from the
// operator cache trusted without an ar.io hop header.
func TestGatewayHandlerServesOperatorCacheDirectly(t *testing.T) {
	useGateways(t, &http.Client{Transport: gatewayRoundTripper(func(request *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Request: request,
			Body: io.NopCloser(strings.NewReader("cached"))}, nil
	})}, "https://s3.laisky.com/public/arweave/uploadcache/")

	recorder := serveGateway(httptest.NewRequest(http.MethodGet, "/gateway/"+testFileKey, nil))
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "cached", recorder.Body.String())
}

// TestGatewayHandlerKeepsPrivateConfiguredGateway leaves operator-chosen
// gateway hosts on the server client, even when they resolve privately.
func TestGatewayHandlerKeepsPrivateConfiguredGateway(t *testing.T) {
	var hits, dials atomic.Int32
	private := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("X-Ar-Io-Hops", "1")
		_, _ = io.WriteString(w, "private-gateway")
	}))
	t.Cleanup(private.Close)
	useGateways(t, loopbackOnlyClient(t, private, &dials), private.URL+"/")

	recorder := serveGateway(httptest.NewRequest(http.MethodGet, "/gateway/"+testFileKey, nil))
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "private-gateway", recorder.Body.String())
	require.EqualValues(t, 1, hits.Load())
}

// TestGatewayHandlerFollowsPublicRedirectsThroughEgress keeps redirects that
// leave the configured gateway hosts working when the destination is public,
// such as ar.io sandbox subdomains, by sending those hops through egress.
func TestGatewayHandlerFollowsPublicRedirectsThroughEgress(t *testing.T) {
	var egressHosts []string
	useGateways(t, &http.Client{Transport: gatewayRoundTripper(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host != "arweave.example" {
			return nil, errors.Errorf("only the configured host may use the server client, got %s", request.URL.Host)
		}
		return gatewayRedirect(request, "https://sandbox.arweave.example/"+testFileKey), nil
	})}, "https://arweave.example/")
	useGatewayEgress(t, gatewayRoundTripper(func(request *http.Request) (*http.Response, error) {
		egressHosts = append(egressHosts, request.URL.Host)
		return gatewayOK(request), nil
	}))

	recorder := serveGateway(httptest.NewRequest(http.MethodGet, "/gateway/"+testFileKey, nil))
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "content", recorder.Body.String())
	require.Equal(t, []string{"sandbox.arweave.example"}, egressHosts)
	require.Equal(t, "https://sandbox.arweave.example/"+testFileKey, recorder.Header().Get("X-Ar-Io-Url"))
}

// TestGatewayHandlerPolicyFollowsContentType sandboxes active and unknown
// types, keeps PDFs viewable, and detects a missing type from the content.
func TestGatewayHandlerPolicyFollowsContentType(t *testing.T) {
	for _, tc := range []struct {
		name, upstreamType, body, wantType, wantPolicy string
	}{
		{name: "html", upstreamType: "text/html", body: "<html><script>x()</script></html>", wantType: "text/html", wantPolicy: web.UntrustedContentPolicy},
		{name: "pdf", upstreamType: "application/pdf", body: "%PDF-1.7\n", wantType: "application/pdf", wantPolicy: web.PassiveContentPolicy},
		{name: "image", upstreamType: "image/png", body: "\x89PNG\r\n\x1a\n", wantType: "image/png", wantPolicy: web.UntrustedContentPolicy},
		{name: "missing_html", body: "<html><script>x()</script></html>", wantType: "text/html; charset=utf-8", wantPolicy: web.UntrustedContentPolicy},
		{name: "missing_pdf", body: "%PDF-1.7\n", wantType: "application/pdf", wantPolicy: web.PassiveContentPolicy},
		{name: "missing_unknown", body: "\x00\x01binary", wantType: "application/octet-stream", wantPolicy: web.UntrustedContentPolicy},
	} {
		t.Run(tc.name, func(t *testing.T) {
			useGateways(t, &http.Client{Transport: gatewayRoundTripper(func(request *http.Request) (*http.Response, error) {
				header := http.Header{"X-Ar-Io-Hops": []string{"1"}}
				if tc.upstreamType != "" {
					header.Set("Content-Type", tc.upstreamType)
				}
				return &http.Response{StatusCode: http.StatusOK, Header: header, Request: request,
					Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})}, "https://gateway.example/")

			recorder := serveGateway(httptest.NewRequest(http.MethodGet, "/gateway/"+testFileKey, nil))
			require.Equal(t, http.StatusOK, recorder.Code)
			require.Equal(t, tc.body, recorder.Body.String())
			require.Equal(t, tc.wantType, recorder.Header().Get("Content-Type"))
			require.Equal(t, []string{tc.wantPolicy}, recorder.Header().Values("Content-Security-Policy"))
			require.Equal(t, "nosniff", recorder.Header().Get("X-Content-Type-Options"))
		})
	}
}

// TestGatewayHandlerRelaysNotModifiedValidatorsOnly answers a gateway 304 with
// the validators and caching headers a cache needs (RFC 9110, section
// 15.4.5), including Vary, and with no representation metadata or policy: a
// Content-Type or Content-Security-Policy on a 304 replaces the stored one, so
// a cached PDF would be re-labeled and re-sandboxed, or a cached image
// re-labeled as HTML, without its body changing.
func TestGatewayHandlerRelaysNotModifiedValidatorsOnly(t *testing.T) {
	var forwarded http.Header
	useGateways(t, &http.Client{Transport: gatewayRoundTripper(func(request *http.Request) (*http.Response, error) {
		forwarded = request.Header.Clone()
		return &http.Response{
			StatusCode: http.StatusNotModified,
			Header: http.Header{
				"X-Ar-Io-Hops":  []string{"1"},
				"Etag":          []string{`"tx"`},
				"Cache-Control": []string{"public, max-age=31536000, immutable"},
				"Expires":       []string{"Fri, 01 Jan 2027 00:00:00 GMT"},
				"Last-Modified": []string{"Thu, 01 Jan 2026 00:00:00 GMT"},
				"Vary":          []string{"Accept-Encoding"},
				"Content-Type":  []string{"text/html"},
			},
			Body:    http.NoBody,
			Request: request,
		}, nil
	})}, "https://gateway.example/")

	request := httptest.NewRequest(http.MethodGet, "/gateway/"+testFileKey, nil)
	request.Header.Set("If-None-Match", `"tx"`)
	recorder := serveGateway(request)

	require.Equal(t, http.StatusNotModified, recorder.Code)
	require.Empty(t, recorder.Body.String())
	require.Equal(t, `"tx"`, forwarded.Get("If-None-Match"))
	require.Equal(t, `"tx"`, recorder.Header().Get("Etag"))
	require.Equal(t, "public, max-age=31536000, immutable", recorder.Header().Get("Cache-Control"))
	require.Equal(t, "Fri, 01 Jan 2027 00:00:00 GMT", recorder.Header().Get("Expires"))
	require.Equal(t, "Thu, 01 Jan 2026 00:00:00 GMT", recorder.Header().Get("Last-Modified"))
	require.Equal(t, "Accept-Encoding", recorder.Header().Get("Vary"))
	for _, name := range []string{"Content-Type", "Content-Security-Policy", "Content-Length"} {
		require.Empty(t, recorder.Header().Values(name), "a 304 must not replace the cached %s", name)
	}
}
