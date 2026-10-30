package tasks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/laisky-blog-graphql/library/crawleregress"
	rlibs "github.com/Laisky/laisky-blog-graphql/library/db/redis"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/stretchr/testify/require"
)

// crawlerDiagnosticCanaryCause supplies a typed private cause without real credentials or traffic.
type crawlerDiagnosticCanaryCause struct{}

// Error returns synthetic private URL, key and body data for redaction controls.
func (*crawlerDiagnosticCanaryCause) Error() string {
	return "navigation failed: request https://private.invalid/path?api_key=private-key-canary&query=private-query-canary Authorization: Bearer private-auth-canary body=private-body-canary"
}

// TestCrawlerFailureRetainsOrdinaryCause verifies errors.Is and errors.As without losing an ordinary cause.
func TestCrawlerFailureRetainsOrdinaryCause(t *testing.T) {
	cause := &crawlerDiagnosticCanaryCause{}
	err := crawlerFailure("render_failed", "render", errors.WithStack(cause))
	require.ErrorIs(t, err, cause)
	var typed *crawlerDiagnosticCanaryCause
	require.ErrorAs(t, err, &typed)
	require.Same(t, cause, typed)
}

// TestCrawlerFailureCauseSerializationSafe exercises actual JSON error/errorVerbose and stack formatting.
func TestCrawlerFailureCauseSerializationSafe(t *testing.T) {
	var encoded bytes.Buffer
	core := zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
		zapcore.AddSync(&encoded), zap.DebugLevel)
	logger, err := glog.NewWithName("diagnostic-test", glog.LevelDebug,
		zap.WrapCore(func(zapcore.Core) zapcore.Core { return core }))
	require.NoError(t, err)
	failure := crawlerFailure("render_failed", "render", errors.WithStack(&crawlerDiagnosticCanaryCause{}))
	logger.Error("synthetic crawler failure", zap.Error(failure))
	require.NotEmpty(t, encoded.String())
	require.Contains(t, encoded.String(), "render_failed")
	require.Contains(t, encoded.String(), "navigation failed")
	require.Contains(t, failure.Error(), "navigation failed")
	for _, canary := range []string{"private.invalid", "private-query-canary", "private-key-canary", "private-body-canary", "private-auth-canary"} {
		require.NotContains(t, encoded.String(), canary)
		require.NotContains(t, fmt.Sprintf("%+v", failure), canary)
		require.NotContains(t, failure.Error(), canary)
	}
}

// TestVerifiedRendererShutdownCancellationKeepsPrimary reproduces the renderer's primary-plus-capture cancellation.
func TestVerifiedRendererShutdownCancellationKeepsPrimary(t *testing.T) {
	preserveVerifiedWorkerSeams(t)
	ctx := context.Background()
	cause := &crawlerDiagnosticCanaryCause{}
	var fallbacks atomic.Int32
	webFetchProxies = []webFetchProxy{countingVerifiedProxy{calls: &fallbacks}}
	fetchVerifiedHTMLContent = func(parent context.Context, _ *rlibs.HTMLCrawlerTask) (string, *crawleregress.Receipt, error) {
		renderCtx, stopRender := context.WithCancel(parent)
		stopRender()
		return "", nil, errors.Join(cause, renderCtx.Err())
	}
	body, _, receipt, err := fetchVerifiedCrawlerTask(ctx, verifiedTestTask())
	require.Error(t, err)
	require.NoError(t, ctx.Err(), "the caller remains active after internal renderer shutdown")
	code, stage := crawlerDiagnostic(err)
	require.Equal(t, "render_failed", code, "internal shutdown is not user cancellation")
	require.Equal(t, "render", stage)
	require.ErrorIs(t, err, cause)
	require.Nil(t, body)
	require.Nil(t, receipt)
	require.Zero(t, fallbacks.Load())
}

