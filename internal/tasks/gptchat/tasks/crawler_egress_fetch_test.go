package tasks

import (
	"compress/gzip"
	"context"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Laisky/laisky-blog-graphql/library/crawleregress"
	"github.com/stretchr/testify/require"
)

// rendererTestConn maps a synthetic public peer to a controlled loopback test fixture.
type rendererTestConn struct {
	net.Conn
	peer net.Addr
}

// RemoteAddr returns the synthetic admitted peer while all test traffic stays on loopback.
func (c *rendererTestConn) RemoteAddr() net.Addr { return c.peer }

// rendererTestDependencies maps only explicitly pinned addresses to a local test server.
func rendererTestDependencies(t *testing.T, server *httptest.Server, pinned string, hits *atomic.Int32) rendererNetworkDependencies {
	t.Helper()
	return rendererNetworkDependencies{
		resolve: func(_ context.Context, _ string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP(pinned)}}, nil
		},
		dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			require.NoError(t, err)
			require.Equal(t, pinned, host)
			require.Equal(t, "tcp", network)
			hits.Add(1)
			conn, err := new(net.Dialer).DialContext(ctx, "tcp", server.Listener.Addr().String())
			if err != nil {
				return nil, err
			}
			portNumber, err := strconv.Atoi(port)
			require.NoError(t, err)
			return &rendererTestConn{Conn: conn, peer: &net.TCPAddr{IP: net.ParseIP(pinned), Port: portNumber}}, nil
		},
	}
}

// rendererTestPolicy returns a public-address policy without consulting DNS or application settings.
func rendererTestPolicy(host, pinned string) crawleregress.Policy {
	return crawleregress.Policy{Host: host, Addresses: []string{pinned}, MaxRedirects: 2, AllowSubresources: true}
}

// TestRendererEgressPinned tests original-host pinning, canonical URLs, header isolation and actual receipts.
func TestRendererEgressPinned(t *testing.T) {
	var hits, resolutions atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Empty(t, r.Header.Get("Authorization"))
		require.Empty(t, r.Header.Get("Cookie"))
		require.Empty(t, r.Header.Get("X-Api-Key"))
		_, err := w.Write([]byte("safe body"))
		require.NoError(t, err)
	}))
	defer srv.Close()
	deps := rendererTestDependencies(t, srv, "93.184.216.34", &hits)
	deps.resolve = func(context.Context, string) ([]net.IPAddr, error) {
		resolutions.Add(1)
		return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
	}
	target := "http://EXAMPLE.COM.:80#fragment"
	f, err := newRendererEgressFetcher(context.Background(), "test-task", target, rendererTestPolicy("example.com", "93.184.216.34"), deps)
	require.NoError(t, err)
	headers := http.Header{"Authorization": []string{"Bearer synthetic-canary"}, "Cookie": []string{"canary=secret"}, "X-Api-Key": []string{"synthetic-canary"}}
	_, body, err := f.fetch(context.Background(), "first", "", "http://example.com/", crawleregress.OriginDocument, http.MethodGet, headers)
	require.NoError(t, err)
	require.Equal(t, "safe body", string(body))
	receipt, err := f.seal(context.Background())
	require.NoError(t, err)
	require.Len(t, receipt.Origins, 1)
	require.Equal(t, "93.184.216.34", receipt.Origins[0].PeerAddress)
	require.EqualValues(t, 9, receipt.Origins[0].BodyBytes)
	require.EqualValues(t, 0, resolutions.Load())
	require.EqualValues(t, 1, hits.Load())
}

// TestRendererEgressOriginCache prevents a new public origin from rebinding after its first admission.
func TestRendererEgressOriginCache(t *testing.T) {
	var hits, resolutions atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, err := w.Write([]byte("ok")); require.NoError(t, err) }))
	defer srv.Close()
	deps := rendererTestDependencies(t, srv, "93.184.216.34", &hits)
	deps.resolve = func(context.Context, string) ([]net.IPAddr, error) {
		if resolutions.Add(1) > 2 {
			return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
		}
		return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
	}
	f, err := newRendererEgressFetcher(context.Background(), "test", "http://example.com/", rendererTestPolicy("example.com", "93.184.216.34"), deps)
	require.NoError(t, err)
	for i, raw := range []string{"http://example.com/", "http://cdn.example.com/one", "http://CDN.EXAMPLE.COM.:80/two"} {
		kind := crawleregress.OriginSubresource
		if i == 0 {
			kind = crawleregress.OriginDocument
		}
		_, _, err := f.fetch(context.Background(), strconv.Itoa(i), "", raw, kind, http.MethodGet, nil)
		require.NoError(t, err)
	}
	require.EqualValues(t, 2, resolutions.Load())
	require.EqualValues(t, 3, hits.Load())
	_, err = f.seal(context.Background())
	require.NoError(t, err)
}

