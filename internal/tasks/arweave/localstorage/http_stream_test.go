package localstorage

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// errSyntheticTruncation is the read failure of an origin that drops mid-transfer.
var errSyntheticTruncation = errors.New("synthetic origin truncation")

// truncatingBody yields its content and then fails instead of reaching EOF.
type truncatingBody struct{ content *strings.Reader }

// Read returns content and then errSyntheticTruncation.
func (b *truncatingBody) Read(p []byte) (int, error) {
	n, err := b.content.Read(p)
	if errors.Is(err, io.EOF) {
		return n, errSyntheticTruncation
	}
	return n, err
}

// Close always succeeds.
func (b *truncatingBody) Close() error { return nil }

// newProxyTestServer serves CacheHandler behind the production recovery middleware,
// over HTTP/1.1 or, when http2 is set, over TLS with HTTP/2.
func newProxyTestServer(t *testing.T, http2 bool) *httptest.Server {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(gin.Recovery())
	router.GET("/proxy", CacheHandler)
	server := httptest.NewUnstartedServer(router)
	if http2 {
		server.EnableHTTP2 = true
		server.StartTLS()
	} else {
		server.Start()
	}
	t.Cleanup(server.Close)
	return server
}

// TestCacheHandlerSignalsTruncatedStream guards against answering a transfer that
// failed after the status was committed as if it had succeeded: the old handler
// appended a JSON error document to the partial body and let the response end
// normally. The client must instead see the transfer fail, and the partial body
// must never be followed by an error document.
func TestCacheHandlerSignalsTruncatedStream(t *testing.T) {
	for _, tc := range []struct {
		name     string
		size     int
		http2    bool
		streamed bool
	}{
		{"http1 buffered", 1000, false, false},
		{"http1 streamed", 256 << 10, false, true},
		{"http2 streamed", 256 << 10, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			content := "<html>" + strings.Repeat("x", tc.size)
			useArchiveSeams(t, notArchived, nil, nil)
			useProxySources(t, func(context.Context, string) (io.ReadCloser, error) {
				return &truncatingBody{content: strings.NewReader(content)}, nil
			}, failingLoader(errors.New("archive miss")), 0)
			server := newProxyTestServer(t, tc.http2)

			ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
			defer cancel()
			request, err := http.NewRequestWithContext(ctx, http.MethodGet,
				server.URL+"/proxy?url="+url.QueryEscape("http://public.example/page"), nil)
			require.NoError(t, err)
			response, err := server.Client().Do(request)
			if !tc.streamed && err != nil {
				return // the buffered response was aborted before reaching the client
			}
			require.NoError(t, err, "a streamed response has already sent its headers")
			defer response.Body.Close()
			received, readErr := io.ReadAll(response.Body)
			require.NotContains(t, string(received), `{"err"`, "an error document must not be appended to the body")
			require.Error(t, readErr, "a truncated transfer must not look complete; got %d of %d bytes with status %d",
				len(received), len(content), response.StatusCode)
		})
	}
}