// TestVerifiedChallengeDetectionSemanticControls keeps ordinary content separate from active challenge structure.
func TestVerifiedChallengeDetectionSemanticControls(t *testing.T) {
	for _, tc := range []struct {
		name, content string
		challenge     bool
	}{
		{name: "readable_jsd_script", content: `<html><head><script src="/cdn-cgi/challenge-platform/scripts/jsd/api.js?onload=onload"></script></head><body><article><h1>Manage DID</h1><p>Readable article content.</p></article></body></html>`},
		{name: "injected_jsd_script", content: `<html><head><script src="/cdn-cgi/challenge-platform/h/g/scripts/jsd/main.js"></script></head><body><article>Readable content</article></body></html>`},
		{name: "jsd_with_challenge_title", content: `<html><head><title>Just a moment...</title><script src="/cdn-cgi/challenge-platform/h/g/jsd/r"></script></head><body><article>Readable content</article></body></html>`},
		{name: "turnstile_api", content: `<script src="/cdn-cgi/challenge-platform/h/g/orchestrate/chl_api/v1"></script><article>Contact</article>`},
		{name: "mode_prefix_only", content: `<script src="/cdn-cgi/challenge-platform/h/g/orchestrate/chl_page_example/v1"></script><article>Content</article>`},
		{name: "malformed_url", content: `<script src="http://[invalid/cdn-cgi/challenge-platform/h/g/orchestrate/chl_page/v1"></script><article>Content</article>`},
		{name: "localized_managed", content: `<html><head><title>Veuillez patienter</title><script src="/cdn-cgi/challenge-platform/h/g/orchestrate/managed/v1"></script></head><body>Verification</body></html>`, challenge: true},
		{name: "legacy_js_challenge", content: `<script src="/cdn-cgi/challenge-platform/h/b/orchestrate/jsch/v1"></script>`, challenge: true},
		{name: "truncated_error", content: `<html><body><div id="challenge-error-text">Blocked`, challenge: true},
		{name: "article_with_widget", content: `<article>Readable content</article><div id="cf-chl-widget"></div>`, challenge: true},
		{name: "article_with_assignment", content: `<article>Readable content</article><script>window._cf_chl_opt = {};</script>`, challenge: true},
		{name: "ordinary_phrase", content: "<html><body><article><p>Just a moment... the speaker said.</p></article></body></html>"},
		{name: "ordinary_verification_phrase", content: "<html><body><article>Verification is taking longer than expected in this example.</article></body></html>"},
		{name: "active_script_after_division", content: "<html><body><script>const ratio = 1/2; window._cf_chl_opt = {mode: \"managed\"};</script></body></html>", challenge: true},
		{name: "regex_script_marker", content: "<html><body><article>Cloudflare documentation</article><script>const pattern = /_cf_chl_opt = example/;</script></body></html>"},
		{name: "quoted_script_marker", content: "<html><body><article>Cloudflare documentation</article><script>const help = \"_cf_chl_opt = example\";</script></body></html>"},
		{name: "commented_script_marker", content: "<html><body><article>Cloudflare documentation</article><script>/* window._cf_chl_opt = example; */</script></body></html>"},
		{name: "active_script_assignment", content: "<html><body><script>(function(){window._cf_chl_opt = {cType: \"managed\"};})();</script></body></html>", challenge: true},
		{name: "quoted_markers", content: "<html><body><pre>cf-chl-widget _cf_chl_opt challenge-platform challenge-error-text</pre></body></html>"},
		{name: "normal_turnstile_form", content: "<html><body><article>Contact us</article><form><div class=\"cf-turnstile\"></div><input name=\"cf-turnstile-response\"></form></body></html>"},
		{name: "active_widget", content: "<html><body><div id=\"cf-chl-widget\"></div></body></html>", challenge: true},
		{name: "active_resource", content: "<html><head><title>Just a moment...</title><script src=\"/cdn-cgi/challenge-platform/h/g/orchestrate/chl_page/v1\"></script></head><body>Please wait</body></html>", challenge: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			preserveVerifiedWorkerSeams(t)
			var fallbacks atomic.Int32
			webFetchProxies = []webFetchProxy{countingVerifiedProxy{calls: &fallbacks}}
			fetchVerifiedHTMLContent = func(_ context.Context, task *rlibs.HTMLCrawlerTask) (string, *crawleregress.Receipt, error) {
				return tc.content, verifiedTestReceipt(task), nil
			}
			task := verifiedTestTask()
			task.OutputMarkdown = false
			body, _, receipt, err := fetchVerifiedCrawlerTask(context.Background(), task)
			if tc.challenge {
				require.Error(t, err)
				code, stage := crawlerDiagnostic(err)
				require.Equal(t, "challenge", code)
				require.Equal(t, "render", stage)
				require.Nil(t, body)
				require.Nil(t, receipt)
			} else {
				require.NoError(t, err, "ordinary content with verified receipt must not be rejected as a challenge")
				require.NotEmpty(t, body)
				require.NoError(t, crawleregress.VerifyReceipt(context.Background(), task.TaskID, task.Url, *task.Egress, receipt))
			}
			require.Zero(t, fallbacks.Load())
		})
	}
}

