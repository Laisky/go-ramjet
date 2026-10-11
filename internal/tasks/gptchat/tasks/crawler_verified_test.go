package tasks

import (
	"context"
	"encoding/json"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/laisky-blog-graphql/library/crawleregress"
	rlibs "github.com/Laisky/laisky-blog-graphql/library/db/redis"
	"github.com/stretchr/testify/require"
)

// verifiedTestTask returns a synthetic task with public initial pins and no DNS I/O.
func verifiedTestTask() *rlibs.HTMLCrawlerTask {
	policy := crawleregress.Policy{Host: "example.com", Addresses: []string{"8.8.8.8"}, MaxRedirects: 5}
	return rlibs.NewHTMLCrawlerTaskWithEgress("https://example.com/private?canary=query", "token-canary", true, &policy)
}

// verifiedTestReceipt records a synthetic observed initial connection for task.
func verifiedTestReceipt(task *rlibs.HTMLCrawlerTask) *crawleregress.Receipt {
	receipt := crawleregress.NewReceipt(task.TaskID, task.Url, *task.Egress)
	receipt.Origins = []crawleregress.Origin{{URL: task.Url, Addresses: []string{"8.8.8.8"}, PeerAddress: "8.8.8.8", Kind: crawleregress.OriginDocument}}
	return receipt
}

// preserveVerifiedWorkerSeams restores dependency seams after a serial unit test.
func preserveVerifiedWorkerSeams(t *testing.T) {
	t.Helper()
	fetch, publish, persist, convert := fetchVerifiedHTMLContent, publishHTMLCrawlerTask, persistVerifiedCrawlRecord, convertVerifiedMarkdown
	admit, enqueue, load, proxies := admitHTMLCrawlerURL, enqueueAdmittedHTMLCrawlerTask, loadVerifiedHTMLCrawlerTask, webFetchProxies
	t.Cleanup(func() {
		fetchVerifiedHTMLContent, publishHTMLCrawlerTask, persistVerifiedCrawlRecord, convertVerifiedMarkdown = fetch, publish, persist, convert
		admitHTMLCrawlerURL, enqueueAdmittedHTMLCrawlerTask, loadVerifiedHTMLCrawlerTask, webFetchProxies = admit, enqueue, load, proxies
	})
	t.Setenv("CRAWLER_HTTP_PROXY", "")
}

// countingVerifiedProxy detects any unintended transmission to an external fallback.
type countingVerifiedProxy struct{ calls *atomic.Int32 }

// Name returns the synthetic external proxy identifier.
func (p countingVerifiedProxy) Name() string { return "external-canary" }

// Priority returns the priority of the synthetic proxy.
func (p countingVerifiedProxy) Priority() int { return 100 }

// Fetch records an unintended outbound invocation and returns synthetic content.
func (p countingVerifiedProxy) Fetch(context.Context, string) (string, error) {
	p.calls.Add(1)
	return "external-result", nil
}

// TestVerifiedTaskLegacyEnvelopeLoss reproduces the old model's policy/evidence
// loss and demonstrates that the modern envelope preserves the secure contract.
func TestVerifiedTaskLegacyEnvelopeLoss(t *testing.T) {
	task := verifiedTestTask()
	task.EgressReceipt = verifiedTestReceipt(task)
	task.RequestChain = []string{task.Url}
	payload, err := task.ToString()
	require.NoError(t, err)
	// This is the historical May library's crawler wire schema. Unknown fields
	// were ignored and never appeared in the worker's serialized response.
	var legacy struct {
		TaskID         string `json:"task_id"`
		URL            string `json:"url"`
		APIKey         string `json:"api_key,omitempty"`
		OutputMarkdown bool   `json:"output_markdown,omitempty"`
		ResultHTML     []byte `json:"result_html,omitempty"`
	}
	require.NoError(t, json.Unmarshal([]byte(payload), &legacy))
	legacy.ResultHTML = []byte("successful-render")
	legacyPayload, err := json.Marshal(legacy)
	require.NoError(t, err)
	legacyResult, err := rlibs.NewHTMLCrawlerTaskFromString(string(legacyPayload))
	require.NoError(t, err)
	require.Nil(t, legacyResult.Egress)
	require.Empty(t, legacyResult.RequestChain)
	require.ErrorIs(t, crawleregress.VerifyReceipt(context.Background(), task.TaskID, task.Url, *task.Egress, legacyResult.EgressReceipt), crawleregress.ErrUnverified)
	modern, err := rlibs.NewHTMLCrawlerTaskFromString(payload)
	require.NoError(t, err)
	require.Equal(t, task.Egress, modern.Egress)
	require.NoError(t, crawleregress.VerifyReceipt(context.Background(), task.TaskID, task.Url, *task.Egress, modern.EgressReceipt))
}

