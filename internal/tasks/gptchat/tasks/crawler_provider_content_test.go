package tasks

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"sync/atomic"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/laisky-blog-graphql/library/crawleregress"
	rlibs "github.com/Laisky/laisky-blog-graphql/library/db/redis"
	"github.com/stretchr/testify/require"
)

// TestCrawlerProviderMarkdownChallengeExamples distinguishes quoted challenge
// documentation from active challenge elements in provider content.
func TestCrawlerProviderMarkdownChallengeExamples(t *testing.T) {
	for _, example := range []string{
		"```html\n<div id=\"cf-chl-widget\"></div>\n```",
		"An inline `<div id=\"cf-chl-widget\"></div>` example.",
	} {
		content := fmt.Sprintf("# Challenge documentation\n\n%s\n\nUseful explanation.", example)
		result, err := prepareCrawlerProviderContent(context.Background(), content, true)
		require.NoError(t, err)
		require.Equal(t, content, result.markdown)
		require.Contains(t, string(result.body), "<code")
	}
	for _, content := range []string{
		`<body><div id="cf-chl-widget"></div></body>`,
		"# Just a moment\n\n<div id=\"cf-chl-widget\"></div>",
	} {
		result, err := prepareCrawlerProviderContent(context.Background(), content, true)
		require.Error(t, err)
		require.Nil(t, result)
	}
}

// TestCrawlerProviderHTMLFragments preserves provider HTML documents/fragments
// while returning actual Markdown or HTML for the requested representation.
func TestCrawlerProviderHTMLFragments(t *testing.T) {
	for _, content := range []string{
		"<article><h1>Article</h1><p>Useful body</p></article>",
		"<!-- source annotation -->\n<article><h1>Article</h1><p>Useful body</p></article>",
		"<html><body><h1>Article</h1><p>Useful body</p></body></html>",
	} {
		for _, markdown := range []bool{true, false} {
			result, err := prepareCrawlerProviderContent(context.Background(), content, markdown)
			require.NoError(t, err)
			require.Contains(t, string(result.body), "<h1>Article</h1>")
			if markdown {
				require.Contains(t, result.markdown, "# Article")
				require.NotContains(t, result.markdown, "<article>")
			}
		}
	}
	for _, content := range []string{"<!-- empty -->", "<p>  </p>", "<body>\n </body>"} {
		result, err := prepareCrawlerProviderContent(context.Background(), content, false)
		require.Error(t, err, content)
		require.Nil(t, result)
	}
	result, err := prepareCrawlerProviderContent(context.Background(), "# Article\n\nText with <em>inline emphasis</em>.", true)
	require.NoError(t, err)
	require.Contains(t, result.markdown, "Text with <em>inline emphasis</em>.")
}

// TestCrawlerEmptyDirectHTMLMode distinguishes empty structure from useful raw
// HTML such as images, without changing old direct-only callers' behavior.
func TestCrawlerEmptyDirectHTMLMode(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		fallback   bool
	}{
		{"empty", "<body></body>", true},
		{"whitespace", "<body>  \n </body>", true},
		{"image_only", `<body><img src="https://example.com/image.png" alt="Diagram"></body>`, false},
		{"canvas", `<body><canvas width="100" height="100"></canvas></body>`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			preserveVerifiedWorkerSeams(t)
			task := fallbackTestTask(t, true)
			task.OutputMarkdown = false
			fetchVerifiedHTMLContent = func(_ context.Context, task *rlibs.HTMLCrawlerTask) (string, *crawleregress.Receipt, error) {
				return tc.body, verifiedTestReceipt(task), nil
			}
			var calls atomic.Int32
			webFetchProxies = []webFetchProxy{countingVerifiedProxy{calls: &calls}}
			result, err := acquireCrawlerContent(context.Background(), task)
			require.NoError(t, err)
			if tc.fallback {
				require.EqualValues(t, 1, calls.Load())
				require.Contains(t, string(result.body), "external-result")
			} else {
				require.Zero(t, calls.Load())
				require.NotNil(t, result.receipt)
			}
		})
	}
}

// TestCrawlerProviderFailureDiagnostics preserves direct captures and safe
// provider outcomes even when the direct cause is a typed transport error.
func TestCrawlerProviderFailureDiagnostics(t *testing.T) {
	for _, mode := range []string{"provider_html", "direct_transport"} {
		t.Run(mode, func(t *testing.T) {
			preserveVerifiedWorkerSeams(t)
			output := crawlerWorkerLogBuffer(t)
			captureFallbackPublication(t)
			fetchVerifiedHTMLContent = func(_ context.Context, task *rlibs.HTMLCrawlerTask) (string, *crawleregress.Receipt, error) {
				if mode == "direct_transport" {
					return "", nil, &url.Error{Op: "Get", URL: task.Url, Err: &net.OpError{Op: "read", Err: errors.New("reset")}}
				}
				return "<body>direct-input-canary</body>", verifiedTestReceipt(task), nil
			}
			convertVerifiedMarkdown = func(string) (string, error) { return "", errors.New("fixture conversion failure") }
			webFetchProxies = []webFetchProxy{
				fallbackTestProxy{name: "jina", priority: 2, fetch: func(context.Context, string) (string, error) {
					return "<body>provider-payload-must-not-be-logged</body>", nil
				}},
				fallbackTestProxy{name: "defuddle", priority: 1, fetch: func(context.Context, string) (string, error) {
					return "", errors.New("body=provider-error-body-must-not-be-logged")
				}},
			}
			require.NoError(t, processHTMLCrawlerTask(context.Background(), fallbackTestTask(t, true)))
			records := crawlerWorkerFailureRecords(t, output)
			require.Len(t, records, 1)
			require.NotContains(t, output.String(), "provider-payload-must-not-be-logged")
			require.NotContains(t, output.String(), "provider-error-body-must-not-be-logged")
			require.Equal(t, []any{"jina:unusable_content", "defuddle:request_failed"}, records[0]["provider_attempts"])
			if mode == "provider_html" {
				require.Equal(t, "<body>direct-input-canary</body>", records[0]["conversion_input"])
			}
		})
	}
}
