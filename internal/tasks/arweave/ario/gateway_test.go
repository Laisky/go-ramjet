package ario

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// testFileKey is a syntactically valid Arweave transaction id.
const testFileKey = "bNbA3TEQVL60xlgCcqdz4ZPHFZ711cZ3hmkpGttDt_U"

// gatewayRoundTripper answers fan-out requests in memory without opening sockets.
type gatewayRoundTripper func(*http.Request) (*http.Response, error)

// RoundTrip returns the synthetic response for request.
func (fn gatewayRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

// gatewayOK returns a successful gateway response that the handler accepts.
func gatewayOK(request *http.Request) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"X-Ar-Io-Hops": []string{"1"}},
		Body:       io.NopCloser(strings.NewReader("content")),
		Request:    request,
	}
}

// useGateways replaces the gateway list and client for one test and restores them afterwards.
func useGateways(t *testing.T, client *http.Client, gateways ...string) {
	t.Helper()
	originalGateways, originalClient := ArweaveGateways, httpcli
	t.Cleanup(func() { ArweaveGateways, httpcli = originalGateways, originalClient })
	ArweaveGateways, httpcli = gateways, client
}

// loopbackOnlyClient trusts server's certificate, counts every dial attempt in
// dials and connects to nothing but server, so a test never resolves or
// contacts any other host.
func loopbackOnlyClient(t *testing.T, server *httptest.Server, dials *atomic.Int32) *http.Client {
	t.Helper()
	base, ok := server.Client().Transport.(*http.Transport)
	require.True(t, ok)
	transport := base.Clone()
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		dials.Add(1)
		if address != server.Listener.Addr().String() {
			return nil, errors.Errorf("unexpected dial to %s", address)
		}
		var dialer net.Dialer
		conn, err := dialer.DialContext(ctx, network, address)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		return conn, nil
	}
	return &http.Client{Transport: transport}
}

// serveGateway sends request through a router that mounts GatewayHandler like production.
func serveGateway(request *http.Request) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Any("/gateway/*fileKey", GatewayHandler)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	return recorder
}

