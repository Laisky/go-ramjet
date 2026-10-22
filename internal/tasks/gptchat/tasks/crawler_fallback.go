package tasks

import (
	"bytes"
	"context"
	"net/url"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/laisky-blog-graphql/library/crawleregress"
	rlibs "github.com/Laisky/laisky-blog-graphql/library/db/redis"
	"github.com/Laisky/zap"
	"github.com/yuin/goldmark/v2/extension"
	"github.com/yuin/goldmark/v2/parser"
	htmlrenderer "github.com/yuin/goldmark/v2/renderer/html"
)

// crawlerContent holds accepted content and its actual acquisition provenance.
// A provider result deliberately has no direct connection receipt.
type crawlerContent struct {
	body        []byte
	markdown    string
	receipt     *crawleregress.Receipt
	acquisition *crawleregress.Acquisition
}

// crawlerEnforcementError marks a direct-route denial that must never cause
// transmission of the target to a provider. Unwrap preserves the original cause.
type crawlerEnforcementError struct{ cause error }

// Error returns the existing enforcement error text without changing diagnostics.
func (e *crawlerEnforcementError) Error() string { return e.cause.Error() }

// Unwrap returns the original policy, peer, evidence, or resource-limit failure.
func (e *crawlerEnforcementError) Unwrap() error { return e.cause }

// crawlerEnforcementFailure preserves a terminal direct-route boundary failure.
func crawlerEnforcementFailure(err error) error {
	return errors.WithStack(&crawlerEnforcementError{cause: err})
}

// crawlerProviderAttemptError records a safe provider identifier and finite outcome.
// Raw provider errors may contain response bodies and are not rendered as text.
type crawlerProviderAttemptError struct {
	provider, code string
	cause          error
}

// Error returns the provider and outcome without any response or credential data.
func (e *crawlerProviderAttemptError) Error() string { return "provider " + e.provider + ": " + e.code }

// Unwrap preserves cancellation and typed transport causes for internal checks.
func (e *crawlerProviderAttemptError) Unwrap() error { return e.cause }

// acquireCrawlerContent attempts direct acquisition, then configured providers
// for eligible failures within the same worker context and remaining budget.
func acquireCrawlerContent(ctx context.Context, task *rlibs.HTMLCrawlerTask) (*crawlerContent, error) {
	body, markdown, receipt, directErr := fetchVerifiedCrawlerTask(ctx, task)
	if directErr != nil {
		if !crawlerFallbackEligible(ctx, task, directErr) {
			return nil, directErr
		}
		return acquireCrawlerProviderContent(ctx, task, directErr)
	}
	if task.AllowProviderFallback && !task.OutputMarkdown && !crawlerHTMLHasContent(body) {
		return acquireCrawlerProviderContent(ctx, task, crawlerFailure("empty_body", "conversion", nil))
	}
	return &crawlerContent{body: body, markdown: markdown, receipt: receipt,
		acquisition: &crawleregress.Acquisition{Method: crawleregress.AcquisitionDirect}}, nil
}

// crawlerFallbackEligible separates content/transport failures from terminal
// enforcement failures; it never treats cancellation as permission to retry.
func crawlerFallbackEligible(ctx context.Context, task *rlibs.HTMLCrawlerTask, err error) bool {
	if ctx.Err() != nil || task == nil || !task.AllowProviderFallback || task.Egress == nil {
		return false
	}
	var denied *crawlerEnforcementError
	if errors.As(err, &denied) || errors.Is(err, crawleregress.ErrRejected) || errors.Is(err, crawleregress.ErrUnverified) {
		return false
	}
	code, stage := crawlerDiagnostic(err)
	switch stage {
	case "render":
		return code == "render_failed" || code == "timeout" || code == "challenge" || code == "empty_body"
	case "conversion":
		return code == "conversion_failed" || code == "empty_body" || code == "empty_markdown"
	default:
		return false
	}
}

// crawlerProviderURLPermitted is the single provider-disclosure boundary for
// verified fallback. It deliberately does not guess which query names contain
// credentials: any query requires an explicit operator policy choice. Userinfo
// and fragments are never needed for server-side acquisition by a provider.
func crawlerProviderURLPermitted(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.User != nil || parsed.Fragment != "" || parsed.RawFragment != "" {
		return false
	}
	if parsed.RawQuery != "" || parsed.ForceQuery {
		return currentWebFetchConfig().AllowQueryForwarding
	}
	return true
}

