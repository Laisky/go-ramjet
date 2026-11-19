// Package cv provides the CV task for managing resume content.
package cv

import (
	"bytes"
	"context"
	"encoding/base64"
	"html/template"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/yuin/goldmark/v2/extension"
	"github.com/yuin/goldmark/v2/parser"
	"github.com/yuin/goldmark/v2/renderer/html"
)

const (
	// pdfBrowserStartupTimeout bounds launching Chrome and attaching its first tab. It matches Puppeteer's default
	// launch timeout, exceeds the slowest completed cold start observed on hosted CI runners (about 21s), and is
	// longer than the former shared startup-plus-render deadline, so no launch that used to succeed can now fail.
	pdfBrowserStartupTimeout = 30 * time.Second
	// pdfRenderTimeout bounds the page work once the browser is ready, from navigation through printing.
	pdfRenderTimeout  = 25 * time.Second
	pdfViewportWidth  = 1280
	pdfViewportHeight = 720
)

const cvPDFTemplate = `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <title>{{ .Title }}</title>
    <link rel="preconnect" href="https://fonts.googleapis.com" />
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin />
    <link href="https://fonts.googleapis.com/css2?family=Newsreader:opsz,wght@6..72,400;6..72,600&family=Space+Grotesk:wght@400;500;600;700&family=Noto+Sans+SC:wght@400;500;700&family=Noto+Sans+TC:wght@400;500;700&family=Noto+Sans+JP:wght@400;500;700&family=Noto+Sans+KR:wght@400;500;700&family=Noto+Serif+SC:wght@400;600&family=Noto+Serif+TC:wght@400;600&family=Noto+Serif+JP:wght@400;600&family=Noto+Serif+KR:wght@400;600&display=swap" rel="stylesheet" />
    <style>
      :root {
        --cv-ink: #0b1020;
        --cv-muted: #4b5366;
        --cv-accent: #8f2d14;
        --cv-border: #d6d2c6;
        /* CJK + Latin unified stacks: per-glyph fallback walks the list. */
        --cv-font-serif: "Newsreader", "Times New Roman",
          "Noto Serif SC", "Noto Serif TC", "Noto Serif JP", "Noto Serif KR",
          "Noto Serif CJK SC", "Noto Serif CJK TC", "Noto Serif CJK JP", "Noto Serif CJK KR",
          "Source Han Serif SC", "Source Han Serif TC", "Source Han Serif",
          "PingFang SC", "PingFang TC", "Hiragino Mincho ProN", "Yu Mincho", "Songti SC",
          "Microsoft YaHei", "SimSun", serif;
        --cv-font-sans: "Space Grotesk", "Segoe UI",
          "Noto Sans SC", "Noto Sans TC", "Noto Sans JP", "Noto Sans KR",
          "Noto Sans CJK SC", "Noto Sans CJK TC", "Noto Sans CJK JP", "Noto Sans CJK KR",
          "Source Han Sans SC", "Source Han Sans TC", "Source Han Sans",
          "PingFang SC", "PingFang TC", "Hiragino Sans", "Yu Gothic", "Heiti SC",
          "Microsoft YaHei", sans-serif;
      }

      @page {
        size: A4;
        margin: 14mm;
      }

      * {
        box-sizing: border-box;
      }

      body {
        margin: 0;
        padding: 0;
        background: #ffffff;
        color: var(--cv-ink);
        font-family: var(--cv-font-serif);
        font-size: 12pt;
        line-height: 1.55;
      }

      h1,
      h2,
      h3,
      h4 {
        font-family: var(--cv-font-sans);
        letter-spacing: -0.01em;
      }

      h1 {
        font-size: 26pt;
        margin-bottom: 4pt;
      }

      h2 {
        font-size: 15.5pt;
        margin-top: 18pt;
        margin-bottom: 6pt;
        padding-bottom: 5pt;
        border-bottom: 1px solid var(--cv-border);
        break-before: auto;
        break-after: auto;
        page-break-before: auto;
        page-break-after: auto;
      }

      h3 {
        font-size: 12pt;
        margin-top: 12pt;
        margin-bottom: 4pt;
        break-before: auto;
        break-after: auto;
        page-break-before: auto;
        page-break-after: auto;
      }

      p {
        margin: 0 0 6pt;
      }

      a {
        color: var(--cv-accent);
        text-decoration: none;
      }

      ul {
        margin: 4pt 0 8pt;
        padding-left: 16pt;
        break-inside: auto;
        page-break-inside: auto;
      }

      li {
        margin-bottom: 3pt;
        break-inside: avoid;
        page-break-inside: avoid;
      }

      h2 + p,
      h2 + ul,
      h2 + ol,
      h3 + p,
      h3 + ul,
      h3 + ol {
        margin-top: 2pt;
      }

      .cv-container {
        padding: 0;
      }

      .cv-content {
        margin-top: 8pt;
      }

      .cv-summary {
        color: var(--cv-muted);
        font-size: 11pt;
      }
    </style>
  </head>
  <body>
    <div class="cv-container">
      <div class="cv-content">
        {{ .Content }}
      </div>
    </div>
  </body>
</html>`

