package tasks

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/laisky-blog-graphql/library/crawleregress"
	rlibs "github.com/Laisky/laisky-blog-graphql/library/db/redis"
	"github.com/stretchr/testify/require"
)

// TestCrawlerAlertContextObservedDocument uses real pinned HTTP exchanges, not a
// fabricated receipt, and excludes subresources from final-document diagnostics.
func TestCrawlerAlertContextObservedDocument(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/private":
			w.Header().Set("Location", "https://example.com/final?opaque=private")
			w.WriteHeader(http.StatusFound)
		case "/subframe":
			w.Header().Set("Content-Type", "application/json; private=synthetic-only")
			w.WriteHeader(http.StatusNotFound)
			_, err := w.Write([]byte("subresource-only"))
			require.NoError(t, err)
		default:
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_, err := w.Write([]byte("<html><body><script>synthetic-private-body</script></body></html>"))
			require.NoError(t, err)
		}
	}))
	defer server.Close()
	renderer := func(ctx context.Context, task *rlibs.HTMLCrawlerTask) (string, *crawleregress.Receipt, error) {
		policy := crawleregress.ClonePolicy(*task.Egress)
		policy.AllowSubresources = true
		deps := rendererTestDependencies(t, server, "8.8.8.8", &hits)
		deps.rootCAs = x509.NewCertPool()
		deps.rootCAs.AddCert(server.Certificate())
		f, err := newRendererEgressFetcher(ctx, task.TaskID, task.Url, policy, deps)
		require.NoError(t, err)
		first, _, err := f.fetch(ctx, "first", "", task.Url, crawleregress.OriginDocument, http.MethodGet, nil)
		require.NoError(t, err)
		_, body, err := f.fetch(ctx, "final", "first", first.Header.Get("Location"), crawleregress.OriginDocument, http.MethodGet, nil)
		require.NoError(t, err)
		_, _, err = f.fetch(ctx, "subframe", "", "https://example.com/subframe", crawleregress.OriginSubresource, http.MethodGet, nil)
		require.NoError(t, err)
		receipt, err := f.seal(ctx)
		require.NoError(t, err)
		return string(body), receipt, nil
	}
	wire, message := crawlerAlertContextFixture(t, "", nil, true, renderer)
	var envelope struct {
		Context *crawlerFailureContext `json:"failure_context"`
	}
	require.NoError(t, json.Unmarshal([]byte(wire), &envelope))
	require.NotNil(t, envelope.Context)
	require.Equal(t, "https", envelope.Context.FinalScheme)
	require.Equal(t, "example.com", envelope.Context.FinalHostname)
	require.EqualValues(t, 200, *envelope.Context.UpstreamStatus)
	require.EqualValues(t, 1, *envelope.Context.RedirectCount)
	require.Equal(t, "text/html", envelope.Context.ContentType)
	require.Contains(t, message, `"upstream_status":200`)
	require.NotContains(t, message, "opaque=private")
	require.NotContains(t, message, "subresource-only")
	require.EqualValues(t, 3, hits.Load())
}

// TestCrawlerAlertContextBoundsAndSnapshots proves the aggregate wire budget
// covers every valid field simultaneously, including real overrun observations.
func TestCrawlerAlertContextBoundsAndSnapshots(t *testing.T) {
	host := strings.Repeat("a", 63) + "." + strings.Repeat("b", 63) + "." + strings.Repeat("c", 63) + "." + strings.Repeat("d", 61)
	require.Len(t, host, 253)
	max, redirect, status := int64(math.MaxInt64), int64(20), int64(599)
	value := &crawlerFailureContext{TargetScheme: "https", TargetHostname: host, FinalScheme: "https", FinalHostname: host,
		ContentType: "a/" + strings.Repeat("&", 125), Converter: "html-to-markdown/v2", UpstreamStatus: &status, RedirectCount: &redirect,
		RenderedHTMLBytes: &max, ConversionInputBytes: &max, ConversionOutputBytes: &max, ConversionOutputTrimmedBytes: &max, BodyTextRunes: &max, BodyTextNodes: &max}
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	require.LessOrEqual(t, len(raw), crawlerFailureContextMaxBytes)
	task := verifiedTestTask()
	task.Status = rlibs.TaskStatusFailed
	cause := crawlerFailure("empty_markdown", "conversion", nil)
	observed := withCrawlerObservedFailure(cause, value)
	*value.BodyTextRunes = 0
	snapshot := crawlerFailureContextFrom(observed)
	require.Equal(t, int64(math.MaxInt64), *snapshot.BodyTextRunes)
	*snapshot.BodyTextRunes = 1
	require.Equal(t, int64(math.MaxInt64), *crawlerFailureContextFrom(observed).BodyTextRunes)
	code, stage := crawlerDiagnostic(observed)
	require.Equal(t, "empty_markdown", code)
	require.Equal(t, "conversion", stage)
	require.ErrorIs(t, observed, cause)
	wire, err := encodeHTMLCrawlerTaskResult(withCrawlerFailure(context.Background(), observed), task)
	require.NoError(t, err)
	require.Contains(t, wire, `"body_text_runes":9223372036854775807`)
	require.Less(t, strings.Index(wire, `"failure_stage"`), strings.Index(wire, `"failure_context"`))
	require.Len(t, crawlerFailureContextFields(observed), 14)
	decoded, err := rlibs.NewHTMLCrawlerTaskFromString(wire)
	require.NoError(t, err)
	require.Equal(t, task.TaskID, decoded.TaskID)
	task.Status = rlibs.TaskStatusSuccess
	wire, err = encodeHTMLCrawlerTaskResult(withCrawlerFailure(context.Background(), observed), task)
	require.NoError(t, err)
	require.NotContains(t, wire, "failure_context")
}

