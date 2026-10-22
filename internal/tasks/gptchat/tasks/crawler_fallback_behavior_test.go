package tasks

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/laisky-blog-graphql/library/crawleregress"
	rlibs "github.com/Laisky/laisky-blog-graphql/library/db/redis"
	"github.com/stretchr/testify/require"
)

// fallbackTestProxy supplies deterministic provider behavior without network I/O.
type fallbackTestProxy struct {
	name     string
	priority int
	fetch    func(context.Context, string) (string, error)
}

// Name returns the fixture provider's diagnostic identifier.
func (p fallbackTestProxy) Name() string { return p.name }

// Priority returns the fixture's configured priority.
func (p fallbackTestProxy) Priority() int { return p.priority }

// Fetch executes the fixture behavior with the actual target and worker context.
func (p fallbackTestProxy) Fetch(ctx context.Context, target string) (string, error) {
	return p.fetch(ctx, target)
}

// TestCrawlerFallbackEligibleFailures covers transport, child timeout and empty
// or unconvertible direct content without extending the caller's time budget.
func TestCrawlerFallbackEligibleFailures(t *testing.T) {
	for _, tc := range []struct {
		name, content string
		renderErr     error
		convertErr    bool
	}{
		{"network", "", &net.OpError{Op: "read", Net: "tcp", Err: errors.New("connection reset")}, false},
		{"direct_child_timeout", "", context.DeadlineExceeded, false},
		{"empty_body", " \n ", nil, false},
		{"empty_markdown", "<body></body>", nil, false},
		{"conversion_error", "<body>usable source</body>", nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			preserveVerifiedWorkerSeams(t)
			task := fallbackTestTask(t, true)
			fetchVerifiedHTMLContent = func(_ context.Context, direct *rlibs.HTMLCrawlerTask) (string, *crawleregress.Receipt, error) {
				return tc.content, verifiedTestReceipt(direct), tc.renderErr
			}
			if tc.convertErr {
				convertVerifiedMarkdown = func(string) (string, error) { return "", errors.New("local converter failed") }
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			t.Cleanup(cancel)
			deadline, ok := ctx.Deadline()
			require.True(t, ok)
			calls := 0
			webFetchProxies = []webFetchProxy{fallbackTestProxy{name: "fixture", fetch: func(providerCtx context.Context, target string) (string, error) {
				calls++
				actualDeadline, ok := providerCtx.Deadline()
				require.True(t, ok)
				require.Equal(t, deadline, actualDeadline)
				require.Equal(t, task.Url, target)
				return "# Provider content\n\nPreserved **Markdown**.", nil
			}}}
			result, err := acquireCrawlerContent(ctx, task)
			require.NoError(t, err)
			require.Equal(t, 1, calls)
			require.Equal(t, "# Provider content\n\nPreserved **Markdown**.", result.markdown)
			require.Contains(t, string(result.body), "<strong>Markdown</strong>")
			require.Nil(t, result.receipt)
			require.Equal(t, "fixture", result.acquisition.Provider)
		})
	}
}

// TestCrawlerFallbackStopsAtParentCancellation rejects canceled, expired and
// late provider results, including a provider that ignores context cancellation.
func TestCrawlerFallbackStopsAtParentCancellation(t *testing.T) {
	for _, mode := range []string{"before", "expired", "during", "late_success"} {
		t.Run(mode, func(t *testing.T) {
			preserveVerifiedWorkerSeams(t)
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			if mode == "before" {
				cancel()
			} else if mode == "expired" {
				var expire context.CancelFunc
				ctx, expire = context.WithDeadline(ctx, time.Now().Add(-time.Second))
				t.Cleanup(expire)
			}
			fetchVerifiedHTMLContent = func(_ context.Context, task *rlibs.HTMLCrawlerTask) (string, *crawleregress.Receipt, error) {
				return `<body><div id="cf-chl-widget"></div></body>`, verifiedTestReceipt(task), nil
			}
			calls := 0
			webFetchProxies = []webFetchProxy{
				fallbackTestProxy{name: "first", priority: 2, fetch: func(providerCtx context.Context, _ string) (string, error) {
					calls++
					cancel()
					if mode == "late_success" {
						return "late bytes must not succeed", nil
					}
					return "", providerCtx.Err()
				}},
				fallbackTestProxy{name: "second", priority: 1, fetch: func(context.Context, string) (string, error) {
					t.Fatal("cancellation must stop the next provider")
					return "", nil
				}},
			}
			result, err := acquireCrawlerContent(ctx, fallbackTestTask(t, true))
			require.Error(t, err)
			require.Nil(t, result)
			if mode == "expired" {
				require.ErrorIs(t, err, context.DeadlineExceeded)
			} else {
				require.ErrorIs(t, err, context.Canceled)
			}
			if mode == "before" || mode == "expired" {
				require.Zero(t, calls)
			} else {
				require.Equal(t, 1, calls)
			}
		})
	}
}