// PDFRenderer renders markdown content into PDF bytes.
//
// Render converts the provided markdown content and returns the PDF bytes or an error.
type PDFRenderer interface {
	Render(ctx context.Context, content string) ([]byte, error)
}

// CVPDFRenderer converts markdown content into a styled PDF.
type CVPDFRenderer struct {
	parser   parser.Parser
	renderer html.Renderer
	tmpl     *template.Template
}

// NewCVPDFRenderer creates a CVPDFRenderer with markdown and HTML templates configured.
func NewCVPDFRenderer() (*CVPDFRenderer, error) {
	tmpl, err := template.New("cv_pdf").Parse(cvPDFTemplate)
	if err != nil {
		return nil, errors.Wrap(err, "parse cv pdf template")
	}

	return &CVPDFRenderer{
		parser: parser.New(
			parser.WithExtensions(extension.GFMParser),
			parser.WithAutoHeadingID(),
		),
		renderer: html.New(
			html.WithExtensions(extension.GFMHTMLRenderer),
			html.WithUnsafe(),
		),
		tmpl: tmpl,
	}, nil
}

// Render converts markdown content into a PDF document.
func (r *CVPDFRenderer) Render(ctx context.Context, content string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, errors.Wrap(err, "context done")
	}

	htmlBody, err := r.renderMarkdown(content)
	if err != nil {
		return nil, err
	}

	title := extractMarkdownTitle(content)
	htmlDoc, err := r.buildHTML(title, htmlBody)
	if err != nil {
		return nil, err
	}

	basePDF, err := renderHTMLToPDF(ctx, htmlDoc)
	if err != nil {
		return nil, err
	}

	lettersPDF, err := renderRecommendationLettersPDF(ctx, cvRecommendationLetters)
	if err != nil {
		return nil, errors.Wrap(err, "render recommendation letters pdf")
	}
	if len(lettersPDF) == 0 {
		return basePDF, nil
	}

	mergedPDF, err := mergePDFBytes(ctx, basePDF, lettersPDF)
	if err != nil {
		return nil, errors.Wrap(err, "merge cv pdf with recommendation letters")
	}

	return mergedPDF, nil
}

// renderMarkdown converts markdown into HTML.
func (r *CVPDFRenderer) renderMarkdown(content string) (string, error) {
	var buf bytes.Buffer
	source := []byte(content)
	document := r.parser.Parse(source)
	if err := r.renderer.Render(&buf, source, document); err != nil {
		return "", errors.Wrap(err, "render markdown")
	}

	return buf.String(), nil
}

// buildHTML wraps rendered markdown HTML in the PDF template.
// It takes the title and HTML body content and returns the HTML document or an error.
func (r *CVPDFRenderer) buildHTML(title string, bodyHTML string) (string, error) {
	var buf bytes.Buffer
	data := struct {
		Title   string
		Content template.HTML
	}{
		Title:   title,
		Content: template.HTML(bodyHTML),
	}

	if err := r.tmpl.Execute(&buf, data); err != nil {
		return "", errors.Wrap(err, "execute cv pdf template")
	}

	return buf.String(), nil
}

// extractMarkdownTitle extracts the first H1 heading from markdown content.
func extractMarkdownTitle(content string) string {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))
		}
	}
	return "CV"
}