// TestVerifiedTaskNoExternalFallbacks covers all legacy fallback triggers while
// asserting that the secure task worker never invokes external fetch/conversion.
func TestVerifiedTaskNoExternalFallbacks(t *testing.T) {
	for _, tc := range []struct {
		name, html, code string
		renderErr        error
		conversionErr    bool
	}{
		{name: "direct_error", code: "render_failed", renderErr: errors.New("private-upstream token-canary https://example.com/?secret=yes")},
		{name: "challenge", html: "<body>cf-chl-widget</body>", code: "challenge"},
		{name: "empty_body", html: " \n ", code: "empty_body"},
		{name: "empty_markdown", html: "<html><body></body></html>", code: "empty_markdown"},
		{name: "conversion_error", html: "<body>local-text</body>", code: "conversion_failed", conversionErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			preserveVerifiedWorkerSeams(t)
			var calls atomic.Int32
			webFetchProxies = []webFetchProxy{countingVerifiedProxy{calls: &calls}}
			fetchVerifiedHTMLContent = func(_ context.Context, task *rlibs.HTMLCrawlerTask) (string, *crawleregress.Receipt, error) {
				require.Empty(t, task.APIKey)
				return tc.html, verifiedTestReceipt(task), tc.renderErr
			}
			if tc.conversionErr {
				convertVerifiedMarkdown = func(string) (string, error) { return "", errors.New("private-conversion token-canary") }
			}
			body, markdown, receipt, err := fetchVerifiedCrawlerTask(context.Background(), verifiedTestTask())
			require.Error(t, err)
			code, _ := crawlerDiagnostic(err)
			require.Equal(t, tc.code, code)
			require.Nil(t, body)
			require.Empty(t, markdown)
			require.Nil(t, receipt)
			require.Zero(t, calls.Load())
			require.NotContains(t, err.Error(), "token-canary")
			require.NotContains(t, err.Error(), "https://")
			require.NotContains(t, err.Error(), "private-upstream")
		})
	}
}

// TestVerifiedTaskValidHTMLAndMarkdown is the positive control for pinned render
// and local conversion, including preservation of body and observed evidence.
func TestVerifiedTaskValidHTMLAndMarkdown(t *testing.T) {
	for _, outputMarkdown := range []bool{false, true} {
		t.Run(strings.ToLower(map[bool]string{false: "HTML", true: "Markdown"}[outputMarkdown]), func(t *testing.T) {
			preserveVerifiedWorkerSeams(t)
			task := verifiedTestTask()
			task.OutputMarkdown = outputMarkdown
			fetchVerifiedHTMLContent = func(_ context.Context, renderTask *rlibs.HTMLCrawlerTask) (string, *crawleregress.Receipt, error) {
				require.Empty(t, renderTask.APIKey)
				return "<html><body><h1>Local title</h1><p>Local content.</p></body></html>", verifiedTestReceipt(renderTask), nil
			}
			body, markdown, receipt, err := fetchVerifiedCrawlerTask(context.Background(), task)
			require.NoError(t, err)
			require.Contains(t, string(body), "<h1>Local title</h1>")
			require.NoError(t, crawleregress.VerifyReceipt(context.Background(), task.TaskID, task.Url, *task.Egress, receipt))
			if outputMarkdown {
				require.Contains(t, markdown, "Local content.")
			} else {
				require.Empty(t, markdown)
			}
		})
	}
}

