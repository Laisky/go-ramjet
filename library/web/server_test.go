package web

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestNormalizeHandlerDuplicatePrefix verifies duplicated task prefixes are normalized.
func TestNormalizeHandlerDuplicatePrefix(t *testing.T) {
	var gotPath string
	h := &normalizeHandler{handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	})}

	req := httptest.NewRequest(http.MethodGet, "http://example.com/gptchat/gptchat/favicon.ico", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "/gptchat/favicon.ico", gotPath)
}

func TestSecurityMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(securityMiddleware)
	r.GET("/test", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
	require.Equal(t, "SAMEORIGIN", w.Header().Get("X-Frame-Options"))
	require.Equal(t, "strict-origin-when-cross-origin", w.Header().Get("Referrer-Policy"))
}

// TestServeUntilCanceledStopsAdmission verifies graceful cancellation without production dependencies.
func TestServeUntilCanceledStopsAdmission(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	address := listener.Addr().String()
	require.NoError(t, listener.Close())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := &http.Server{Addr: address, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })}
	done := make(chan error, 1)
	go func() { done <- serveUntilCanceled(ctx, server) }()
	client := &http.Client{Timeout: 100 * time.Millisecond}
	require.Eventually(t, func() bool {
		response, err := client.Get("http://" + address)
		if err != nil {
			return false
		}
		defer response.Body.Close()
		return response.StatusCode == http.StatusNoContent
	}, time.Second, 10*time.Millisecond)
	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("HTTP cancellation did not stop server")
	}
	_, err = client.Get("http://" + address)
	require.Error(t, err)
}

// TestServeUntilCanceledDrainsActiveRequest preserves an accepted response while stopping new admission.
func TestServeUntilCanceledDrainsActiveRequest(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	address := listener.Addr().String()
	require.NoError(t, listener.Close())
	entered := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := &http.Server{Addr: address, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-release
		w.WriteHeader(http.StatusAccepted)
	})}
	done := make(chan error, 1)
	go func() { done <- serveUntilCanceled(ctx, server) }()
	client := &http.Client{Timeout: 2 * time.Second}
	response := make(chan int, 1)
	go func() {
		var finalErr error
		for range 30 {
			res, requestErr := client.Get("http://" + address)
			if requestErr != nil {
				finalErr = requestErr
				time.Sleep(10 * time.Millisecond)
				continue
			}
			res.Body.Close()
			response <- res.StatusCode
			return
		}
		_ = finalErr
		response <- 0
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("request did not reach server")
	}
	cancel()
	require.Eventually(t, func() bool {
		connection, err := net.DialTimeout("tcp", address, 50*time.Millisecond)
		if err != nil {
			return true
		}
		connection.Close()
		return false
	}, time.Second, 10*time.Millisecond)
	select {
	case <-done:
		t.Fatal("server returned before accepted request drained")
	default:
	}
	unblock()
	select {
	case status := <-response:
		require.Equal(t, http.StatusAccepted, status)
	case <-time.After(time.Second):
		t.Fatal("accepted response lost")
	}
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("server failed to finish draining")
	}
}

// TestServeUntilCanceledReturnsListenerFailure preserves nonzero root RunE error behavior.
func TestServeUntilCanceledReturnsListenerFailure(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	server := &http.Server{Addr: listener.Addr().String()}
	err = serveUntilCanceled(context.Background(), server)
	require.Error(t, err)
	require.Contains(t, err.Error(), "serve HTTP")
}