// renderHTMLToPDF renders HTML into PDF bytes using headless Chrome.
func renderHTMLToPDF(ctx context.Context, htmlContent string) ([]byte, error) {
	return renderHTMLToPDFWithRenderer(ctx, htmlContent, renderHTMLToPDFInBrowser)
}

// renderHTMLToPDFWithRenderer rejects ended contexts before invoking renderer and otherwise preserves its inputs, output and error.
func renderHTMLToPDFWithRenderer(ctx context.Context, htmlContent string, renderer func(context.Context, string) ([]byte, error)) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, errors.Wrap(err, "context done")
	}
	return renderer(ctx, htmlContent)
}

// pdfBrowserBudgets holds the independent browser startup and page rendering limits for one PDF render.
type pdfBrowserBudgets struct {
	startup time.Duration
	render  time.Duration
}

// defaultPDFBrowserBudgets are the production limits: a dedicated startup budget and an unchanged render budget.
var defaultPDFBrowserBudgets = pdfBrowserBudgets{startup: pdfBrowserStartupTimeout, render: pdfRenderTimeout}

// renderHTMLToPDFInBrowser renders HTML with the production startup and render budgets.
func renderHTMLToPDFInBrowser(ctx context.Context, htmlContent string) ([]byte, error) {
	return renderHTMLToPDFInBrowserWithBudgets(ctx, htmlContent, defaultPDFBrowserBudgets)
}

// startPDFBrowserWithin bounds a browser launch by its own budget without binding the launched browser to it.
//
// chromedp ties the browser lifetime to the context of the first Run, so a timeout on that context would also
// stop the browser after startup. Instead, ctx is the caller context, budget limits only the launch, abort cancels
// the browser lifetime context with a cause when the budget elapses, and start launches the browser and receives a
// context that carries the startup deadline for observations. It returns nil once the browser is ready, the
// wrapped start error, or a wrapped context.DeadlineExceeded cause when the startup budget elapsed first.
func startPDFBrowserWithin(ctx context.Context, budget time.Duration, abort context.CancelCauseFunc, start func(startupCtx context.Context) error) error {
	expired := errors.Wrapf(context.DeadlineExceeded, "PDF browser startup exceeded %s", budget)
	startupCtx, cancelStartup := context.WithTimeoutCause(ctx, budget, expired)
	defer cancelStartup()
	stopWatchdog := context.AfterFunc(startupCtx, func() { abort(context.Cause(startupCtx)) })
	err := start(startupCtx)
	if !stopWatchdog() {
		// The budget elapsed or the caller ended first, and the browser lifetime has already been aborted.
		return errors.Wrap(context.Cause(startupCtx), "start PDF browser")
	}
	if err != nil {
		return errors.Wrap(err, "start PDF browser")
	}
	return nil
}