// TestCrawlerResultExtendedEncoderCompatibility checks the real encoder against
// the frozen old queue schema and the current metadata-aware decoder.
func TestCrawlerResultExtendedEncoderCompatibility(t *testing.T) {
	task := verifiedTestTask()
	task.TaskID = "37b25fb8-4974-4a11-a5f5-b2f6c296025c"
	task.Url = "https://example.com/document"
	task.APIKey = ""
	task.Status = rlibs.TaskStatusFailed
	category := "conversion_failed"
	task.FailedReason = &category
	now := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)
	task.FinishedAt = &now
	ctx := withCrawlerFailure(context.Background(), crawlerFailure(category, "conversion", &crawlerDiagnosticCanaryCause{}))
	payload, err := encodeHTMLCrawlerTaskResult(ctx, task)
	require.NoError(t, err)
	var envelope map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(payload), &envelope))
	require.JSONEq(t, "\"conversion\"", string(envelope["failure_stage"]))
	require.JSONEq(t, "\"conversion_failed\"", string(envelope["failure_category"]))
	for _, canary := range []string{"private.invalid", "private-key-canary", "private-body-canary", "private-auth-canary"} {
		require.NotContains(t, payload, canary)
	}
	const fixturePath = "testdata/crawler_failure_diagnostics.json"
	if os.Getenv("UPDATE_CRAWLER_DIAGNOSTIC_FIXTURE") == "1" {
		require.NoError(t, os.MkdirAll("testdata", 0o750))
		require.NoError(t, os.WriteFile(fixturePath, []byte(payload+"\n"), 0o600))
	}
	fixture, err := os.ReadFile(fixturePath)
	require.NoError(t, err)
	current, err := rlibs.NewHTMLCrawlerTaskFromString(string(fixture))
	require.NoError(t, err)
	require.Equal(t, "conversion", current.FailureStage)
	require.Equal(t, category, current.FailureCategory)
	oldDecoder, oldPayload := roundTripLegacyCrawlerTask(t, string(fixture))
	require.Equal(t, task.TaskID, oldDecoder.TaskID)
	require.Equal(t, task.Url, oldDecoder.Url)
	require.Equal(t, task.Status, oldDecoder.Status)
	require.Equal(t, task.FailedReason, oldDecoder.FailedReason)
	require.Equal(t, task.FinishedAt, oldDecoder.FinishedAt)
	require.Equal(t, task.OutputMarkdown, oldDecoder.OutputMarkdown)
	require.NotNil(t, oldDecoder.Egress)
	require.Equal(t, task.Egress.Host, oldDecoder.Egress.Host)
	require.Equal(t, task.Egress.Addresses, oldDecoder.Egress.Addresses)
	require.Equal(t, task.Egress.MaxRedirects, oldDecoder.Egress.MaxRedirects)
	require.Equal(t, task.Egress.AllowSubresources, oldDecoder.Egress.AllowSubresources)
	require.Empty(t, oldDecoder.APIKey)
	require.Empty(t, oldDecoder.ResultHTML)
	require.Empty(t, oldDecoder.ResultMarkdown)
	require.Nil(t, oldDecoder.EgressReceipt)
	require.NotContains(t, oldPayload, "failure_stage")
	require.NotContains(t, oldPayload, "failure_category")
	for _, status := range []string{rlibs.TaskStatusRunning, rlibs.TaskStatusSuccess} {
		task.Status = status
		clean, err := encodeHTMLCrawlerTaskResult(ctx, task)
		require.NoError(t, err)
		require.NotContains(t, clean, "failure_stage")
		require.NotContains(t, clean, "failure_category")
	}
}

