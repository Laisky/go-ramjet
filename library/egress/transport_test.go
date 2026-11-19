package egress

import (
	"context"
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

// peerConn reports a synthetic public peer for a connection to a local fixture.
type peerConn struct {
	net.Conn
	peer net.Addr
}

// RemoteAddr returns the synthetic peer instead of the fixture's loopback address.
func (c *peerConn) RemoteAddr() net.Addr { return c.peer }

// publicFixture routes every pinned dial for a synthetic public host to server,
// reporting the pinned public peer, and counts dials.
func publicFixture(t *testing.T, server *httptest.Server, dials *atomic.Int32) Dependencies {
	t.Helper()
	return Dependencies{
		Resolve: func(_ context.Context, host string) ([]net.IPAddr, error) {
			if host != "public.example" {
				return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
			}
			return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
		},
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			dials.Add(1)
			host, port, err := net.SplitHostPort(address)
			require.NoError(t, err)
			require.Equal(t, "93.184.216.34", host, "dial must use the admitted pin")
			var dialer net.Dialer
			conn, err := dialer.DialContext(ctx, network, server.Listener.Addr().String())
			if err != nil {
				return nil, errors.WithStack(err)
			}
			portNumber, err := net.LookupPort("tcp", port)
			require.NoError(t, err)
			return &peerConn{Conn: conn, peer: &net.TCPAddr{IP: net.ParseIP(host), Port: portNumber}}, nil
		},
	}
}

// TestPinnedClientFetchesPublicTarget preserves ordinary public fetches and ignores environment proxies.
func TestPinnedClientFetchesPublicTarget(t *testing.T) {
	var dials atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "public.example", r.Host)
		_, err := io.WriteString(w, "public-content")
		require.NoError(t, err)
	}))
	t.Cleanup(server.Close)
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	client := NewHTTPClient(5*time.Second, publicFixture(t, server, &dials))
	response, err := client.Get("http://public.example/page")
	require.NoError(t, err)
	body, err := io.ReadAll(response.Body)
	require.NoError(t, errors.Join(err, response.Body.Close()))
	require.Equal(t, "public-content", string(body))
	require.EqualValues(t, 1, dials.Load())
}

// TestPinnedClientRefusesRedirectIntoPrivateNetwork re-admits every redirect hop,
// so an admitted public host cannot bounce the client into a private address.
func TestPinnedClientRefusesRedirectIntoPrivateNetwork(t *testing.T) {
	var privateRequests, dials atomic.Int32
	private := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		privateRequests.Add(1)
	}))
	t.Cleanup(private.Close)
	public := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, private.URL+"/metadata", http.StatusFound)
	}))
	t.Cleanup(public.Close)
	client := NewHTTPClient(5*time.Second, publicFixture(t, public, &dials))
	response, err := client.Get("http://public.example/start")
	if response != nil {
		require.NoError(t, response.Body.Close())
	}
	require.Error(t, err)
	reason, ok := ReasonOf(err)
	require.True(t, ok)
	require.Equal(t, ReasonDestinationDenied, reason)
	require.Zero(t, privateRequests.Load(), "private redirect target must never be contacted")
	require.EqualValues(t, 1, dials.Load())
}

// TestPinnedClientBoundsRedirects stops a public redirect loop at the shared budget.
func TestPinnedClientBoundsRedirects(t *testing.T) {
	var requests, dials atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.Redirect(w, r, "/loop", http.StatusFound)
	}))
	t.Cleanup(server.Close)
	client := NewHTTPClient(5*time.Second, publicFixture(t, server, &dials))
	response, err := client.Get("http://public.example/loop")
	if response != nil {
		require.NoError(t, response.Body.Close())
	}
	require.ErrorContains(t, err, "stopped after 5 redirects")
	require.EqualValues(t, 6, requests.Load())
}

// TestPinnedTransportRejectsPeerOutsidePins never sends a request to a peer
// other than the admitted addresses, even after a successful dial.
func TestPinnedTransportRejectsPeerOutsidePins(t *testing.T) {
	var requests, dials atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests.Add(1)
	}))
	t.Cleanup(server.Close)
	deps := publicFixture(t, server, &dials)
	pinnedDial := deps.Dial
	deps.Dial = func(ctx context.Context, network, address string) (net.Conn, error) {
		conn, err := pinnedDial(ctx, network, address)
		if err != nil {
			return nil, err
		}
		return &peerConn{Conn: conn, peer: &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 80}}, nil
	}
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://public.example/", strings.NewReader("body"))
	require.NoError(t, err)
	response, err := PinnedTransport{Deps: deps}.RoundTrip(request)
	if response != nil {
		require.NoError(t, response.Body.Close())
	}
	require.ErrorContains(t, err, "peer outside admitted pins")
	require.Zero(t, requests.Load())
}

// TestPinnedTransportRefusesBeforeDial proves refused targets never open a connection.
func TestPinnedTransportRefusesBeforeDial(t *testing.T) {
	var dials atomic.Int32
	deps := Dependencies{Dial: func(context.Context, string, string) (net.Conn, error) {
		dials.Add(1)
		return nil, errors.New("unexpected dial")
	}}
	for _, target := range []string{"http://127.0.0.1/", "http://localhost/", "http://user:secret@public.example/"} {
		request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, target, nil)
		require.NoError(t, err)
		response, err := PinnedTransport{Deps: deps}.RoundTrip(request)
		require.Nil(t, response)
		require.True(t, IsExpectedRefusal(err), target)
	}
	require.Zero(t, dials.Load())
}

// TestPinnedClientResponseHeaderTimeout applies the configured header bound,
// so callers of slow upstreams such as model APIs can extend the default.
func TestPinnedClientResponseHeaderTimeout(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		_, err := io.WriteString(w, "late")
		require.NoError(t, err)
	}))
	t.Cleanup(server.Close)
	var dials atomic.Int32
	deps := publicFixture(t, server, &dials)

	client := NewHTTPClient(5*time.Second, deps, WithResponseHeaderTimeout(50*time.Millisecond))
	require.Equal(t, 50*time.Millisecond, client.Transport.(PinnedTransport).ResponseHeaderTimeout)
	response, err := client.Get("http://public.example/slow")
	if response != nil {
		require.NoError(t, response.Body.Close())
	}
	require.ErrorContains(t, err, "timeout awaiting response headers")

	client = NewHTTPClient(5*time.Second, deps, WithResponseHeaderTimeout(time.Minute))
	time.AfterFunc(200*time.Millisecond, func() { close(release) })
	response, err = client.Get("http://public.example/slow")
	require.NoError(t, err)
	body, err := io.ReadAll(response.Body)
	require.NoError(t, errors.Join(err, response.Body.Close()))
	require.Equal(t, "late", string(body))

	for _, opts := range [][]ClientOption{nil, {WithResponseHeaderTimeout(0)}, {WithResponseHeaderTimeout(-time.Second)}} {
		client = NewHTTPClient(time.Second, deps, opts...)
		require.Zero(t, client.Transport.(PinnedTransport).ResponseHeaderTimeout, "non-positive values keep the default")
	}
}