// TestGatewayHandlerDomainCannotSteerHost proves that the caller-supplied
// domain only ever selects a subdomain of a configured gateway. Without that,
// URL delimiters in domain move the request authority to any host, here a
// loopback HTTPS service standing in for an internal endpoint.
func TestGatewayHandlerDomainCannotSteerHost(t *testing.T) {
	var internalHits, dials atomic.Int32
	internal := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		internalHits.Add(1)
		w.Header().Set("X-Ar-Io-Hops", "1")
		_, _ = io.WriteString(w, "internal-secret")
	}))
	t.Cleanup(internal.Close)
	useGateways(t, loopbackOnlyClient(t, internal, &dials), "https://gateway.invalid/")
	authority := strings.TrimPrefix(internal.URL, "https://")

	for _, domain := range []string{
		authority + "/",
		authority + "#",
		authority + "?",
		"user@" + authority + "/",
		"user:pass@" + authority + "/",
		authority + `\`,
		"evil.example#",
		"a.b",
		"name%2F",
		" name",
		"-name",
		"name-",
		strings.Repeat("a", 64),
	} {
		t.Run(domain, func(t *testing.T) {
			internalHits.Store(0)
			dials.Store(0)
			target := "/gateway/" + testFileKey + "?domain=" + url.QueryEscape(domain)
			recorder := serveGateway(httptest.NewRequest(http.MethodGet, target, nil))
			require.Equal(t, http.StatusBadRequest, recorder.Code)
			require.NotContains(t, recorder.Body.String(), "internal-secret")
			require.Zero(t, internalHits.Load(), "a domain must never reach a host outside the gateway list")
			require.Zero(t, dials.Load(), "an invalid domain must be refused before any connection")
		})
	}
}

// TestGatewayHandlerDomainSelectsGatewaySubdomain keeps ArNS name and undername lookups working.
func TestGatewayHandlerDomainSelectsGatewaySubdomain(t *testing.T) {
	for _, domain := range []string{"ardrive", "docs_ardrive", "my-name", "a", "Name0", strings.Repeat("a", 63)} {
		t.Run(domain, func(t *testing.T) {
			var mu sync.Mutex
			var hosts []string
			useGateways(t, &http.Client{Transport: gatewayRoundTripper(func(request *http.Request) (*http.Response, error) {
				mu.Lock()
				hosts = append(hosts, request.URL.Host)
				mu.Unlock()
				return gatewayOK(request), nil
			})}, "https://gateway.example/")
			target := "/gateway/" + testFileKey + "?domain=" + url.QueryEscape(domain)
			recorder := serveGateway(httptest.NewRequest(http.MethodGet, target, nil))
			require.Equal(t, http.StatusOK, recorder.Code)
			require.Equal(t, "content", recorder.Body.String())
			require.Equal(t, []string{domain + ".gateway.example"}, hosts)
		})
	}
}

// gatewayMiss returns a gateway failure, so the handler waits for every gateway.
func gatewayMiss(request *http.Request) *http.Response {
	return &http.Response{StatusCode: http.StatusNotFound, Header: http.Header{}, Body: http.NoBody, Request: request}
}

// TestGatewayHandlerForwardsOnlyContentNegotiationHeaders keeps the caller's
// credentials, cookies and client metadata away from third-party gateways
// while still forwarding the headers that shape the requested content.
func TestGatewayHandlerForwardsOnlyContentNegotiationHeaders(t *testing.T) {
	var mu sync.Mutex
	var seen []http.Header
	useGateways(t, &http.Client{Transport: gatewayRoundTripper(func(request *http.Request) (*http.Response, error) {
		mu.Lock()
		seen = append(seen, request.Header.Clone())
		mu.Unlock()
		return gatewayMiss(request), nil
	})}, "https://one.example/", "https://two.example/")

	request := httptest.NewRequest(http.MethodGet, "/gateway/"+testFileKey, nil)
	request.Header.Set("Cookie", "session=SYNTHETIC-SESSION")
	request.Header.Set("Authorization", "Bearer SYNTHETIC-TOKEN")
	request.Header.Set("Proxy-Authorization", "Basic SYNTHETIC")
	request.Header.Set("X-Forwarded-For", "203.0.113.7")
	request.Header.Set("X-Laisky-Api-Base", "https://provider.example")
	request.Header.Set("Range", "bytes=0-9")
	request.Header.Set("If-None-Match", `"etag"`)
	request.Header.Set("Accept", "image/png")
	recorder := serveGateway(request)
	require.Equal(t, http.StatusBadRequest, recorder.Code)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, seen, 2)
	for _, header := range seen {
		for _, name := range []string{"Cookie", "Authorization", "Proxy-Authorization", "X-Forwarded-For", "X-Laisky-Api-Base"} {
			require.Empty(t, header.Values(name), name)
		}
		require.Equal(t, "bytes=0-9", header.Get("Range"))
		require.Equal(t, `"etag"`, header.Get("If-None-Match"))
		require.Equal(t, "image/png", header.Get("Accept"))
	}
}

// TestGatewayHandlerGivesEveryGatewayTheWholeBody sends each concurrent
// gateway request its own copy of the caller's body instead of letting all of
// them race over one shared reader.
func TestGatewayHandlerGivesEveryGatewayTheWholeBody(t *testing.T) {
	var mu sync.Mutex
	var bodies []string
	useGateways(t, &http.Client{Transport: gatewayRoundTripper(func(request *http.Request) (*http.Response, error) {
		payload, err := io.ReadAll(request.Body)
		if err != nil {
			return nil, err
		}
		mu.Lock()
		bodies = append(bodies, string(payload))
		mu.Unlock()
		return gatewayMiss(request), nil
	})}, "https://one.example/", "https://two.example/", "https://three.example/")

	request := httptest.NewRequest(http.MethodPost, "/gateway/"+testFileKey, strings.NewReader("synthetic-payload"))
	recorder := serveGateway(request)
	require.Equal(t, http.StatusBadRequest, recorder.Code)

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, []string{"synthetic-payload", "synthetic-payload", "synthetic-payload"}, bodies)
}
