package tasks

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/Laisky/laisky-blog-graphql/library/crawleregress"
	rlibs "github.com/Laisky/laisky-blog-graphql/library/db/redis"
	"github.com/stretchr/testify/require"

	iconfig "github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
)

// TestCrawlerProviderQueryRequiresExplicitPolicy reproduces accidental
// disclosure of a target's opaque query values to every configured provider.
// It intentionally uses unrecognizable parameter names: name heuristics are
// not a security boundary. The policy applies only to verified fallback.
func TestCrawlerProviderQueryRequiresExplicitPolicy(t *testing.T) {
	originalConfig := iconfig.Config
	iconfig.Config = nil // Assert the shipped no-override default, not another test's config.
	t.Cleanup(func() { iconfig.Config = originalConfig })
	for _, tc := range []struct {
		name, target string
		directOK     bool
		wantProvider bool
	}{
		{"opaque_query_is_not_forwarded", "http://example.com/article?customA=token-canary&view=one", false, false},
		{"empty_query_delimiter_is_not_forwarded", "http://example.com/article?", false, false},
		{"ordinary_url_remains_available", "http://example.com/article", false, true},
		{"query_url_still_works_directly", "http://example.com/article?customA=token-canary", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			preserveVerifiedWorkerSeams(t)
			task := fallbackTestTask(t, true)
			task.Url = tc.target
			fetchVerifiedHTMLContent = func(_ context.Context, direct *rlibs.HTMLCrawlerTask) (string, *crawleregress.Receipt, error) {
				if tc.directOK {
					return "<body>direct content</body>", verifiedTestReceipt(direct), nil
				}
				return "<body><div id=\"cf-chl-widget\"></div></body>", verifiedTestReceipt(direct), nil
			}
			var calls atomic.Int32
			webFetchProxies = []webFetchProxy{fallbackTestProxy{name: "fixture", fetch: func(_ context.Context, target string) (string, error) {
				calls.Add(1)
				require.Equal(t, tc.target, target, "explicitly permitted URLs must remain byte-for-byte intact")
				return "# Provider article", nil
			}}}
			result, err := acquireCrawlerContent(t.Context(), task)
			if tc.directOK {
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Equal(t, crawleregress.AcquisitionDirect, result.acquisition.Method)
				require.Zero(t, calls.Load())
				return
			}
			if tc.wantProvider {
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Equal(t, crawleregress.AcquisitionProvider, result.acquisition.Method)
				require.EqualValues(t, 1, calls.Load())
				return
			}
			require.Error(t, err, "direct challenge remains a failure when a private query cannot be forwarded")
			require.Nil(t, result)
			require.Zero(t, calls.Load(), "no provider may observe or receive the URL")
		})
	}
}

// TestCrawlerProviderQueryOperatorOptIn preserves exact-query retrieval where
// an operator explicitly accepts disclosure to the configured providers.
func TestCrawlerProviderQueryOperatorOptIn(t *testing.T) {
	preserveVerifiedWorkerSeams(t)
	originalConfig := iconfig.Config
	iconfig.Config = &iconfig.OpenAI{WebFetch: iconfig.WebFetchConfig{AllowQueryForwarding: true}}
	t.Cleanup(func() { iconfig.Config = originalConfig })

	const target = "http://example.com/article?customA=canary%2Btoken&view=one"
	task := fallbackTestTask(t, true)
	task.Url = target
	fetchVerifiedHTMLContent = func(_ context.Context, direct *rlibs.HTMLCrawlerTask) (string, *crawleregress.Receipt, error) {
		return "<body><div id=\"cf-chl-widget\"></div></body>", verifiedTestReceipt(direct), nil
	}
	calls := 0
	webFetchProxies = []webFetchProxy{fallbackTestProxy{name: "fixture", fetch: func(_ context.Context, received string) (string, error) {
		calls++
		require.Equal(t, target, received, "operator-authorized provider receives the exact original query")
		return "# Opted-in article", nil
	}}}
	result, err := acquireCrawlerContent(t.Context(), task)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 1, calls)
	require.Equal(t, crawleregress.AcquisitionProvider, result.acquisition.Method)
	require.Equal(t, "# Opted-in article", result.markdown)
	require.Nil(t, result.receipt)

	// Query authorization does not authorize userinfo or URL fragments.
	require.False(t, crawlerProviderURLPermitted("https://alice:password@example.com/article"))
	require.False(t, crawlerProviderURLPermitted("https://example.com/article#token-canary"))
	require.True(t, crawlerProviderURLPermitted(target))
}
