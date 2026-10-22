package tasks

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/Laisky/laisky-blog-graphql/library/crawleregress"
	rlibs "github.com/Laisky/laisky-blog-graphql/library/db/redis"
	"github.com/stretchr/testify/require"
)

// TestCrawlerProviderQueryRequiresExplicitPolicy reproduces accidental
// disclosure of a target's opaque query values to every configured provider.
// It intentionally uses unrecognizable parameter names: name heuristics are
// not a security boundary. The policy applies only to verified fallback.
func TestCrawlerProviderQueryRequiresExplicitPolicy(t *testing.T) {
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