// TestCrawlerAlertContextLateResponses cannot mutate a terminal observation,
// nor invent redirect depth when the previous document was not observed.
func TestCrawlerAlertContextLateResponses(t *testing.T) {
	ctx, holder := newCrawlerContextObservation(context.Background(), "https://EXAMPLE.COM.:443/p?secret=yes#fragment")
	observeCrawlerDocumentResponse(ctx, crawleregress.OriginDocument, "one", "unknown", "https://final.example/p?secret=yes", 200, "text/html; charset=utf-8")
	first := holder.snapshot()
	require.Equal(t, "example.com", first.TargetHostname)
	require.Nil(t, first.RedirectCount)
	holder.sealDocuments()
	observeCrawlerDocumentResponse(ctx, crawleregress.OriginDocument, "two", "", "https://wrong.example/", 503, "text/plain")
	require.Equal(t, first, holder.snapshot())
	err := withCrawlerObservedFailure(crawlerFailure("render_failed", "render", context.Canceled), first)
	require.ErrorIs(t, err, context.Canceled)
	var typed *crawlerTaskError
	require.True(t, errors.As(err, &typed))
	require.Equal(t, "canceled", typed.code)
}

// TestCrawlerAlertContextOriginAndMIME projects only the observed origin and
// parsed base media type; malformed metadata cannot add arbitrary alert text.
func TestCrawlerAlertContextOriginAndMIME(t *testing.T) {
	for _, tc := range []struct{ raw, scheme, host string }{
		{"https://EXAMPLE.COM.:443/private?token=secret#fragment", "https", "example.com"},
		{"https://user:pass@example.com/private", "", ""},
		{"https://bücher.example/private", "https", "bücher.example"},
		{"https://[2606:4700:4700:0:0:0:0:1111]/private", "https", "2606:4700:4700:0:0:0:0:1111"},
		{"https://[2606:4700:4700::1111]/path", "https", "2606:4700:4700::1111"},
		{"file:///private", "", ""},
		{"https://%zz/", "", ""},
	} {
		scheme, host := crawlerContextOrigin(tc.raw)
		require.Equal(t, tc.scheme, scheme)
		require.Equal(t, tc.host, host)
	}
	ctx, holder := newCrawlerContextObservation(context.Background(), "https://example.com/")
	observeCrawlerDocumentResponse(ctx, crawleregress.OriginSubresource, "sub", "", "https://wrong.example/", 404, "application/json")
	require.Empty(t, holder.snapshot().FinalHostname)
	observeCrawlerDocumentResponse(ctx, crawleregress.OriginDocument, "main", "", "https://example.com/", 200, "text/html; charset=utf-8")
	require.Equal(t, "text/html", holder.snapshot().ContentType)
	observeCrawlerDocumentResponse(ctx, crawleregress.OriginDocument, "bad", "", "https://example.com/", 200, "text/html; malformed=")
	require.Empty(t, holder.snapshot().ContentType)
	observeCrawlerDocumentResponse(ctx, crawleregress.OriginDocument, "token", "", "https://example.com/", 200, "bogus")
	require.Empty(t, holder.snapshot().ContentType)
	require.EqualValues(t, 200, *holder.snapshot().UpstreamStatus)
}

// TestCrawlerAlertContextFailedRedirectResponse retains the latest actual
// response when its body fails, rather than misattributing an earlier redirect.
func TestCrawlerAlertContextFailedRedirectResponse(t *testing.T) {
	preserveVerifiedWorkerSeams(t)
	var hits atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/private" {
			w.Header().Set("Location", "https://example.com/final")
			w.WriteHeader(http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Encoding", "gzip")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, err := w.Write([]byte("synthetic-not-gzip"))
		require.NoError(t, err)
	}))
	defer server.Close()
	fetchVerifiedHTMLContent = func(ctx context.Context, task *rlibs.HTMLCrawlerTask) (string, *crawleregress.Receipt, error) {
		deps := rendererTestDependencies(t, server, "8.8.8.8", &hits)
		deps.rootCAs = x509.NewCertPool()
		deps.rootCAs.AddCert(server.Certificate())
		f, err := newRendererEgressFetcher(ctx, task.TaskID, task.Url, *task.Egress, deps)
		require.NoError(t, err)
		response, _, err := f.fetch(ctx, "first", "", task.Url, crawleregress.OriginDocument, http.MethodGet, nil)
		require.NoError(t, err)
		_, _, err = f.fetch(ctx, "final", "first", response.Header.Get("Location"), crawleregress.OriginDocument, http.MethodGet, nil)
		return "", nil, err
	}
	_, _, _, err := fetchVerifiedCrawlerTask(context.Background(), verifiedTestTask())
	require.Error(t, err)
	contextValue := crawlerFailureContextFrom(err)
	require.NotNil(t, contextValue)
	require.EqualValues(t, 503, *contextValue.UpstreamStatus)
	require.EqualValues(t, 1, *contextValue.RedirectCount)
	require.Equal(t, "example.com", contextValue.FinalHostname)
	require.Equal(t, "text/plain", contextValue.ContentType)
	require.Nil(t, contextValue.RenderedHTMLBytes)
	require.Nil(t, contextValue.ConversionOutputBytes)
	require.Len(t, crawlerUpstreamStatusFields(err), 1)
	require.NotContains(t, err.Error(), "synthetic-not-gzip")
	code, stage := crawlerDiagnostic(err)
	require.Equal(t, "render_failed", code)
	require.Equal(t, "render", stage)
	require.EqualValues(t, 2, hits.Load())
}
