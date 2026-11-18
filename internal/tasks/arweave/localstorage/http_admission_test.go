package localstorage

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/internal/tasks/arweave/config"
	"github.com/Laisky/go-ramjet/library/egress"
)

var archiveTestConfigOnce sync.Once

// useArchiveTestConfig installs one process-wide config whose S3 endpoint refuses
// connections. It is never restored because handler goroutines may outlive a test.
func useArchiveTestConfig() {
	archiveTestConfigOnce.Do(func() {
		config.Instance = &config.Config{}
		config.Instance.S3.Endpoint = "127.0.0.1:1"
		config.Instance.S3.Bucket = "archive-test"
	})
}

// drainArchiveTasks empties the shared archive queue and returns how many items it held.
func drainArchiveTasks() int {
	drained := 0
	for {
		select {
		case <-taskChan:
			drained++
		default:
			return drained
		}
	}
}

// serveCacheHandler runs CacheHandler for raw through a real gin router and returns the recorder.
func serveCacheHandler(raw string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/proxy", CacheHandler)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/proxy?url="+url.QueryEscape(raw), nil)
	router.ServeHTTP(recorder, request)
	return recorder
}

// TestCacheHandlerRefusesNonPublicTargets proves /arweave/proxy applies the shared
// destination policy before any direct fetch, cache lookup or archive scheduling.
// Every target below is owned by the test; none of them may receive a request.
func TestCacheHandlerRefusesNonPublicTargets(t *testing.T) {
	useArchiveTestConfig()
	var contacted atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		contacted.Add(1)
		if _, err := io.WriteString(w, "private-content"); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(target.Close)

	for _, raw := range []string{
		target.URL + "/private-path?token=private-query",
		"http://localhost:1/metadata",
		"http://169.254.169.254/latest/meta-data/",
		"ftp://example.com/resource",
	} {
		t.Run(raw, func(t *testing.T) {
			drainArchiveTasks()
			recorder := serveCacheHandler(raw)
			require.Equal(t, http.StatusBadRequest, recorder.Code)
			require.NotContains(t, recorder.Body.String(), "private-content")
			require.Zero(t, contacted.Load(), "non-public target must never be contacted")
			require.Zero(t, drainArchiveTasks(), "refused target must not be archived")
		})
	}
}

// fixturePeerConn reports a synthetic public peer for a connection to a local fixture.
type fixturePeerConn struct {
	net.Conn
	peer net.Addr
}

// RemoteAddr returns the synthetic public peer.
func (c *fixturePeerConn) RemoteAddr() net.Addr { return c.peer }

// usePublicTargetFixture makes public.example resolve to a synthetic public
// address whose pinned connections reach server, for both handler admission
// and the direct-fetch client, and restores the production network afterwards.
func usePublicTargetFixture(t *testing.T, server *httptest.Server) {
	t.Helper()
	deps := egress.Dependencies{
		Resolve: func(_ context.Context, host string) ([]net.IPAddr, error) {
			if host != "public.example" {
				return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
			}
			return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
		},
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			var dialer net.Dialer
			conn, err := dialer.DialContext(ctx, network, server.Listener.Addr().String())
			if err != nil {
				return nil, errors.WithStack(err)
			}
			return &fixturePeerConn{Conn: conn, peer: &net.TCPAddr{IP: net.ParseIP("93.184.216.34"), Port: 80}}, nil
		},
	}
	oldNetwork, oldClient := targetNetwork, targetHTTPClient
	targetNetwork, targetHTTPClient = deps, egress.NewHTTPClient(5*time.Second, deps)
	t.Cleanup(func() { targetNetwork, targetHTTPClient = oldNetwork, oldClient })
}

// TestCacheHandlerServesPublicTarget preserves the existing live-fetch response for public targets.
func TestCacheHandlerServesPublicTarget(t *testing.T) {
	useArchiveTestConfig()
	public := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "public.example", r.Host)
		if _, err := io.WriteString(w, "public-content"); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(public.Close)
	usePublicTargetFixture(t, public)

	recorder := serveCacheHandler("http://public.example/page")
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "DYNAMIC", recorder.Header().Get("X-Laisky-Cache-Status"))
	require.Equal(t, "public-content", recorder.Body.String())
}

// TestCacheHandlerRefusesRedirectIntoPrivateNetwork re-admits redirects on the
// direct-fetch path: an admitted public origin cannot bounce the proxy into a
// private address, and the refusal is answered as a 400 input error.
func TestCacheHandlerRefusesRedirectIntoPrivateNetwork(t *testing.T) {
	useArchiveTestConfig()
	var privateRequests atomic.Int32
	private := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		privateRequests.Add(1)
		if _, err := io.WriteString(w, "private-content"); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(private.Close)
	public := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, private.URL+"/metadata", http.StatusFound)
	}))
	t.Cleanup(public.Close)
	usePublicTargetFixture(t, public)

	recorder := serveCacheHandler("http://public.example/start")
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), string(egress.ReasonDestinationDenied))
	require.NotContains(t, recorder.Body.String(), "private-content")
	require.Zero(t, privateRequests.Load(), "redirect target must never be contacted")
	drainArchiveTasks()
}
