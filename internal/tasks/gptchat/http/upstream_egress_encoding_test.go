package http

import (
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// gzipAlwaysService answers every request with a gzip-encoded body, whether or
// not the request asked for it, as some origins do, and records each request's
// Accept-Encoding.
type gzipAlwaysService struct {
	*httptest.Server
	mu        sync.Mutex
	encodings []string
}

// newGzipAlwaysService starts a fixture that answers every request with body gzip-encoded.
func newGzipAlwaysService(t *testing.T, body string) *gzipAlwaysService {
	t.Helper()
	var encoded bytes.Buffer
	writer := gzip.NewWriter(&encoded)
	_, err := writer.Write([]byte(body))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	service := &gzipAlwaysService{}
	service.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		service.mu.Lock()
		service.encodings = append(service.encodings, r.Header.Get("Accept-Encoding"))
		service.mu.Unlock()
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Content-Encoding", "gzip")
		_, _ = w.Write(encoded.Bytes())
	}))
	t.Cleanup(service.Close)
	return service
}

// acceptEncodings returns the Accept-Encoding of every request received so far.
func (s *gzipAlwaysService) acceptEncodings() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.encodings...)
}

// TestTenantEgressNegotiatesAndDecodesGzip keeps caller-selected requests on
// the server client's content-coding behavior: they advertise gzip, and a
// gzip body, even one an origin sends unasked, reaches the caller decoded.
// Both the pinned direct path and the operator-proxy path must agree.
func TestTenantEgressNegotiatesAndDecodesGzip(t *testing.T) {
	const plain = "public-notes in plain text"
	t.Run("pinned", func(t *testing.T) {
		provider := newGzipAlwaysService(t, plain)
		egressTestSetup(t, provider.Server)
		t.Setenv("HTTP_PROXY", "")
		require.NoError(t, SetupHTTPCli())

		content, err := fetchStaticURLContent(t.Context(), "http://"+egressProviderHost+"/notes.txt")
		require.NoError(t, err)
		require.Equal(t, plain, string(content))
		require.Equal(t, []string{"gzip"}, provider.acceptEncodings())
	})
	t.Run("proxied", func(t *testing.T) {
		direct := newInternalService(t, http.StatusOK, "direct")
		proxy := newGzipAlwaysService(t, plain)
		egressTestSetup(t, direct.Server)
		t.Setenv("HTTP_PROXY", proxy.URL)
		require.NoError(t, SetupHTTPCli())

		content, err := fetchStaticURLContent(t.Context(), "http://"+egressProviderHost+"/notes.txt")
		require.NoError(t, err)
		require.Equal(t, plain, string(content))
		require.Equal(t, []string{"gzip"}, proxy.acceptEncodings())
		require.Zero(t, direct.hits.Load())
	})
}