// acquireCrawlerProviderContent reuses configured adapters and priority order,
// accepting only usable content with explicit provider provenance.
func acquireCrawlerProviderContent(ctx context.Context, task *rlibs.HTMLCrawlerTask, directErr error) (*crawlerContent, error) {
	if !crawlerProviderURLPermitted(task.Url) {
		gmw.GetLogger(ctx).Named("crawler_provider_fallback").Debug("crawler provider fallback skipped by URL disclosure policy",
			zap.String("task_id", crawlerLogTaskID(task.TaskID)), zap.String("reason", "target_url_not_authorized"))
		// Keep the original direct failure and diagnostics. In particular, do
		// not transmit a sanitized but semantically different URL to a provider.
		return nil, directErr
	}
	proxies, err := registeredWebFetchProxies()
	if err != nil {
		return nil, crawlerFailure("render_failed", "render", errors.Join(directErr,
			&crawlerProviderAttemptError{provider: "configuration", code: "invalid_configuration"}))
	}
	if len(proxies) == 0 {
		return nil, directErr
	}
	orderWebFetchProxiesByPriority(proxies)
	logger := gmw.GetLogger(ctx).Named("crawler_provider_fallback")
	code, stage := crawlerDiagnostic(directErr)
	logger.Debug("crawler provider fallback started", zap.String("task_id", crawlerLogTaskID(task.TaskID)),
		zap.String("direct_error_code", code), zap.String("direct_stage", stage))
	ctx = withVerifiedWebFetchClient(ctx, rendererNetworkDependencies{})
	attempts := []error{directErr}
	for _, proxy := range proxies {
		if err := ctx.Err(); err != nil {
			return nil, crawlerFailure("render_failed", "render", err)
		}
		name := proxy.Name()
		if !crawleregress.ValidProviderName(name) {
			attempts = append(attempts, &crawlerProviderAttemptError{provider: "configuration", code: "invalid_provider"})
			continue
		}
		content, fetchErr := proxy.Fetch(ctx, task.Url)
		if err := ctx.Err(); err != nil {
			return nil, crawlerFailure("render_failed", "render", err)
		}
		outcome := "request_failed"
		var result *crawlerContent
		if fetchErr != nil {
			// Adapter errors are retained as typed causes, never emitted as payloads.
		} else {
			result, fetchErr = prepareCrawlerProviderContent(ctx, content, task.OutputMarkdown)
			outcome = "unusable_content"
		}
		if fetchErr != nil {
			attempt := &crawlerProviderAttemptError{provider: name, code: outcome, cause: fetchErr}
			attempts = append(attempts, attempt)
			logger.Debug("crawler provider attempt failed", zap.String("task_id", crawlerLogTaskID(task.TaskID)),
				zap.String("provider", name), zap.String("stage", "provider"), zap.String("error_code", outcome))
			continue
		}
		result.acquisition = &crawleregress.Acquisition{Method: crawleregress.AcquisitionProvider, Provider: name}
		logger.Info("crawler provider content acquired", zap.String("task_id", crawlerLogTaskID(task.TaskID)),
			zap.String("acquisition_method", crawleregress.AcquisitionProvider), zap.String("provider", name))
		return result, nil
	}
	return nil, crawlerFailure("render_failed", "render", errors.Join(attempts...))
}

// prepareCrawlerProviderContent validates provider output and converts Markdown
// to HTML locally for the HTML result field, preserving Markdown bytes as text.
func prepareCrawlerProviderContent(ctx context.Context, content string, outputMarkdown bool) (*crawlerContent, error) {
	if err := ctx.Err(); err != nil {
		return nil, errors.WithStack(err)
	}
	if len(content) > rendererMaxResponseBytes {
		return nil, errors.New("provider response exceeds crawler limit")
	}
	content = strings.TrimSpace(content)
	if content == "" || providerReportsTargetFailure(content) {
		return nil, errors.New("provider returned empty or unsuccessful content")
	}
	var body []byte
	markdown := content
	if crawlerProviderIsHTML(content) {
		if isCloudflareChallenge(content) {
			return nil, errors.New("provider returned active challenge content")
		}
		var err error
		// Provider conversion must not replace the original direct-render capture.
		conversionCtx := context.WithValue(ctx, crawlerCaptureKey{}, (*crawlerFailureCapture)(nil))
		conversionCtx = context.WithValue(conversionCtx, crawlerContextKey{}, (*crawlerContextObservation)(nil))
		body, markdown, err = extractVerifiedHTMLBody(conversionCtx, content, outputMarkdown)
		if err != nil {
			return nil, err
		}
	} else {
		source := []byte(content)
		document := parser.New(parser.WithExtensions(extension.GFMParser)).Parse(source)
		var rendered bytes.Buffer
		// Parse Markdown quotes/code before detecting active HTML. The detector
		// needs raw HTML; published HTML retains the default safe rendering.
		if err := htmlrenderer.New(htmlrenderer.WithExtensions(extension.GFMHTMLRenderer), htmlrenderer.WithUnsafe()).Render(&rendered, source, document); err != nil {
			return nil, errors.Wrap(err, "render provider Markdown for challenge detection")
		}
		if rendered.Len() > rendererMaxResponseBytes || isCloudflareChallenge(rendered.String()) {
			return nil, errors.New("provider returned oversized or active challenge content")
		}
		rendered.Reset()
		if err := htmlrenderer.New(htmlrenderer.WithExtensions(extension.GFMHTMLRenderer)).Render(&rendered, source, document); err != nil {
			return nil, errors.Wrap(err, "render provider Markdown locally")
		}
		body = append([]byte("<body>"), rendered.Bytes()...)
		body = append(body, []byte("</body>")...)
	}
	if err := ctx.Err(); err != nil {
		return nil, errors.WithStack(err)
	}
	if len(body) > rendererMaxResponseBytes || len(markdown) > rendererMaxResponseBytes {
		return nil, errors.New("converted provider content exceeds crawler limit")
	}
	if !crawlerHTMLHasContent(body) {
		return nil, errors.New("provider content has no usable body")
	}
	if !outputMarkdown {
		markdown = ""
	}
	return &crawlerContent{body: body, markdown: markdown}, nil
}

// providerReportsTargetFailure recognizes a provider's structured failure
// preamble without treating ordinary articles about HTTP errors as failures.
func providerReportsTargetFailure(content string) bool {
	metadata, _, found := strings.Cut(content, "\nMarkdown Content:")
	if !found || !strings.HasPrefix(content, "Title:") || !strings.Contains(metadata, "\nURL Source:") {
		return false
	}
	for _, line := range strings.Split(metadata, "\n") {
		if strings.HasPrefix(line, "Warning: Target URL returned error ") {
			return true
		}
	}
	return false
}
