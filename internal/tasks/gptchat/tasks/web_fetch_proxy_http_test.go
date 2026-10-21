package tasks

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/x509"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
)

// providerHTTPFixture describes an existing adapter and its owned HTTP response.
type providerHTTPFixture struct {
	name, response, authorization, apiToken string
	provider                                func(string) webFetchProxy
}

// providerHTTPFixtures returns the existing adapter variants with synthetic
// configured credentials; no caller credential is accepted by their interface.
func providerHTTPFixtures() []providerHTTPFixture {
	return []providerHTTPFixture{
		{name: "prefix", response: "provider-content", provider: func(endpoint string) webFetchProxy {
			return prefixedWebFetchProxy{name: "custom-reader", prefix: endpoint + "/"}
		}},
		{name: "scrapeless", response: `{"data":{"content":"provider-content"}}`, apiToken: "provider-token-only", provider: func(endpoint string) webFetchProxy {
			return scrapelessWebFetchProxy{apiURL: endpoint, apiKey: "provider-token-only", actor: "fixture", proxyCountry: "ANY"}
		}},
		{name: "firecrawl", response: `{"success":true,"data":{"markdown":"provider-content","metadata":{"statusCode":200}}}`, authorization: "Bearer provider-token-only", provider: func(endpoint string) webFetchProxy {
			return firecrawlWebFetchProxy{apiURL: endpoint, apiKey: "provider-token-only"}
		}},
	}
}

// TestVerifiedProviderHTTPRejectsPrivateEndpoint proves scoped providers reject
// a private configured endpoint while the legacy adapter retains its behavior.
func TestVerifiedProviderHTTPRejectsPrivateEndpoint(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, err := w.Write([]byte("provider-content"))
		require.NoError(t, err)
	}))
	t.Cleanup(server.Close)
	provider := prefixedWebFetchProxy{name: "local-fixture", prefix: server.URL + "/"}
	legacy, err := provider.Fetch(context.Background(), "https://example.com/document")
	require.NoError(t, err)
	require.Equal(t, "provider-content", legacy)
	require.EqualValues(t, 1, requests.Load())

	ctx := withVerifiedWebFetchClient(t.Context(), rendererNetworkDependencies{})
	body, err := provider.Fetch(ctx, "https://example.com/document")
	require.Error(t, err)
	require.Empty(t, body)
	require.EqualValues(t, 1, requests.Load(), "private provider endpoint must not receive a second request")
}

// TestVerifiedProviderHTTPAdapters preserves all configured adapters while
// binding public provider exchanges to their pins and only their own credentials.
func TestVerifiedProviderHTTPAdapters(t *testing.T) {
	for _, fixture := range providerHTTPFixtures() {
		t.Run(fixture.name, func(t *testing.T) {
			var requests, dials atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				requests.Add(1)
				require.Equal(t, "example.com", request.Host)
				require.Equal(t, fixture.authorization, request.Header.Get("Authorization"))
				require.Equal(t, fixture.apiToken, request.Header.Get("X-Api-Token"))
				require.Empty(t, request.Header.Get("Cookie"))
				require.Empty(t, request.Header.Get("X-Api-Key"))
				require.Empty(t, request.Header.Get("Referer"))
				if request.Method == http.MethodPost {
					body, err := io.ReadAll(request.Body)
					require.NoError(t, err)
					require.Contains(t, string(body), "https://example.org/document")
					require.NotContains(t, string(body), "provider-token-only")
					require.NotContains(t, string(body), `"headers"`)
				}
				_, err := io.WriteString(w, fixture.response)
				require.NoError(t, err)
			}))
			t.Cleanup(server.Close)
			t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
			t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
			deps := rendererTestDependencies(t, server, "93.184.216.34", &dials)
			ctx := withVerifiedWebFetchClient(t.Context(), deps)
			// The worker enables enforcement again without replacing owned test dependencies.
			ctx = withVerifiedWebFetchClient(ctx, rendererNetworkDependencies{})
			body, err := fixture.provider("http://EXAMPLE.COM.:80").Fetch(ctx, "https://example.org/document")
			require.NoError(t, err)
			require.Equal(t, "provider-content", body)
			require.EqualValues(t, 1, requests.Load())
			require.EqualValues(t, 1, dials.Load())
		})
	}
}