// TestCrawlerResultMetadataInvalidPairsAreOmitted rejects unknown/canary diagnostics without parsing FailedReason.
func TestCrawlerResultMetadataInvalidPairsAreOmitted(t *testing.T) {
	task := verifiedTestTask()
	task.Status = rlibs.TaskStatusFailed
	for _, pair := range []crawlerFailureMetadata{
		{Stage: "render", Category: "private-key-canary"},
		{Stage: "private-key-canary", Category: "render_failed"},
		{Stage: "conversion", Category: "policy_invalid"},
	} {
		ctx := context.WithValue(context.Background(), crawlerFailureKey{}, pair)
		payload, err := encodeHTMLCrawlerTaskResult(ctx, task)
		require.NoError(t, err)
		require.NotContains(t, payload, "failure_stage")
		require.NotContains(t, payload, "failure_category")
		require.NotContains(t, payload, "private-key-canary")
	}
}

// TestRendererPrimaryFailureSelection preserves the concrete cause and genuine caller lifecycle separately.
func TestRendererPrimaryFailureSelection(t *testing.T) {
	cause := &crawlerDiagnosticCanaryCause{}
	err := combineRendererFailure(context.Background(), cause, context.Canceled)
	require.ErrorIs(t, err, cause)
	require.NotErrorIs(t, err, context.Canceled)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = combineRendererFailure(ctx, cause, context.Canceled)
	require.ErrorIs(t, err, cause)
	require.ErrorIs(t, err, context.Canceled)
	err = combineRendererFailure(context.Background(), nil, context.DeadlineExceeded)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

// TestCrawlerCleanupPreservesConcreteFailure keeps known failure metadata when the parent also ends.
func TestCrawlerCleanupPreservesConcreteFailure(t *testing.T) {
	preserveVerifiedWorkerSeams(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cause := &crawlerDiagnosticCanaryCause{}
	fetchVerifiedHTMLContent = func(context.Context, *rlibs.HTMLCrawlerTask) (string, *crawleregress.Receipt, error) {
		cancel()
		return "", nil, cause
	}
	published := 0
	publishHTMLCrawlerTask = func(publishCtx context.Context, task *rlibs.HTMLCrawlerTask) error {
		published++
		if task.Status == rlibs.TaskStatusFailed {
			require.NoError(t, publishCtx.Err())
			require.Equal(t, "render_failed", *task.FailedReason)
			payload, err := encodeHTMLCrawlerTaskResult(publishCtx, task)
			require.NoError(t, err)
			require.Contains(t, payload, "\"failure_category\":\"render_failed\"")
			require.Contains(t, payload, "\"failure_stage\":\"render\"")
			require.Empty(t, task.ResultHTML)
			require.Nil(t, task.EgressReceipt)
		}
		return nil
	}
	err := processHTMLCrawlerTask(ctx, verifiedTestTask())
	require.ErrorIs(t, err, cause)
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 2, published)
}

// TestCrawlerProducerFailureCodeCompatibility retains the pre-existing consumer rejection identity.
func TestCrawlerProducerFailureCodeCompatibility(t *testing.T) {
	code, stage := crawlerDiagnostic(crawlerFailure("result_invalid", "verification", nil))
	require.Equal(t, "result_invalid", code)
	require.Equal(t, "verification", stage)
}
