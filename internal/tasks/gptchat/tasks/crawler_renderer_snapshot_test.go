package tasks

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/laisky-blog-graphql/library/crawleregress"
	rlibs "github.com/Laisky/laisky-blog-graphql/library/db/redis"
	"github.com/stretchr/testify/require"
)

// TestCrawlerRendererSnapshotSurvivesLateFailure exercises the same terminal renderer
// checks used after CDP observation, preserving bytes even when no HTML can be returned.
func TestCrawlerRendererSnapshotSurvivesLateFailure(t *testing.T) {
	for _, mode := range []string{"late_read_failure", "over_budget_snapshot"} {
		t.Run(mode, func(t *testing.T) {
			ctx, capture := withCrawlerFailureCapture(context.Background(), verifiedTestTask())
			document := "<html><body>observed before a later failure 雪</body></html>"
			var primary error
			if mode == "late_read_failure" {
				primary = errors.Wrap(io.ErrUnexpectedEOF, "synthetic later request")
			} else {
				document = "<html><body>" + strings.Repeat("x", crawleregress.MaxResponseBytes) + "</body></html>"
			}
			observeRendererDOMSnapshot(ctx, document, nil)
			if primary != nil {
				observeRendererDOMSnapshot(ctx, "", primary)
			}
			html, receipt, err := finishRendererSnapshot(ctx, document, primary, nil, nil, 0)
			require.Error(t, err)
			require.Empty(t, html, "rejected snapshots never bypass browser acceptance")
			require.Nil(t, receipt)
			if primary != nil {
				require.ErrorIs(t, err, io.ErrUnexpectedEOF)
			} else {
				require.Contains(t, err.Error(), "renderer DOM response budget exhausted")
			}
			logged := crawlerFieldsThroughJSONCore(t, capture.fields())
			require.Equal(t, true, logged["rendered_html_observed"])
			require.Equal(t, document, logged["rendered_html"])
			require.EqualValues(t, len(document), logged["rendered_html_bytes"])
			require.Equal(t, false, logged["conversion_input_observed"])
		})
	}
}

// TestRendererBrowserOverBudgetDOMStillLogsObservedSnapshot expands a small admitted
// page inside the actual browser and retains its full DOM while the existing limit rejects it.
func TestRendererBrowserOverBudgetDOMStillLogsObservedSnapshot(t *testing.T) {
	opts := rendererBrowserTestOptions(t)
	preserveVerifiedWorkerSeams(t)
	output := crawlerWorkerLogBuffer(t)
	var hits atomic.Int32
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, err := io.WriteString(w, "<html><body><script>document.body.textContent='x'.repeat("+strconv.Itoa(crawleregress.MaxResponseBytes+128)+");</script></body></html>")
		require.NoError(t, err)
	}))
	t.Cleanup(fixture.Close)
	deps := rendererTestDependencies(t, fixture, "93.184.216.34", &hits)
	fetchVerifiedHTMLContent = func(ctx context.Context, task *rlibs.HTMLCrawlerTask) (string, *crawleregress.Receipt, error) {
		return renderVerifiedHTMLWithDependencies(ctx, task, deps, opts)
	}
	publishHTMLCrawlerTask = func(context.Context, *rlibs.HTMLCrawlerTask) error { return nil }
	persistVerifiedCrawlRecord = func(context.Context, *CrawlRecord) error {
		t.Fatal("rejected DOM must not persist a success")
		return nil
	}
	convertVerifiedMarkdown = func(string) (string, error) { t.Fatal("rejected DOM must not reach conversion"); return "", nil }
	task := rendererBrowserTestTask("http://render.example/", false)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	require.NoError(t, processHTMLCrawlerTask(ctx, task))
	require.Equal(t, rlibs.TaskStatusFailed, task.Status)
	logged := crawlerWorkerFailureRecords(t, output)
	require.Len(t, logged, 1)
	require.Contains(t, logged[0]["error"], "renderer DOM response budget exhausted")
	require.Equal(t, true, logged[0]["rendered_html_observed"])
	document := logged[0]["rendered_html"].(string)
	require.Greater(t, len(document), crawleregress.MaxResponseBytes)
	require.Contains(t, document, strings.Repeat("x", 1024))
	require.EqualValues(t, len(document), logged[0]["rendered_html_bytes"])
	require.Equal(t, false, logged[0]["conversion_input_observed"])
	require.EqualValues(t, 1, hits.Load(), "observation does not refetch the target")
}