// TestVerifiedTaskAdmissionAndEvidenceFailures denies unsupported settings and
// missing/forged evidence before returning any potentially private body.
func TestVerifiedTaskAdmissionAndEvidenceFailures(t *testing.T) {
	for _, tc := range []string{"missing_policy", "private_pin", "configured_proxy", "missing_receipt", "forged_task", "forged_peer", "forged_policy"} {
		t.Run(tc, func(t *testing.T) {
			preserveVerifiedWorkerSeams(t)
			task := verifiedTestTask()
			switch tc {
			case "missing_policy":
				task.Egress = nil
			case "private_pin":
				task.Egress.Addresses = []string{"127.0.0.1"}
			case "configured_proxy":
				t.Setenv("CRAWLER_HTTP_PROXY", "http://proxy-canary.invalid")
			}
			calls := 0
			fetchVerifiedHTMLContent = func(_ context.Context, renderTask *rlibs.HTMLCrawlerTask) (string, *crawleregress.Receipt, error) {
				calls++
				receipt := verifiedTestReceipt(renderTask)
				switch tc {
				case "missing_receipt":
					receipt = nil
				case "forged_task":
					receipt.TaskID = "different-task"
				case "forged_peer":
					receipt.Origins[0].PeerAddress = "127.0.0.1"
				case "forged_policy":
					receipt.PolicyDigest = "forged"
				}
				return "<body>must-not-return</body>", receipt, nil
			}
			body, _, receipt, err := fetchVerifiedCrawlerTask(context.Background(), task)
			require.Error(t, err)
			require.Nil(t, body)
			require.Nil(t, receipt)
			if tc == "missing_policy" || tc == "private_pin" || tc == "configured_proxy" {
				require.Zero(t, calls)
			}
		})
	}
}

// TestVerifiedTaskCancellationNeverPublishesSuccess stops canceled work before
// rendering and when a renderer completes after its context has expired.
func TestVerifiedTaskCancellationNeverPublishesSuccess(t *testing.T) {
	for _, before := range []bool{true, false} {
		t.Run(map[bool]string{true: "before_render", false: "late_renderer"}[before], func(t *testing.T) {
			preserveVerifiedWorkerSeams(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if before {
				cancel()
			}
			calls, publications := 0, 0
			fetchVerifiedHTMLContent = func(_ context.Context, task *rlibs.HTMLCrawlerTask) (string, *crawleregress.Receipt, error) {
				calls++
				cancel()
				return "<body>late-result</body>", verifiedTestReceipt(task), nil
			}
			publishHTMLCrawlerTask = func(ctx context.Context, task *rlibs.HTMLCrawlerTask) error {
				if err := ctx.Err(); err != nil {
					return errors.WithStack(err)
				}
				publications++
				require.NotEqual(t, rlibs.TaskStatusSuccess, task.Status)
				if publications == 2 {
					require.Equal(t, rlibs.TaskStatusFailed, task.Status)
					require.Equal(t, "canceled", *task.FailedReason)
					require.Empty(t, task.ResultHTML)
				}
				return nil
			}
			err := processHTMLCrawlerTask(ctx, verifiedTestTask())
			require.ErrorIs(t, err, context.Canceled)
			if before {
				require.Zero(t, calls)
				require.Zero(t, publications)
			} else {
				require.Equal(t, 1, calls)
				require.Equal(t, 2, publications)
			}
		})
	}
	// Deadline errors are classified independently from deliberate cancellation.
	err := crawlerFailure("render_failed", "render", context.DeadlineExceeded)
	code, _ := crawlerDiagnostic(err)
	require.Equal(t, "timeout", code)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

// TestVerifiedWorkerPublishesSanitizedResults exercises the real queue-worker
// integration and ensures credentials and raw failure details never persist.
func TestVerifiedWorkerPublishesSanitizedResults(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "failure"}[fail], func(t *testing.T) {
			preserveVerifiedWorkerSeams(t)
			task := verifiedTestTask()
			withInjectedPop(t, func(context.Context) (*rlibs.HTMLCrawlerTask, error) { return task, nil })
			fetchVerifiedHTMLContent = func(_ context.Context, renderTask *rlibs.HTMLCrawlerTask) (string, *crawleregress.Receipt, error) {
				if fail {
					return "", nil, errors.New("token-canary https://example.com/private?canary=query")
				}
				return "<body>safe-result</body>", verifiedTestReceipt(renderTask), nil
			}
			states := []string{}
			publishHTMLCrawlerTask = func(_ context.Context, result *rlibs.HTMLCrawlerTask) error {
				payload, err := result.ToString()
				require.NoError(t, err)
				require.NotContains(t, payload, "token-canary")
				states = append(states, result.Status)
				return nil
			}
			audits := 0
			persistVerifiedCrawlRecord = func(_ context.Context, record *CrawlRecord) error {
				audits++
				require.Empty(t, record.APIKeyPrefix)
				return nil
			}
			require.NoError(t, runDynamicWebCrawler())
			require.Equal(t, rlibs.TaskStatusRunning, states[0])
			if fail {
				require.Equal(t, []string{rlibs.TaskStatusRunning, rlibs.TaskStatusFailed}, states)
				require.Equal(t, "render_failed", *task.FailedReason)
				require.Nil(t, task.EgressReceipt)
				require.Zero(t, audits)
			} else {
				require.Equal(t, []string{rlibs.TaskStatusRunning, rlibs.TaskStatusSuccess}, states)
				require.NotNil(t, task.EgressReceipt)
				require.Equal(t, []string{task.Url}, task.RequestChain)
				require.Equal(t, 1, audits)
			}
		})
	}
}

