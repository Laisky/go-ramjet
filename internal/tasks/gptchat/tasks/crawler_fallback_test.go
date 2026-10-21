package tasks

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/Laisky/laisky-blog-graphql/library/crawleregress"
	rlibs "github.com/Laisky/laisky-blog-graphql/library/db/redis"
	"github.com/stretchr/testify/require"
)

// fallbackTestTask reads the additive capability through the real queue decoder.
// Older decoders ignore it, which reproduces the original direct-only behavior.
func fallbackTestTask(t *testing.T, enabled bool) *rlibs.HTMLCrawlerTask {
	t.Helper()
	task := verifiedTestTask()
	task.Url = "http://example.com/document"
	payload, err := task.ToString()
	require.NoError(t, err)
	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(payload), &fields))
	if enabled {
		fields["allow_provider_fallback"] = json.RawMessage("true")
	}
	encoded, err := json.Marshal(fields)
	require.NoError(t, err)
	decoded, err := rlibs.NewHTMLCrawlerTaskFromString(string(encoded))
	require.NoError(t, err)
	return decoded
}

// captureFallbackPublication installs an owned queue/result fixture and returns
// its terminal snapshots so tests assert the externally visible worker behavior.
func captureFallbackPublication(t *testing.T) *[]*rlibs.HTMLCrawlerTask {
	t.Helper()
	states := []*rlibs.HTMLCrawlerTask{}
	publishHTMLCrawlerTask = func(_ context.Context, task *rlibs.HTMLCrawlerTask) error {
		states = append(states, wireRoundTripVerifiedTask(t, task))
		return nil
	}
	persistVerifiedCrawlRecord = func(context.Context, *CrawlRecord) error { return nil }
	return &states
}

// TestCrawlerContentFirstChallenge reproduces issue #94 with an actual HTTP 403
// fixture and checks useful 403 content, quoted markers, and old caller controls.
func TestCrawlerContentFirstChallenge(t *testing.T) {
	for _, tc := range []struct {
		name, content string
		capability    bool
		fallback      bool
		success       bool
	}{
		{"challenge_falls_back", `<html><body><div id="cf-chl-widget"></div></body></html>`, true, true, true},
		{"ordinary_403_stays_direct", `<html><body>ordinary response</body></html>`, true, false, true},
		{"quoted_challenge_stays_direct", `<html><body><pre>cf-chl-widget _cf_chl_opt</pre></body></html>`, true, false, true},
		{"old_caller_stays_direct_only", `<html><body><div id="cf-chl-widget"></div></body></html>`, false, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			preserveVerifiedWorkerSeams(t)
			var directCalls, providerCalls atomic.Int32
			fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Empty(t, r.Header.Get("Authorization"))
				require.Empty(t, r.Header.Get("Cookie"))
				w.Header().Set("Content-Type", "text/html")
				w.WriteHeader(http.StatusForbidden)
				_, err := w.Write([]byte(tc.content))
				require.NoError(t, err)
			}))
			t.Cleanup(fixture.Close)
			deps := rendererTestDependencies(t, fixture, "8.8.8.8", &directCalls)
			fetchVerifiedHTMLContent = func(ctx context.Context, task *rlibs.HTMLCrawlerTask) (string, *crawleregress.Receipt, error) {
				require.Empty(t, task.APIKey)
				fetcher, err := newRendererEgressFetcher(ctx, task.TaskID, task.Url, *task.Egress, deps)
				if err != nil {
					return "", nil, err
				}
				_, body, err := fetcher.fetch(ctx, "document", "", task.Url, crawleregress.OriginDocument, http.MethodGet, nil)
				if err != nil {
					return "", nil, err
				}
				receipt, err := fetcher.seal(ctx)
				return string(body), receipt, err
			}
			webFetchProxies = []webFetchProxy{countingVerifiedProxy{calls: &providerCalls}}
			states := captureFallbackPublication(t)
			require.NoError(t, processHTMLCrawlerTask(context.Background(), fallbackTestTask(t, tc.capability)))
			require.Len(t, *states, 2)
			result := (*states)[1]
			require.EqualValues(t, 1, directCalls.Load())
			if !tc.success {
				require.Equal(t, rlibs.TaskStatusFailed, result.Status)
				require.Zero(t, providerCalls.Load())
				return
			}
			require.Equal(t, rlibs.TaskStatusSuccess, result.Status)
			if tc.fallback {
				require.EqualValues(t, 1, providerCalls.Load())
				require.Equal(t, "external-result", string(result.ResultMarkdown))
				require.Nil(t, result.EgressReceipt, "provider bytes are not verified direct content")
				require.Empty(t, result.RequestChain)
				payload, err := result.ToString()
				require.NoError(t, err)
				var envelope map[string]any
				require.NoError(t, json.Unmarshal([]byte(payload), &envelope))
				require.Equal(t, map[string]any{"method": "provider", "provider": "external-canary"}, envelope["acquisition"])
			} else {
				require.Zero(t, providerCalls.Load())
				require.NotEmpty(t, result.ResultMarkdown)
				require.NoError(t, crawleregress.VerifyReceipt(context.Background(), result.TaskID, result.Url, *result.Egress, result.EgressReceipt))
			}
		})
	}
}