// TestRendererEgressRejectsBeforeDial checks private targets, methods, budgets and mismatched policy.
func TestRendererEgressRejectsBeforeDial(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, err := w.Write([]byte("ok")); require.NoError(t, err) }))
	defer srv.Close()
	deps := rendererTestDependencies(t, srv, "93.184.216.34", &hits)
	ctx := context.Background()
	for _, pins := range [][]string{nil, {"127.0.0.1"}, {"::1"}, {"::ffff:127.0.0.1"}} {
		_, err := newRendererEgressFetcher(ctx, "test", "http://example.com/", crawleregress.Policy{Host: "example.com", Addresses: pins}, deps)
		require.Error(t, err)
	}
	f, err := newRendererEgressFetcher(ctx, "test", "http://example.com/", rendererTestPolicy("other.example", "93.184.216.34"), deps)
	require.Error(t, err)
	require.Nil(t, f)
	f, err = newRendererEgressFetcher(ctx, "test", "http://example.com/", rendererTestPolicy("example.com", "93.184.216.34"), deps)
	require.NoError(t, err)
	_, _, err = f.fetch(ctx, "initial", "", "http://example.com/", crawleregress.OriginDocument, http.MethodGet, nil)
	require.NoError(t, err)
	for i, raw := range []string{"http://127.0.0.1/secret", "http://[::1]/secret", "http://2130706433/secret", "http://0x7f000001/secret", "http://169.254.169.254/secret", "file:///etc/passwd"} {
		_, _, err := f.fetch(ctx, fmt.Sprintf("private%d", i), "", raw, crawleregress.OriginSubresource, http.MethodGet, nil)
		require.Error(t, err)
	}
	_, _, err = f.fetch(ctx, "post", "", "http://example.com/", crawleregress.OriginSubresource, http.MethodPost, nil)
	require.Error(t, err)
	_, _, err = f.fetch(ctx, "parent", "missing", "http://example.com/", crawleregress.OriginDocument, http.MethodGet, nil)
	require.Error(t, err)
	require.EqualValues(t, 1, hits.Load())
	f.policy.MaxRedirects = 0
	_, _, err = f.fetch(ctx, "redirect", "initial", "http://public.example/", crawleregress.OriginDocument, http.MethodGet, nil)
	require.Error(t, err)
	f.policy.AllowSubresources = false
	_, _, err = f.fetch(ctx, "resource", "", "http://example.com/image", crawleregress.OriginSubresource, http.MethodGet, nil)
	require.Error(t, err)
	f.requests = rendererMaxRequests
	_, _, err = f.fetch(ctx, "budget", "", "http://example.com/image", crawleregress.OriginSubresource, http.MethodGet, nil)
	require.Error(t, err)
	require.EqualValues(t, 1, hits.Load())
}

// TestRendererEgressRedirects defers redirects to browser admission and rejects private hops without contact.
func TestRendererEgressRedirects(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.Header().Set("Location", "http://redirect.example/next")
			w.WriteHeader(302)
			return
		}
		_, err := w.Write([]byte("redirected"))
		require.NoError(t, err)
	}))
	defer srv.Close()
	f, err := newRendererEgressFetcher(context.Background(), "test", "http://example.com/", rendererTestPolicy("example.com", "93.184.216.34"), rendererTestDependencies(t, srv, "93.184.216.34", &hits))
	require.NoError(t, err)
	response, _, err := f.fetch(context.Background(), "first", "", "http://example.com/", crawleregress.OriginDocument, http.MethodGet, nil)
	require.NoError(t, err)
	require.Equal(t, 302, response.StatusCode)
	require.EqualValues(t, 1, hits.Load())
	_, _, err = f.fetch(context.Background(), "second", "first", response.Header.Get("Location"), crawleregress.OriginDocument, http.MethodGet, nil)
	require.NoError(t, err)
	_, _, err = f.fetch(context.Background(), "private", "second", "http://127.0.0.1/", crawleregress.OriginDocument, http.MethodGet, nil)
	require.Error(t, err)
	require.EqualValues(t, 2, hits.Load())
	receipt, err := f.seal(context.Background())
	require.NoError(t, err)
	require.Len(t, receipt.Origins, 2)
}

// TestRendererEgressIPv6 checks literal IPv6 pin dialing and matched connection evidence.
func TestRendererEgressIPv6(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, err := w.Write([]byte("v6")); require.NoError(t, err) }))
	defer srv.Close()
	pin := "2606:4700:4700::1111"
	f, err := newRendererEgressFetcher(context.Background(), "test", "http://example.com/", rendererTestPolicy("example.com", pin), rendererTestDependencies(t, srv, pin, &hits))
	require.NoError(t, err)
	_, _, err = f.fetch(context.Background(), "first", "", "http://example.com/", crawleregress.OriginDocument, http.MethodGet, nil)
	require.NoError(t, err)
	receipt, err := f.seal(context.Background())
	require.NoError(t, err)
	require.Equal(t, pin, receipt.Origins[0].PeerAddress)
}