// TestVerifiedProducerAdmitsBeforeEnqueue ensures all producer tasks carry pins
// and no provider token, and failed/canceled admission cannot enqueue work.
func TestVerifiedProducerAdmitsBeforeEnqueue(t *testing.T) {
	for _, scenario := range []string{"valid", "rejected", "canceled_during_admission"} {
		t.Run(scenario, func(t *testing.T) {
			preserveVerifiedWorkerSeams(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			admitHTMLCrawlerURL = func(ctx context.Context, raw string) (crawleregress.Admission, error) {
				if scenario == "rejected" {
					return crawleregress.Admission{}, errors.New("target rejected")
				}
				admission, err := crawleregress.AdmitURLWithResolver(ctx, raw, func(context.Context, string) ([]net.IPAddr, error) {
					return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
				})
				if scenario == "canceled_during_admission" {
					cancel()
				}
				return admission, err
			}
			enqueued := 0
			enqueueAdmittedHTMLCrawlerTask = func(_ context.Context, task *rlibs.HTMLCrawlerTask) error {
				enqueued++
				require.Empty(t, task.APIKey)
				require.Equal(t, []string{"8.8.8.8"}, task.Egress.Addresses)
				require.False(t, task.Egress.AllowSubresources)
				return nil
			}
			id, err := addHTMLCrawlerTask(ctx, "https://example.com/", "token-canary", true)
			if scenario == "valid" {
				require.NoError(t, err)
				require.NotEmpty(t, id)
				require.Equal(t, 1, enqueued)
			} else {
				require.Error(t, err)
				require.Zero(t, enqueued)
			}
		})
	}
}

// TestVerifiedConsumerRejectsUnverifiedBody protects the task-return boundary
// independently from worker implementation and consumes Markdown when requested.
func TestVerifiedConsumerRejectsUnverifiedBody(t *testing.T) {
	for _, scenario := range []string{"valid", "legacy_receipt", "changed_identity", "changed_format", "failed_reason", "pending_cancellation", "oversized_result"} {
		t.Run(scenario, func(t *testing.T) {
			preserveVerifiedWorkerSeams(t)
			submitted := verifiedTestTask()
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
			defer cancel()
			loadVerifiedHTMLCrawlerTask = func(context.Context, string) (*rlibs.HTMLCrawlerTask, error) {
				result := *submitted
				result.Status = rlibs.TaskStatusSuccess
				result.ResultHTML, result.ResultMarkdown = []byte("raw-html"), []byte("local-markdown")
				result.EgressReceipt = verifiedTestReceipt(submitted)
				switch scenario {
				case "legacy_receipt":
					result.EgressReceipt = nil
					result.RequestChain = []string{submitted.Url}
				case "changed_identity":
					result.TaskID = "other-task"
				case "changed_format":
					result.OutputMarkdown = false
				case "failed_reason":
					result.Status = rlibs.TaskStatusFailed
					reason := "token-canary"
					result.FailedReason = &reason
				case "pending_cancellation":
					result.Status = rlibs.TaskStatusRunning
				case "oversized_result":
					result.ResultHTML = make([]byte, rendererMaxResponseBytes+1)
				}
				return &result, nil
			}
			body, err := waitVerifiedHTMLCrawlerTask(ctx, submitted)
			if scenario == "valid" {
				require.NoError(t, err)
				require.Equal(t, []byte("local-markdown"), body)
			} else {
				require.Error(t, err)
				require.Nil(t, body)
				require.NotContains(t, err.Error(), "token-canary")
			}
			if scenario == "pending_cancellation" {
				require.ErrorIs(t, err, context.DeadlineExceeded)
			}
		})
	}
}

// TestVerifiedWorkerNilTask returns a safe failure for an absent accepted task.
func TestVerifiedWorkerNilTask(t *testing.T) {
	require.Error(t, processHTMLCrawlerTask(context.Background(), nil))
}

// TestVerifiedWorkerTimeoutHasTerminalState records a timed-out accepted task
// using bounded cleanup without publishing a body or invoking audit persistence.
func TestVerifiedWorkerTimeoutHasTerminalState(t *testing.T) {
	preserveVerifiedWorkerSeams(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	task := verifiedTestTask()
	states := []string{}
	publishHTMLCrawlerTask = func(publishCtx context.Context, result *rlibs.HTMLCrawlerTask) error {
		require.NoError(t, publishCtx.Err())
		states = append(states, result.Status)
		if result.Status == rlibs.TaskStatusFailed {
			deadline, ok := publishCtx.Deadline()
			require.True(t, ok)
			require.LessOrEqual(t, time.Until(deadline), 5*time.Second)
			require.Equal(t, "timeout", *result.FailedReason)
			require.Empty(t, result.ResultHTML)
			require.Empty(t, result.ResultMarkdown)
			require.Nil(t, result.EgressReceipt)
		}
		return nil
	}
	fetchVerifiedHTMLContent = func(renderCtx context.Context, _ *rlibs.HTMLCrawlerTask) (string, *crawleregress.Receipt, error) {
		<-renderCtx.Done()
		return "", nil, errors.WithStack(renderCtx.Err())
	}
	persistVerifiedCrawlRecord = func(context.Context, *CrawlRecord) error {
		t.Fatal("timed-out task must not persist content")
		return nil
	}
	require.ErrorIs(t, processHTMLCrawlerTask(ctx, task), context.DeadlineExceeded)
	require.Equal(t, []string{rlibs.TaskStatusRunning, rlibs.TaskStatusFailed}, states)
}

// TestCrawlerTaskLogIDBoundsUserInput prevents arbitrary queue metadata from
// becoming an alert field while preserving generated UUID correlation.
func TestCrawlerTaskLogIDBoundsUserInput(t *testing.T) {
	require.Equal(t, "invalid", crawlerLogTaskID("token-canary https://example.com"))
	require.Equal(t, "invalid", crawlerLogTaskID("01234567-0123-0123-0123-01234567890z"))
	require.Equal(t, "01234567-0123-0123-0123-01234567890a", crawlerLogTaskID("01234567-0123-0123-0123-01234567890a"))
}