// TestCrawlerProviderContentAcceptance retries unusable provider results in
// configured order and returns a valid local HTML representation when requested.
func TestCrawlerProviderContentAcceptance(t *testing.T) {
	for _, markdown := range []bool{true, false} {
		t.Run(map[bool]string{true: "markdown", false: "html"}[markdown], func(t *testing.T) {
			preserveVerifiedWorkerSeams(t)
			task := fallbackTestTask(t, true)
			task.OutputMarkdown = markdown
			fetchVerifiedHTMLContent = func(_ context.Context, direct *rlibs.HTMLCrawlerTask) (string, *crawleregress.Receipt, error) {
				return "", verifiedTestReceipt(direct), nil
			}
			results := []string{" \n", `<html><body><div id="cf-chl-widget"></div></body></html>`,
				"Title: Just a moment\nURL Source: https://example.com/\nWarning: Target URL returned error 403: Forbidden\nMarkdown Content:\nEnable JavaScript",
				"# An article about HTTP 403\n\nUseful **content**."}
			calls := []int{}
			for i, content := range results {
				webFetchProxies = append(webFetchProxies, fallbackTestProxy{name: "configured", priority: len(results) - i,
					fetch: func(context.Context, string) (string, error) { calls = append(calls, i); return content, nil }})
			}
			result, err := acquireCrawlerContent(context.Background(), task)
			require.NoError(t, err)
			require.Equal(t, []int{0, 1, 2, 3}, calls)
			require.Contains(t, string(result.body), "<h1>An article about HTTP 403</h1>")
			if markdown {
				require.Equal(t, results[3], result.markdown)
			} else {
				require.Empty(t, result.markdown)
			}
		})
	}
}

// TestCrawlerProviderExhaustionKeepsSafeDiagnostics retains failed provider
// identities without returning provider response bodies or credential canaries.
func TestCrawlerProviderExhaustionKeepsSafeDiagnostics(t *testing.T) {
	preserveVerifiedWorkerSeams(t)
	fetchVerifiedHTMLContent = func(_ context.Context, task *rlibs.HTMLCrawlerTask) (string, *crawleregress.Receipt, error) {
		return "", verifiedTestReceipt(task), nil
	}
	webFetchProxies = []webFetchProxy{
		fallbackTestProxy{name: "jina", priority: 2, fetch: func(context.Context, string) (string, error) {
			return "", errors.New("provider error body=private-provider-body-canary api_key=private-key-canary")
		}},
		fallbackTestProxy{name: "defuddle", priority: 1, fetch: func(context.Context, string) (string, error) { return " ", nil }},
	}
	result, err := acquireCrawlerContent(context.Background(), fallbackTestTask(t, true))
	require.Error(t, err)
	require.Nil(t, result)
	for _, name := range []string{"jina", "defuddle", "request_failed", "unusable_content"} {
		require.Contains(t, err.Error(), name)
	}
	for _, secret := range []string{"private-provider-body-canary", "private-key-canary"} {
		require.NotContains(t, err.Error(), secret)
	}
}

// TestCrawlerFallbackNeverOverridesEnforcement exercises the actual private
// redirect guard and retains receipt, initial-admission and resource boundaries.
func TestCrawlerFallbackNeverOverridesEnforcement(t *testing.T) {
	for _, mode := range []string{"private_redirect", "private_target", "missing_receipt", "oversized_direct", "budget"} {
		t.Run(mode, func(t *testing.T) {
			preserveVerifiedWorkerSeams(t)
			task := fallbackTestTask(t, true)
			var calls atomic.Int32
			webFetchProxies = []webFetchProxy{countingVerifiedProxy{calls: &calls}}
			if mode == "private_target" {
				task.Egress.Addresses = []string{"127.0.0.1"}
			}
			fetchVerifiedHTMLContent = func(ctx context.Context, direct *rlibs.HTMLCrawlerTask) (string, *crawleregress.Receipt, error) {
				switch mode {
				case "missing_receipt":
					return "<body>unverified</body>", nil, nil
				case "oversized_direct":
					return strings.Repeat("x", rendererMaxResponseBytes+1), verifiedTestReceipt(direct), nil
				case "budget":
					fetcher, err := newRendererEgressFetcher(ctx, direct.TaskID, direct.Url, *direct.Egress, rendererNetworkDependencies{})
					require.NoError(t, err)
					fetcher.requests = rendererMaxRequests
					_, _, err = fetcher.fetch(ctx, "document", "", direct.Url, crawleregress.OriginDocument, http.MethodGet, nil)
					return "", nil, err
				case "private_redirect":
					var hits atomic.Int32
					fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
						w.Header().Set("Location", "http://127.0.0.1/private")
						w.WriteHeader(http.StatusFound)
					}))
					defer fixture.Close()
					fetcher, err := newRendererEgressFetcher(ctx, direct.TaskID, direct.Url, *direct.Egress,
						rendererTestDependencies(t, fixture, "8.8.8.8", &hits))
					require.NoError(t, err)
					response, _, err := fetcher.fetch(ctx, "document", "", direct.Url, crawleregress.OriginDocument, http.MethodGet, nil)
					require.NoError(t, err)
					_, _, err = fetcher.fetch(ctx, "redirect", "document", response.Header.Get("Location"), crawleregress.OriginDocument, http.MethodGet, nil)
					require.Error(t, err)
					require.EqualValues(t, 1, hits.Load(), "private redirect is rejected before dialing")
					return "", nil, err
				default:
					t.Fatal("private initial admission must precede rendering")
					return "", nil, nil
				}
			}
			result, err := acquireCrawlerContent(context.Background(), task)
			require.Error(t, err)
			require.Nil(t, result)
			require.Zero(t, calls.Load())
		})
	}
}
