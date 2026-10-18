package tasks

import (
	"bytes"
	"context"
	"os"
	"strings"
	"time"

	md "github.com/JohannesKaufmann/html-to-markdown/v2"
	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"

	"github.com/Laisky/laisky-blog-graphql/library/crawleregress"
	rlibs "github.com/Laisky/laisky-blog-graphql/library/db/redis"
	"github.com/Laisky/zap"
	"golang.org/x/net/html"

	"github.com/Laisky/go-ramjet/library/log"
)

var (
	fetchVerifiedHTMLContent   = renderVerifiedHTML
	publishHTMLCrawlerTask     = setHTMLCrawlerTaskResult
	persistVerifiedCrawlRecord = SaveCrawlRecord
	convertVerifiedMarkdown    = convertVerifiedHTMLToMarkdown
)

// convertVerifiedHTMLToMarkdown converts already-rendered content locally and
// returns Markdown or the converter error, without any remote fetch or token.
func convertVerifiedHTMLToMarkdown(content string) (string, error) {
	return md.ConvertString(content)
}

// crawlerLogTaskID returns a UUID-shaped task identifier or a redacted marker.
func crawlerLogTaskID(taskID string) string {
	if len(taskID) != 36 {
		return "invalid"
	}
	for i, char := range taskID {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if char != '-' {
				return "invalid"
			}
			continue
		}
		if !(char >= '0' && char <= '9' || char >= 'a' && char <= 'f') {
			return "invalid"
		}
	}
	return taskID
}

// fetchVerifiedCrawlerTask renders task using its admitted pins and returns body,
// local Markdown and verified observed connections. It never invokes external
// fetch or conversion fallbacks, and never passes the task API key to a renderer.
func fetchVerifiedCrawlerTask(ctx context.Context, task *rlibs.HTMLCrawlerTask) (resultBody []byte, resultMarkdown string, resultReceipt *crawleregress.Receipt, resultErr error) {
	var observation *crawlerContextObservation
	defer func() {
		if observation != nil && resultErr != nil {
			resultErr = withCrawlerObservedFailure(resultErr, observation.snapshot())
		}
	}()
	if err := ctx.Err(); err != nil {
		return nil, "", nil, crawlerFailure("worker_failed", "admission", err)
	}
	if task == nil || task.Egress == nil || task.TaskID == "" {
		return nil, "", nil, crawlerFailure("policy_missing", "admission", nil)
	}
	policy := crawleregress.ClonePolicy(*task.Egress)
	admission, err := crawleregress.ValidateDestination(ctx, task.Url, policy.Addresses)
	if err != nil {
		return nil, "", nil, crawlerFailure("policy_invalid", "admission", err)
	}
	if admission.Host != policy.Host || policy.MaxRedirects < 0 || policy.MaxRedirects > crawleregress.MaxRedirects {
		return nil, "", nil, crawlerFailure("policy_invalid", "admission", nil)
	}
	// An external browser proxy has no enforceable connection evidence. Refuse
	// it before transmitting the target instead of bypassing configured intent.
	if os.Getenv("CRAWLER_HTTP_PROXY") != "" {
		return nil, "", nil, crawlerFailure("unsupported_proxy", "admission", nil)
	}
	ctx, observation = newCrawlerContextObservation(ctx, task.Url)
	renderTask := *task
	renderTask.APIKey = ""
	renderPolicy := crawleregress.ClonePolicy(policy)
	renderTask.Egress = &renderPolicy
	renderTask.RequestChain = nil
	renderTask.EgressReceipt = nil
	renderTask.ResultHTML = nil
	renderTask.ResultMarkdown = nil
	htmlContent, receipt, err := fetchVerifiedHTMLContent(ctx, &renderTask)
	if err != nil {
		if htmlContent != "" {
			observeCrawlerCaptureRendered(ctx, htmlContent)
		}
	} else {
		observeCrawlerCaptureRendered(ctx, htmlContent)
	}
	observation.sealDocuments()
	if err != nil {
		return nil, "", nil, crawlerFailure("render_failed", "render", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, "", nil, crawlerFailure("render_failed", "render", err)
	}
	observeCrawlerRenderedBytes(ctx, len(htmlContent))
	if len(htmlContent) > rendererMaxResponseBytes {
		return nil, "", nil, crawlerFailure("result_too_large", "render", nil)
	}
	if err := crawleregress.VerifyReceipt(ctx, task.TaskID, task.Url, policy, receipt); err != nil {
		return nil, "", nil, crawlerFailure("evidence_rejected", "verification", err)
	}
	if signal := cloudflareChallengeSignal(htmlContent); signal != "" {
		return nil, "", nil, crawlerChallengeFailure(signal)
	}
	if strings.TrimSpace(htmlContent) == "" {
		return nil, "", nil, crawlerFailure("empty_body", "render", nil)
	}
	body, markdown, err := extractVerifiedHTMLBody(ctx, htmlContent, task.OutputMarkdown)
	if err != nil {
		return nil, "", nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, "", nil, crawlerFailure("worker_failed", "conversion", err)
	}
	if len(body) > rendererMaxResponseBytes || len(markdown) > rendererMaxResponseBytes {
		return nil, "", nil, crawlerFailure("result_too_large", "conversion", nil)
	}
	return body, markdown, receipt, nil
}

// extractVerifiedHTMLBody extracts content and optionally converts it locally.
// It returns a scalar failure if the requested conversion cannot produce text.
func extractVerifiedHTMLBody(ctx context.Context, content string, outputMarkdown bool) ([]byte, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", crawlerFailure("conversion_failed", "conversion", err)
	}
	document, err := html.Parse(strings.NewReader(content))
	if err != nil {
		return nil, "", crawlerFailure("conversion_failed", "conversion", err)
	}
	body := findHTMLBody(document)
	if body == nil {
		return nil, "", crawlerFailure("empty_body", "conversion", nil)
	}
	var rendered bytes.Buffer
	if err := html.Render(&rendered, body); err != nil {
		return nil, "", crawlerFailure("conversion_failed", "conversion", err)
	}
	if !outputMarkdown {
		return rendered.Bytes(), "", nil
	}
	observeCrawlerConversionInput(ctx, body, rendered.Len())
	converterInput := rendered.String()
	observeCrawlerCaptureConverter(ctx, converterInput)
	markdown, err := convertVerifiedMarkdown(converterInput)
	if err != nil {
		return nil, "", crawlerFailure("conversion_failed", "conversion", err)
	}
	observeCrawlerConversionOutput(ctx, markdown, strings.TrimSpace(markdown))
	markdown = strings.TrimSpace(markdown)
	if markdown == "" {
		return nil, "", crawlerFailure("empty_markdown", "conversion", nil)
	}
	return rendered.Bytes(), markdown, nil
}