// TestVerifiedProviderHTTPRejectsRedirects exercises actual adapter exchanges
// and proves configured credentials never reach a provider's redirect target.
func TestVerifiedProviderHTTPRejectsRedirects(t *testing.T) {
	for _, fixture := range providerHTTPFixtures() {
		t.Run(fixture.name, func(t *testing.T) {
			var requests, dials atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				requests.Add(1)
				if request.Host == "redirect.example" {
					t.Error("provider redirect must never be requested")
					_, err := io.WriteString(w, fixture.response)
					require.NoError(t, err)
					return
				}
				w.Header().Set("Location", "http://redirect.example/credential-sentinel")
				w.WriteHeader(http.StatusTemporaryRedirect)
			}))
			t.Cleanup(server.Close)
			ctx := withVerifiedWebFetchClient(t.Context(), rendererTestDependencies(t, server, "93.184.216.34", &dials))
			body, err := fixture.provider("http://example.com").Fetch(ctx, "https://example.org/document")
			require.Error(t, err)
			require.Contains(t, err.Error(), "307")
			require.Empty(t, body)
			require.EqualValues(t, 1, requests.Load())
			require.EqualValues(t, 1, dials.Load())
		})
	}
}

// TestVerifiedProviderHTTPRejectsUnapprovedEndpoints rejects endpoint credentials,
// nonpublic destinations and mixed DNS answers before opening a connection.
func TestVerifiedProviderHTTPRejectsUnapprovedEndpoints(t *testing.T) {
	for _, scenario := range []struct {
		name, endpoint string
		addresses      []string
	}{
		{name: "private_literal", endpoint: "http://127.0.0.1"},
		{name: "private_dns", endpoint: "http://example.com", addresses: []string{"127.0.0.1"}},
		{name: "mixed_dns", endpoint: "http://example.com", addresses: []string{"93.184.216.34", "127.0.0.1"}},
		{name: "credentials", endpoint: "http://user:credential@example.com", addresses: []string{"93.184.216.34"}},
		{name: "local_name", endpoint: "http://metadata.internal", addresses: []string{"93.184.216.34"}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			var dials atomic.Int32
			deps := rendererNetworkDependencies{
				resolve: func(context.Context, string) ([]net.IPAddr, error) {
					addresses := make([]net.IPAddr, 0, len(scenario.addresses))
					for _, address := range scenario.addresses {
						addresses = append(addresses, net.IPAddr{IP: net.ParseIP(address)})
					}
					return addresses, nil
				},
				dial: func(context.Context, string, string) (net.Conn, error) {
					dials.Add(1)
					return nil, errors.New("unexpected provider dial")
				},
			}
			provider := prefixedWebFetchProxy{name: "custom-reader", prefix: scenario.endpoint + "/"}
			body, err := provider.Fetch(withVerifiedWebFetchClient(t.Context(), deps), "https://example.org/document")
			require.Error(t, err)
			require.Empty(t, body)
			require.Zero(t, dials.Load())
		})
	}
}

// TestVerifiedProviderHTTPRejectsWrongPeer proves a private connected peer cannot
// receive a request, even after DNS admitted a synthetic public endpoint.
func TestVerifiedProviderHTTPRejectsWrongPeer(t *testing.T) {
	var requests, dials atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, err := io.WriteString(w, "must-not-read")
		require.NoError(t, err)
	}))
	t.Cleanup(server.Close)
	deps := rendererTestDependencies(t, server, "93.184.216.34", &dials)
	publicDial := deps.dial
	deps.dial = func(ctx context.Context, network, address string) (net.Conn, error) {
		conn, err := publicDial(ctx, network, address)
		if err != nil {
			return nil, errors.Wrap(err, "dial owned fixture")
		}
		return &rendererTestConn{Conn: conn, peer: &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 80}}, nil
	}
	provider := prefixedWebFetchProxy{name: "custom-reader", prefix: "http://example.com/"}
	body, err := provider.Fetch(withVerifiedWebFetchClient(t.Context(), deps), "https://example.org/document")
	require.Error(t, err)
	require.Empty(t, body)
	require.EqualValues(t, 1, dials.Load())
	require.Zero(t, requests.Load())
}

