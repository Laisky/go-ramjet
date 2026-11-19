package ario

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// errGatewayTruncation is the read failure of a gateway that drops mid-transfer.
var errGatewayTruncation = errors.New("synthetic gateway truncation")

// truncatingGatewayBody yields its content and then fails instead of reaching EOF.
type truncatingGatewayBody struct{ content *strings.Reader }

// Read returns content and then errGatewayTruncation.
func (b *truncatingGatewayBody) Read(p []byte) (int, error) {
	n, err := b.content.Read(p)
	if errors.Is(err, io.EOF) {
		return n, errGatewayTruncation
	}
	return n, err
}

// Close always succeeds.
func (b *truncatingGatewayBody) Close() error { return nil }

// newGatewayTestServer serves GatewayHandler behind the production recovery
// middleware, over HTTP/1.1 or, when http2 is set, over TLS with HTTP/2.
func newGatewayTestServer(t *testing.T, http2 bool) *httptest.Server {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(gin.Recovery())
	router.Any("/gateway/*fileKey", GatewayHandler)
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

// TestGatewayHandlerSignalsTruncatedStream guards against answering a gateway
// transfer that failed after the status was committed as if it had succeeded:
// appending a JSON error document to the partial body makes a truncated file
// look complete. The client must see the transfer fail instead, and the partial
// body must never be followed by an error document.
func TestGatewayHandlerSignalsTruncatedStream(t *testing.T) {
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
			content := strings.Repeat("x", tc.size)
			useGatewayEgress(t, gatewayRoundTripper(func(*http.Request) (*http.Response, error) {
				return nil, errors.New("egress must not be used")
			}))
			useGateways(t, &http.Client{Transport: gatewayRoundTripper(func(request *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header: http.Header{
						"X-Ar-Io-Hops": []string{"1"},
						"Content-Type": []string{"application/octet-stream"},
					},
					Body:    &truncatingGatewayBody{content: strings.NewReader(content)},
					Request: request,
				}, nil
			})}, "https://gateway.example/")
			server := newGatewayTestServer(t, tc.http2)

			ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
			defer cancel()
			request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/gateway/"+testFileKey, nil)
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

// TestGatewayHandlerFailureBeforeBodyIsSelfDescribing covers a gateway that
// fails before its first body byte: the error document must not go out under
// the gateway's length, coding, validators and immutable caching headers.
func TestGatewayHandlerFailureBeforeBodyIsSelfDescribing(t *testing.T) {
	useGatewayEgress(t, gatewayRoundTripper(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("egress must not be used")
	}))
	useGateways(t, &http.Client{Transport: gatewayRoundTripper(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"X-Ar-Io-Hops":     []string{"1"},
				"Content-Type":     []string{"text/html"},
				"Content-Length":   []string{"4096"},
				"Content-Encoding": []string{"br"},
				"Cache-Control":    []string{"public, max-age=2592000, immutable"},
				"Etag":             []string{"abc"},
			},
			Body:    &truncatingGatewayBody{content: strings.NewReader("")},
			Request: request,
		}, nil
	})}, "https://gateway.example/")
	server := newGatewayTestServer(t, false)

	response, err := server.Client().Get(server.URL + "/gateway/" + testFileKey)
	require.NoError(t, err)
	body, err := io.ReadAll(response.Body)
	require.NoError(t, errors.Join(err, response.Body.Close()), "the error document must be complete")
	require.Equal(t, http.StatusBadRequest, response.StatusCode)
	require.Contains(t, string(body), `{"err"`)
	require.Equal(t, "application/json; charset=utf-8", response.Header.Get("Content-Type"))
	require.Equal(t, "no-store", response.Header.Get("Cache-Control"))
	require.Empty(t, response.Header.Get("Content-Encoding"))
	require.Empty(t, response.Header.Get("Etag"))
}
