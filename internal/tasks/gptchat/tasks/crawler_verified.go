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

// crawlerTaskFailure carries only reviewed scalar diagnostics for task failures.
type crawlerTaskFailure struct {
	code  string
	stage string
	cause error
}

// Error returns the scalar code and stage without the upstream error or target.
func (e *crawlerTaskFailure) Error() string { return "crawler " + e.stage + ": " + e.code }

// Unwrap returns a context lifecycle error, when present, for cancellation checks.
func (e *crawlerTaskFailure) Unwrap() error { return e.cause }

// crawlerFailure classifies err into a safe scalar failure for the supplied stage.
func crawlerFailure(code, stage string, err error) error {
	var cause error
	if errors.Is(err, context.Canceled) {
		code, cause = "canceled", context.Canceled
	} else if errors.Is(err, context.DeadlineExceeded) {
		code, cause = "timeout", context.DeadlineExceeded
	}
	return errors.WithStack(&crawlerTaskFailure{code: code, stage: stage, cause: cause})
}

// crawlerDiagnostic returns reviewed code and stage for err without its text.
func crawlerDiagnostic(err error) (string, string) {
	var failure *crawlerTaskFailure
	if errors.As(err, &failure) {
		return failure.code, failure.stage
	}
	return "worker_failed", "worker"
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
func fetchVerifiedCrawlerTask(ctx context.Context, task *rlibs.HTMLCrawlerTask) ([]byte, string, *crawleregress.Receipt, error) {
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
	if admission.Host != policy.Host || policy.MaxRedirects < 0 || policy.MaxRedirects > 20 {
		return nil, "", nil, crawlerFailure("policy_invalid", "admission", nil)
	}
	// An external browser proxy has no enforceable connection evidence. Refuse
	// it before transmitting the target instead of bypassing configured intent.
	if os.Getenv("CRAWLER_HTTP_PROXY") != "" {
		return nil, "", nil, crawlerFailure("unsupported_proxy", "admission", nil)
	}
	renderTask := *task
	renderTask.APIKey = ""
	renderTask.Egress = &policy
	renderTask.RequestChain = nil
	renderTask.EgressReceipt = nil
	renderTask.ResultHTML = nil
	renderTask.ResultMarkdown = nil
	htmlContent, receipt, err := fetchVerifiedHTMLContent(ctx, &renderTask)
	if err != nil {
		return nil, "", nil, crawlerFailure("render_failed", "render", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, "", nil, crawlerFailure("render_failed", "render", err)
	}
	if len(htmlContent) > rendererMaxResponseBytes {
		return nil, "", nil, crawlerFailure("result_too_large", "render", nil)
	}
	if err := crawleregress.VerifyReceipt(ctx, task.TaskID, task.Url, policy, receipt); err != nil {
		return nil, "", nil, crawlerFailure("evidence_rejected", "verification", err)
	}
	if isCloudflareChallenge(htmlContent) {
		return nil, "", nil, crawlerFailure("challenge", "render", nil)
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
	markdown, err := convertVerifiedMarkdown(rendered.String())
	if err != nil {
		return nil, "", crawlerFailure("conversion_failed", "conversion", err)
	}
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
	logger := gmw.GetLogger(ctx).Named("verified_crawler_task")
	// Secure rendering and local conversion have no use for a provider token.
	task.APIKey = ""
	task.ResultHTML, task.ResultMarkdown, task.RequestChain = nil, nil, nil
	task.EgressReceipt, task.FailedReason, task.FinishedAt = nil, nil, nil
	task.Status = rlibs.TaskStatusRunning
	if err := publishHTMLCrawlerTask(ctx, task); err != nil {
		return errors.Wrap(err, "publish crawler running state")
	}
	body, markdown, receipt, fetchErr := fetchVerifiedCrawlerTask(ctx, task)
	if err := ctx.Err(); err != nil {
		// Accepted work needs a terminal state even when its render context ends.
		// This bounded cleanup publishes no body and cannot resume any fallback.
		cleanupCtx, cancelCleanup := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelCleanup()
		now := time.Now().UTC()
		code, _ := crawlerDiagnostic(crawlerFailure("worker_failed", "render", err))
		task.Status, task.FailedReason, task.FinishedAt = rlibs.TaskStatusFailed, &code, &now
		if cleanupErr := publishHTMLCrawlerTask(cleanupCtx, task); cleanupErr != nil {
			logger.Warn("crawler terminal cleanup failed", zap.String("error_code", "cleanup_store_failed"),
				zap.String("stage", "persistence"))
		}
		return crawlerFailure("worker_failed", "render", err)
	}
	now := time.Now().UTC()
	task.FinishedAt = &now
	if fetchErr != nil {
		code, stage := crawlerDiagnostic(fetchErr)
		reason := code
		task.Status, task.FailedReason = rlibs.TaskStatusFailed, &reason
		logger.Error("verified crawler task failed", zap.Error(fetchErr),
			zap.String("error_code", code), zap.String("stage", stage))
	} else {
		task.Status, task.ResultHTML = rlibs.TaskStatusSuccess, body
		task.ResultMarkdown, task.EgressReceipt = []byte(markdown), receipt
		for _, origin := range receipt.Origins {
			task.RequestChain = append(task.RequestChain, origin.URL)
		}
		var markdownPtr *string
		if markdown != "" {
			markdownPtr = &markdown
		}
		record := &CrawlRecord{TaskID: task.TaskID, CrawledAt: now,
			URL: task.Url, RawBody: body, Markdown: markdownPtr}
		if err := persistVerifiedCrawlRecord(ctx, record); err != nil {
			logger.Warn("crawler audit persistence failed", zap.String("error_code", "audit_store_failed"),
				zap.String("stage", "persistence"))
		}
	}
	if err := publishHTMLCrawlerTask(ctx, task); err != nil {
		return errors.Wrap(err, "publish crawler terminal state")
	}
	return nil
}