// TestVerifiedProviderHTTPTLS checks server identity and certificate verification
// while every physical connection stays on an owned loopback TLS fixture.
func TestVerifiedProviderHTTPTLS(t *testing.T) {
	var requests, dials atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		require.Equal(t, "example.com", request.TLS.ServerName)
		_, err := io.WriteString(w, "provider-content")
		require.NoError(t, err)
	}))
	t.Cleanup(server.Close)
	for _, scenario := range []string{"trusted", "wrong_host", "untrusted"} {
		t.Run(scenario, func(t *testing.T) {
			deps := rendererTestDependencies(t, server, "93.184.216.34", &dials)
			if scenario != "untrusted" {
				deps.rootCAs = x509.NewCertPool()
				deps.rootCAs.AddCert(server.Certificate())
			}
			host := "example.com"
			if scenario == "wrong_host" {
				host = "wrong.example"
			}
			provider := prefixedWebFetchProxy{name: "custom-reader", prefix: "https://" + host + "/"}
			body, err := provider.Fetch(withVerifiedWebFetchClient(t.Context(), deps), "https://example.org/document")
			if scenario == "trusted" {
				require.NoError(t, err)
				require.Equal(t, "provider-content", body)
			} else {
				require.Error(t, err)
				require.Empty(t, body)
			}
		})
	}
	require.EqualValues(t, 1, requests.Load())
	require.EqualValues(t, 3, dials.Load())
}

// TestVerifiedProviderHTTPBodyBudget accepts bounded plain/gzip content and
// rejects either a raw or decoded body exceeding the existing renderer limit.
func TestVerifiedProviderHTTPBodyBudget(t *testing.T) {
	for _, scenario := range []struct {
		name     string
		size     int
		gzipBody bool
	}{
		{name: "raw_boundary", size: rendererMaxResponseBytes},
		{name: "raw_overflow", size: rendererMaxResponseBytes + 1},
		{name: "gzip_content", size: 1024, gzipBody: true},
		{name: "gzip_overflow", size: rendererMaxResponseBytes + 1, gzipBody: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			payload := []byte(strings.Repeat("x", scenario.size))
			if scenario.gzipBody {
				var compressed bytes.Buffer
				writer := gzip.NewWriter(&compressed)
				_, err := writer.Write(payload)
				require.NoError(t, err)
				require.NoError(t, writer.Close())
				payload = compressed.Bytes()
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if scenario.gzipBody {
					w.Header().Set("Content-Encoding", "gzip")
				}
				_, err := w.Write(payload)
				require.NoError(t, err)
			}))
			t.Cleanup(server.Close)
			var dials atomic.Int32
			ctx := withVerifiedWebFetchClient(t.Context(), rendererTestDependencies(t, server, "93.184.216.34", &dials))
			provider := prefixedWebFetchProxy{name: "custom-reader", prefix: "http://example.com/"}
			body, err := provider.Fetch(ctx, "https://example.org/document")
			if scenario.size > rendererMaxResponseBytes {
				require.Error(t, err)
				require.Contains(t, err.Error(), "budget exhausted")
				require.Empty(t, body)
			} else {
				require.NoError(t, err)
				require.Len(t, body, scenario.size)
			}
			require.EqualValues(t, 1, dials.Load())
		})
	}
}

// TestVerifiedProviderHTTPCancellation proves caller cancellation stops an
// in-flight provider request rather than waiting for its independent client cap.
func TestVerifiedProviderHTTPCancellation(t *testing.T) {
	entered := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		close(entered)
		<-request.Context().Done()
	}))
	t.Cleanup(server.Close)
	var dials atomic.Int32
	ctx, cancel := context.WithCancel(withVerifiedWebFetchClient(t.Context(), rendererTestDependencies(t, server, "93.184.216.34", &dials)))
	t.Cleanup(cancel)
	provider := prefixedWebFetchProxy{name: "custom-reader", prefix: "http://example.com/"}
	finished := make(chan error, 1)
	go func() {
		_, err := provider.Fetch(ctx, "https://example.org/document")
		finished <- err
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("owned provider request did not begin")
	}
	cancel()
	select {
	case err := <-finished:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(3 * time.Second):
		t.Fatal("provider ignored caller cancellation")
	}
	require.EqualValues(t, 1, dials.Load())
}

// TestWebFetchProxyLegacyHTTPRedirects preserves the unscoped adapter's existing
// redirect behavior so verified transport enforcement remains explicitly opt-in.
func TestWebFetchProxyLegacyHTTPRedirects(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		if request.URL.Path != "/final" {
			w.Header().Set("Location", "/final")
			w.WriteHeader(http.StatusFound)
			return
		}
		_, err := io.WriteString(w, "legacy-content")
		require.NoError(t, err)
	}))
	t.Cleanup(server.Close)
	provider := prefixedWebFetchProxy{name: "local-fixture", prefix: server.URL + "/"}
	body, err := provider.Fetch(t.Context(), "https://example.org/document")
	require.NoError(t, err)
	require.Equal(t, "legacy-content", body)
	require.EqualValues(t, 2, requests.Load())
}