// processHTMLCrawlerTask executes task and publishes its terminal status with
// safe failure diagnostics; successful bodies require verified connection evidence.
func processHTMLCrawlerTask(ctx context.Context, task *rlibs.HTMLCrawlerTask) error {
	if task == nil {
		return crawlerFailure("task_missing", "admission", nil)
	}
	ctx, capture := withCrawlerFailureCapture(ctx, task)
	logger := log.Logger.Named("run_dynamic_web_crawler.verified_crawler_task")
	ctx = gmw.SetLogger(ctx, logger)
	startedAt := time.Now()
	// Publication errors belong to this reviewed boundary; the outer loop retains backoff without emitting them again.
	publishFailure := func(publishErr error, cleanup bool) error {
		fields := crawlerLifecycleFields(ctx, task.TaskID, "result_publication_failed", task.Status, "failed", startedAt)
		fields = append(fields, capture.fields()...)
		fields = append(fields, zap.String("stage", "persistence"), zap.String("error_code", "publication_failed"),
			zap.Bool("cleanup", cleanup), zap.Error(&crawlerTaskError{code: "publication_failed", stage: "persistence", cause: publishErr}))
		logger.Error("crawler result publication failed", fields...)
		return &crawlerLoggedError{cause: publishErr}
	}
	processingComplete := func(failure error) {
		fields := crawlerLifecycleFields(ctx, task.TaskID, "processing_completed", task.Status, "", startedAt)
		if failure != nil {
			code, stage := crawlerDiagnostic(failure)
			fields = append(fields, capture.fields()...)
			fields = append(fields, zap.String("stage", stage), zap.String("category", code), zap.String("error_code", code))
			fields = append(fields, crawlerFailureContextFields(failure)...)
			if contextValue := crawlerFailureContextFrom(failure); contextValue == nil || contextValue.UpstreamStatus == nil {
				fields = append(fields, crawlerUpstreamStatusFields(failure)...)
			}
			if signal := crawlerDiagnosticSignal(failure); signal != "" {
				fields = append(fields, zap.String("challenge_signal", string(signal)))
			}
			fields = append(fields, zap.Error(failure))
			logger.Error("verified crawler task failed", fields...)
			return
		}
		fields = append(fields, zap.String("stage", "processing"))
		logger.Info("crawler processing completed", fields...)
	}
	publicationComplete := func() {
		logger.Info("crawler terminal result published",
			crawlerLifecycleFields(ctx, task.TaskID, "result_published", task.Status, "success", startedAt)...)
	}
	// Secure rendering and local conversion have no use for a provider token.
	task.APIKey = ""
	task.ResultHTML, task.ResultMarkdown, task.RequestChain = nil, nil, nil
	task.EgressReceipt, task.FailedReason, task.FinishedAt = nil, nil, nil
	task.Status = rlibs.TaskStatusRunning
	if err := publishHTMLCrawlerTask(ctx, task); err != nil {
		return publishFailure(errors.Wrap(err, "publish crawler running state"), false)
	}
	body, markdown, receipt, fetchErr := fetchVerifiedCrawlerTask(ctx, task)
	if err := ctx.Err(); err != nil {
		// Accepted work needs a terminal state even when its independent worker context ends.
		cleanupCtx, cancelCleanup := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelCleanup()
		now := time.Now().UTC()
		failure := crawlerFailure("worker_failed", "render", err)
		if fetchErr != nil {
			// Preserve a concrete primary failure even when the worker context also ends.
			failure = fetchErr
		}
		code, _ := crawlerDiagnostic(failure)
		task.Status, task.FailedReason, task.FinishedAt = rlibs.TaskStatusFailed, &code, &now
		processingComplete(failure)
		if cleanupErr := publishHTMLCrawlerTask(withCrawlerFailure(cleanupCtx, failure), task); cleanupErr != nil {
			// The publication error is separate from the already-emitted processing failure.
			_ = publishFailure(cleanupErr, true)
		} else {
			publicationComplete()
		}
		return &crawlerLoggedError{cause: errors.WithStack(errors.Join(failure, crawlerFailure("worker_failed", "render", err)))}
	}
	now := time.Now().UTC()
	task.FinishedAt = &now
	if fetchErr != nil {
		code, _ := crawlerDiagnostic(fetchErr)
		reason := code
		task.Status, task.FailedReason = rlibs.TaskStatusFailed, &reason
		processingComplete(fetchErr)
		ctx = withCrawlerFailure(ctx, fetchErr)
	} else {
		task.Status, task.ResultHTML = rlibs.TaskStatusSuccess, body
		task.ResultMarkdown, task.EgressReceipt = []byte(markdown), receipt
		for _, origin := range receipt.Origins {
			if origin.Kind == crawleregress.OriginDocument {
				task.RequestChain = append(task.RequestChain, origin.URL)
			}
		}
		processingComplete(nil)
		var markdownPtr *string
		if markdown != "" {
			markdownPtr = &markdown
		}
		record := &CrawlRecord{TaskID: task.TaskID, CrawledAt: now,
			URL: task.Url, RawBody: body, Markdown: markdownPtr}
		if err := persistVerifiedCrawlRecord(ctx, record); err != nil {
			fields := crawlerLifecycleFields(ctx, task.TaskID, "audit_persistence_failed", task.Status, "", startedAt)
			fields = append(fields, capture.fields()...)
			fields = append(fields, zap.String("error_code", "audit_store_failed"), zap.String("stage", "persistence"),
				zap.Error(&crawlerTaskError{code: "audit_store_failed", stage: "persistence", cause: err}))
			logger.Warn("crawler audit persistence failed", fields...)
		}
	}
	if err := publishHTMLCrawlerTask(ctx, task); err != nil {
		return publishFailure(errors.Wrap(err, "publish crawler terminal state"), false)
	}
	publicationComplete()
	return nil
}