// renderHTMLToPDFInBrowserWithBudgets renders HTML to PDF bytes with headless Chrome.
//
// The browser lives on the caller context, startup is limited by budgets.startup, and budgets.render starts only
// once the browser is ready, so a slow launch never shortens the time left for navigation, fonts and printing.
// It returns the PDF bytes, or an error from startup, a rendering phase, or an empty print result.
func renderHTMLToPDFInBrowserWithBudgets(ctx context.Context, htmlContent string, budgets pdfBrowserBudgets) ([]byte, error) {
	encoded := base64.StdEncoding.EncodeToString([]byte(htmlContent))
	dataURL := "data:text/html;base64," + encoded

	probeStarted := time.Now()
	probe := cvPDFPrintProbeFromContext(ctx)

	allocatorOptions := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.NoDefaultBrowserCheck,
		chromedp.NoFirstRun,
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.WindowSize(pdfViewportWidth, pdfViewportHeight),
	)
	// Explicit browser choice belongs only to the owned synthetic probe; ordinary rendering retains implicit selection.
	if executable, ok := ctx.Value(cvPDFPrintProbeBrowserKey{}).(string); ok && probe != nil {
		allocatorOptions = append(allocatorOptions, chromedp.ExecPath(executable))
	}
	browserCtx, abortBrowser := context.WithCancelCause(ctx)
	defer abortBrowser(nil)
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(browserCtx, allocatorOptions...)
	defer cancelAlloc()

	chromeCtx, cancelChrome := chromedp.NewContext(allocCtx)
	defer cancelChrome()

	if err := startPDFBrowserWithin(ctx, budgets.startup, abortBrowser, func(startupCtx context.Context) error {
		phaseStarted := time.Now()
		emitCVPDFPrintProbe(startupCtx, probe, "startup", "begin", probeStarted, phaseStarted, "")
		if err := chromedp.Do(chromeCtx); err != nil {
			emitCVPDFPrintProbe(startupCtx, probe, "startup", "failure", probeStarted, phaseStarted, "")
			return errors.WithStack(err)
		}
		emitCVPDFPrintProbe(startupCtx, probe, "startup", "success", probeStarted, phaseStarted, "")
		return nil
	}); err != nil {
		return nil, err
	}

	// Derived from the started browser context, the render deadline ends rendering actions without stopping Chrome.
	renderCtx, cancelRender := context.WithTimeout(chromeCtx, budgets.render)
	defer cancelRender()
	// This one explicitly measured metadata command exists only for observer-enabled probe contexts.
	observeCVPDFBrowserVersion(renderCtx, probe, probeStarted)
	phaseStarted := time.Now()
	emitCVPDFPrintProbe(renderCtx, probe, "navigation", "begin", probeStarted, phaseStarted, "")
	if err := chromedp.Do(renderCtx, chromedp.Navigate(dataURL)); err != nil {
		emitCVPDFPrintProbe(renderCtx, probe, "navigation", "failure", probeStarted, phaseStarted, "")
		return nil, errors.Wrap(err, "navigate PDF document")
	}
	emitCVPDFPrintProbe(renderCtx, probe, "navigation", "success", probeStarted, phaseStarted, "")
	phaseStarted = time.Now()
	emitCVPDFPrintProbe(renderCtx, probe, "body_ready", "begin", probeStarted, phaseStarted, "")
	if err := chromedp.Do(renderCtx, chromedp.WaitReady(chromedp.CSS("body"))); err != nil {
		emitCVPDFPrintProbe(renderCtx, probe, "body_ready", "failure", probeStarted, phaseStarted, "")
		return nil, errors.Wrap(err, "wait for PDF document body")
	}
	emitCVPDFPrintProbe(renderCtx, probe, "body_ready", "success", probeStarted, phaseStarted, "")

	// Await the font promise before printing, including its JavaScript errors.
	phaseStarted = time.Now()
	emitCVPDFPrintProbe(renderCtx, probe, "fonts", "begin", probeStarted, phaseStarted, "")
	fonts, err := chromedp.Call(renderCtx, runtime.Evaluate, runtime.EvaluateParams{
		Expression:   `document.fonts.ready.then(() => true)`,
		AwaitPromise: new(true),
	})
	if err != nil {
		emitCVPDFPrintProbe(renderCtx, probe, "fonts", "failure", probeStarted, phaseStarted, "")
		return nil, errors.Wrap(err, "await document.fonts.ready")
	}
	if fonts.ExceptionDetails != nil {
		emitCVPDFPrintProbe(renderCtx, probe, "fonts", "failure", probeStarted, phaseStarted, "")
		return nil, errors.WithStack(&chromedp.ExceptionError{ExceptionDetails: fonts.ExceptionDetails})
	}

	emitCVPDFPrintProbe(renderCtx, probe, "fonts", "success", probeStarted, phaseStarted, "")
	phaseStarted = time.Now()
	emitCVPDFPrintProbe(renderCtx, probe, "print", "begin", probeStarted, phaseStarted, "")
	result, err := chromedp.Call(renderCtx, page.PrintToPDF, page.PrintToPDFParams{
		PrintBackground:   new(true),
		PreferCSSPageSize: new(true),
		TransferMode:      page.PrintToPDFTransferModeReturnAsBase64,
	})
	if err != nil {
		emitCVPDFPrintProbe(renderCtx, probe, "print", "failure", probeStarted, phaseStarted, "")
		return nil, errors.Wrap(err, "print PDF document")
	}
	if len(result.Data) == 0 {
		emitCVPDFPrintProbe(renderCtx, probe, "print", "failure", probeStarted, phaseStarted, "")
		return nil, errors.WithStack(errors.New("empty pdf output"))
	}

	emitCVPDFPrintProbe(renderCtx, probe, "print", "success", probeStarted, phaseStarted, "")
	return result.Data, nil
}
