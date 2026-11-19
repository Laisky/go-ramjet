package egress

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
)

// forwardProxy is an HTTP forward proxy fixture that records the absolute
// targets it was asked for and answers them itself.
type forwardProxy struct {
	*httptest.Server
	mu      sync.Mutex
	targets []string
}

// newForwardProxy starts a proxy that redirects /redirect to location and
// answers every other target with "via-proxy".
func newForwardProxy(t *testing.T, location string) *forwardProxy {
	t.Helper()
	proxy := &forwardProxy{}
	proxy.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxy.mu.Lock()
		proxy.targets = append(proxy.targets, r.URL.String())
		proxy.mu.Unlock()
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, location, http.StatusFound)
			return
		}
		_, _ = io.WriteString(w, "via-proxy")
	}))
	t.Cleanup(proxy.Close)
	return proxy
}

// seen returns the targets the proxy received so far.
func (p *forwardProxy) seen() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.targets...)
}

// publicResolver answers public.example with a public address and nothing else.
func publicResolver(_ context.Context, host string) ([]net.IPAddr, error) {
	if host != "public.example" {
		return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
	}
	return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
}

// TestProxiedClientAdmitsEveryHopBeforeTheProxy sends admitted targets through
// the operator proxy and refuses non-public targets, including redirect hops,
// before the proxy is asked for them.
func TestProxiedClientAdmitsEveryHopBeforeTheProxy(t *testing.T) {
	proxy := newForwardProxy(t, "http://10.0.0.1/latest/meta-data/")
	proxyURL, err := url.Parse(proxy.URL)
	require.NoError(t, err)
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	client, err := NewProxiedHTTPClient(5*time.Second, proxyURL, Dependencies{Resolve: publicResolver},
		WithResponseHeaderTimeout(time.Minute))
	require.NoError(t, err)

	response, err := client.Get("http://Public.Example./page")
	require.NoError(t, err)
	body, err := io.ReadAll(response.Body)
	require.NoError(t, errors.Join(err, response.Body.Close()))
	require.Equal(t, "via-proxy", string(body))
	require.Equal(t, []string{"http://public.example/page"}, proxy.seen(), "the proxy gets the admitted, canonical target")

	for _, target := range []string{"http://127.0.0.1:8080/", "http://169.254.169.254/latest/meta-data/", "http://localhost/", "http://unknown.example/"} {
		response, err = client.Get(target)
		if response != nil {
			require.NoError(t, response.Body.Close())
		}
		require.Error(t, err, target)
		_, refused := ReasonOf(err)
		require.True(t, refused, target)
	}
	require.Len(t, proxy.seen(), 1, "refused targets never reach the proxy")

	response, err = client.Get("http://public.example/redirect")
	if response != nil {
		require.NoError(t, response.Body.Close())
	}
	require.True(t, IsExpectedRefusal(err), "a redirect into a private address is refused at its hop")
	require.Equal(t, []string{"http://public.example/page", "http://public.example/redirect"}, proxy.seen())
}

// TestNewProxiedTransportRequiresProxy rejects a missing or relative proxy URL.
func TestNewProxiedTransportRequiresProxy(t *testing.T) {
	_, err := NewProxiedTransport(nil, Dependencies{})
	require.Error(t, err)
	_, err = NewProxiedTransport(&url.URL{Path: "proxy"}, Dependencies{})
	require.Error(t, err)
}
