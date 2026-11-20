package web

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// newStreamAbortServer serves size bytes and then fails the stream through
// AbortStreamErr, behind the production recovery middleware.
func newStreamAbortServer(t *testing.T, size int, http2 bool) *httptest.Server {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(gin.Recovery())
	router.GET("/stream", func(c *gin.Context) {
		if _, err := io.WriteString(c.Writer, strings.Repeat("x", size)); err != nil {
			t.Error(err)
			return
		}
		AbortStreamErr(c, errors.New("upstream dropped"))
	})
	server := httptest.NewUnstartedServer(router)
	server.EnableHTTP2 = http2
	if http2 {
		server.StartTLS()
	} else {
		server.Start()
	}
	t.Cleanup(server.Close)
	return server
}

// TestAbortStreamErrBeforeCommitMatchesAbortErr keeps AbortErr's answer while
// nothing has been written: the status and error document are still sent.
func TestAbortStreamErrBeforeCommitMatchesAbortErr(t *testing.T) {
	t.Parallel()
	ctx, w, _, _ := newTestGinContext(t)
	require.False(t, AbortStreamErr(ctx, nil))
	require.False(t, ctx.IsAborted())

	require.True(t, AbortStreamErr(ctx, errors.New("boom")))
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.JSONEq(t, `{"err":"boom"}`, w.Body.String())
	require.True(t, ctx.IsAborted())
}

// TestAbortStreamErrAfterCommitAbortsTransfer requires a committed response to fail
// visibly on HTTP/1.1 and HTTP/2 instead of ending with an appended error document.
func TestAbortStreamErrAfterCommitAbortsTransfer(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		size    int
		http2   bool
		flushed bool
	}{
		{"http1 buffered", 100, false, false},
		{"http1 flushed", 128 << 10, false, true},
		{"http2 flushed", 128 << 10, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			server := newStreamAbortServer(t, tc.size, tc.http2)

			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/stream", nil)
			require.NoError(t, err)
			response, err := server.Client().Do(request)
			if !tc.flushed && err != nil {
				return // aborted before the buffered response reached the client
			}
			require.NoError(t, err, "a flushed response has already sent its headers")
			defer response.Body.Close()
			if tc.http2 {
				require.Equal(t, "HTTP/2.0", response.Proto)
			}
			received, readErr := io.ReadAll(response.Body)
			require.NotContains(t, string(received), "upstream dropped")
			require.Error(t, readErr, "got %d bytes with status %d", len(received), response.StatusCode)
		})
	}
}

// TestAbortStreamErrHTTP10KeepsBodyUncorrupted documents the HTTP/1.0 limit: a
// body delimited by closing the connection cannot signal truncation, so the
// abort reads as a normal end there. The body still holds only content and is
// never followed by an error document. Proxies in front of streamed relays
// should therefore speak HTTP/1.1 to this server.
func TestAbortStreamErrHTTP10KeepsBodyUncorrupted(t *testing.T) {
	t.Parallel()
	server := newStreamAbortServer(t, 128<<10, false)

	conn, err := net.DialTimeout("tcp", server.Listener.Addr().String(), 5*time.Second)
	require.NoError(t, err)
	defer conn.Close()
	require.NoError(t, conn.SetDeadline(time.Now().Add(10*time.Second)))
	_, err = io.WriteString(conn, "GET /stream HTTP/1.0\r\nHost: stream.test\r\n\r\n")
	require.NoError(t, err)
	response, err := http.ReadResponse(bufio.NewReader(conn), nil)
	require.NoError(t, err)
	defer response.Body.Close()
	received, err := io.ReadAll(response.Body)
	require.NoError(t, err, "HTTP/1.0 cannot tell an aborted body from a complete one")
	require.NotContains(t, string(received), "upstream dropped")
	require.Equal(t, strings.Repeat("x", len(received)), string(received), "the body holds only relayed content")
}

// TestAbortStreamErrBeforeCommitDropsRelayedRepresentation answers a relay that
// failed before its first body byte with a self-describing error: headers that
// described the relayed body (type, length, coding, validators, caching) must
// not label the error document, or a short Content-Length truncates it, a coding
// makes it undecodable and caching headers let caches keep it.
func TestAbortStreamErrBeforeCommitDropsRelayedRepresentation(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(gin.Recovery())
	router.GET("/relay", func(c *gin.Context) {
		for name, value := range map[string]string{
			"Content-Type": "text/html", "Content-Length": "4", "Content-Encoding": "gzip",
			"Content-Range": "bytes 0-3/10", "Content-Disposition": "attachment; filename=a.html",
			"Content-Language": "en", "Accept-Ranges": "bytes", "Cache-Control": "public, max-age=2592000, immutable",
			"Etag": `"abc"`, "Last-Modified": "Wed, 18 Nov 2026 00:00:00 GMT", "Expires": "Thu, 19 Nov 2026 00:00:00 GMT",
			"Content-Security-Policy": "sandbox", "X-Content-Type-Options": "nosniff",
		} {
			c.Header(name, value)
		}
		c.Status(http.StatusOK)
		AbortStreamErr(c, errors.New("upstream failed before the body"))
	})
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	response, err := server.Client().Get(server.URL + "/relay")
	require.NoError(t, err)
	body, err := io.ReadAll(response.Body)
	require.NoError(t, errors.Join(err, response.Body.Close()), "the error document must be complete")

	require.Equal(t, http.StatusBadRequest, response.StatusCode)
	require.JSONEq(t, `{"err":"upstream failed before the body"}`, string(body))
	require.Equal(t, "application/json; charset=utf-8", response.Header.Get("Content-Type"))
	require.Equal(t, "no-store", response.Header.Get("Cache-Control"))
	for _, name := range []string{"Content-Encoding", "Content-Range", "Content-Disposition", "Content-Language",
		"Accept-Ranges", "Etag", "Last-Modified", "Expires"} {
		require.Empty(t, response.Header.Values(name), name)
	}
	require.Equal(t, "sandbox", response.Header.Get("Content-Security-Policy"), "protective headers stay")
	require.Equal(t, "nosniff", response.Header.Get("X-Content-Type-Options"), "protective headers stay")
}
