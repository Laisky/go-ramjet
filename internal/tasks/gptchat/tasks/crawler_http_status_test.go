package tasks

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/laisky-blog-graphql/library/crawleregress"
	"github.com/stretchr/testify/require"
)

// TestRendererDocumentHTTPStatusEvidence retains only a proven document response status when response decoding fails.
func TestRendererDocumentHTTPStatusEvidence(t *testing.T) {
	for _, kind := range []string{crawleregress.OriginDocument, crawleregress.OriginSubresource} {
		t.Run(kind, func(t *testing.T) {
			fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Encoding", "gzip")
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte("private-upstream-body-canary"))
			}))
			t.Cleanup(fixture.Close)
			var hits atomic.Int32
			deps := rendererTestDependencies(t, fixture, "93.184.216.34", &hits)
			policy := rendererTestPolicy("example.com", "93.184.216.34")
			policy.AllowSubresources = true
			fetcher, err := newRendererEgressFetcher(context.Background(), "37b25fb8-4974-4a11-a5f5-b2f6c296025c", "http://example.com/document", policy, deps)
			require.NoError(t, err)
			response, body, err := fetcher.fetch(context.Background(), "request", "", "http://example.com/document", kind, http.MethodGet, nil)
			require.Error(t, err)
			require.Nil(t, response)
			require.Nil(t, body)
			var observed interface{ UpstreamStatusCode() int }
			if kind == crawleregress.OriginDocument {
				require.ErrorAs(t, err, &observed, "document status remains available after decoding fails")
				require.Equal(t, http.StatusServiceUnavailable, observed.UpstreamStatusCode())
			} else {
				require.False(t, errors.As(err, &observed), "subresource status must not become document status")
			}
			require.NotContains(t, err.Error(), "private-upstream-body-canary")
			require.EqualValues(t, 1, hits.Load())
		})
	}
}

// TestRendererHTTPErrorStatusDoesNotChangeAcceptance keeps ordinary non-2xx response behavior unchanged.
func TestRendererHTTPErrorStatusDoesNotChangeAcceptance(t *testing.T) {
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("<html><body>ordinary response</body></html>"))
	}))
	t.Cleanup(fixture.Close)
	var hits atomic.Int32
	deps := rendererTestDependencies(t, fixture, "93.184.216.34", &hits)
	fetcher, err := newRendererEgressFetcher(context.Background(), "37b25fb8-4974-4a11-a5f5-b2f6c296025c", "http://example.com/document", rendererTestPolicy("example.com", "93.184.216.34"), deps)
	require.NoError(t, err)
	response, body, err := fetcher.fetch(context.Background(), "request", "", "http://example.com/document", crawleregress.OriginDocument, http.MethodGet, nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusForbidden, response.StatusCode)
	require.NotEmpty(t, body)
}