// TestRendererEgressTLS verifies SNI and CA identity while the physical fixture remains local.
func TestRendererEgressTLS(t *testing.T) {
	var hits, requests atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		require.Equal(t, "example.com", r.TLS.ServerName)
		require.Equal(t, "example.com", r.Host)
		_, err := w.Write([]byte("tls"))
		require.NoError(t, err)
	}))
	defer srv.Close()
	deps := rendererTestDependencies(t, srv, "93.184.216.34", &hits)
	deps.rootCAs = x509.NewCertPool()
	deps.rootCAs.AddCert(srv.Certificate())
	for _, host := range []string{"example.com", "wrong.example"} {
		raw := "https://" + host + "/"
		f, err := newRendererEgressFetcher(context.Background(), "test", raw, rendererTestPolicy(host, "93.184.216.34"), deps)
		require.NoError(t, err)
		_, _, err = f.fetch(context.Background(), "first", "", raw, crawleregress.OriginDocument, http.MethodGet, nil)
		if host == "example.com" {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
	}
	require.EqualValues(t, 1, requests.Load(), "wrong hostname fails before HTTP content")
	deps.rootCAs = x509.NewCertPool()
	f, err := newRendererEgressFetcher(context.Background(), "test", "https://example.com/", rendererTestPolicy("example.com", "93.184.216.34"), deps)
	require.NoError(t, err)
	_, _, err = f.fetch(context.Background(), "untrusted", "", "https://example.com/", crawleregress.OriginDocument, http.MethodGet, nil)
	require.Error(t, err)
	require.EqualValues(t, 1, requests.Load(), "untrusted CA fails before HTTP content")
}

// TestRendererEgressPeerAndCancellation prevents mismatched peers from receiving HTTP and binds timeout cancellation.
func TestRendererEgressPeerAndCancellation(t *testing.T) {
	var hits, requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path == "/slow" {
			<-r.Context().Done()
			return
		}
		_, err := w.Write([]byte("ok"))
		require.NoError(t, err)
	}))
	defer srv.Close()
	deps := rendererTestDependencies(t, srv, "93.184.216.34", &hits)
	baseDial := deps.dial
	deps.dial = func(ctx context.Context, network, address string) (net.Conn, error) {
		conn, err := baseDial(ctx, network, address)
		if err != nil {
			return nil, err
		}
		return &rendererTestConn{Conn: conn, peer: &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 80}}, nil
	}
	f, err := newRendererEgressFetcher(context.Background(), "test", "http://example.com/", rendererTestPolicy("example.com", "93.184.216.34"), deps)
	require.NoError(t, err)
	_, _, err = f.fetch(context.Background(), "first", "", "http://example.com/", crawleregress.OriginDocument, http.MethodGet, nil)
	require.Error(t, err)
	require.EqualValues(t, 0, requests.Load())
	deps.dial = baseDial
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	f, err = newRendererEgressFetcher(ctx, "test", "http://example.com/slow", rendererTestPolicy("example.com", "93.184.216.34"), deps)
	require.NoError(t, err)
	_, _, err = f.fetch(ctx, "first", "", "http://example.com/slow", crawleregress.OriginDocument, http.MethodGet, nil)
	require.Error(t, err)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	ctx, cancel = context.WithCancel(context.Background())
	cancel()
	_, _, err = f.fetch(ctx, "cancel", "", "http://example.com/image", crawleregress.OriginSubresource, http.MethodGet, nil)
	require.ErrorIs(t, err, context.Canceled)
}

// TestRendererEgressEncoding rejects unsupported encodings and bounds decoded gzip responses.
func TestRendererEgressEncoding(t *testing.T) {
	for _, encoding := range []string{"identity", "gzip", "br", "deflate", "gzip-bomb"} {
		t.Run(encoding, func(t *testing.T) {
			var hits atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				name := encoding
				if name == "gzip-bomb" {
					name = "gzip"
				}
				w.Header().Set("Content-Encoding", name)
				if name == "gzip" {
					gz := gzip.NewWriter(w)
					body := "safe"
					if encoding == "gzip-bomb" {
						body = strings.Repeat("x", rendererMaxResponseBytes+1)
					}
					_, err := gz.Write([]byte(body))
					require.NoError(t, err)
					require.NoError(t, gz.Close())
					return
				}
				_, err := w.Write([]byte("safe"))
				require.NoError(t, err)
			}))
			defer srv.Close()
			f, err := newRendererEgressFetcher(context.Background(), "test", "http://example.com/", rendererTestPolicy("example.com", "93.184.216.34"), rendererTestDependencies(t, srv, "93.184.216.34", &hits))
			require.NoError(t, err)
			response, body, err := f.fetch(context.Background(), "first", "", "http://example.com/", crawleregress.OriginDocument, http.MethodGet, nil)
			if encoding == "identity" || encoding == "gzip" {
				require.NoError(t, err)
				require.Equal(t, "safe", string(body))
				require.Empty(t, response.Header.Get("Content-Encoding"))
			} else {
				require.Error(t, err)
			}
		})
	}
}
