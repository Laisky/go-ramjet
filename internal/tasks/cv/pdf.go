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
	"github.com/chromedp/chromedp"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
)

const (
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
    <link href="https://fonts.googleapis.com/css2?family=Newsreader:opsz,wght@6..72,400;6..72,600&family=Space+Grotesk:wght@400;500;600;700&display=swap" rel="stylesheet" />
    <style>
      :root {
        --cv-ink: #0b1020;
        --cv-muted: #5c6478;
        --cv-accent: #9a3412;
        --cv-border: #ddd7cc;
        --cv-paper: #fdfaf5;
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
        background: var(--cv-paper);
        color: var(--cv-ink);
        font-family: "Newsreader", "Times New Roman", serif;
        font-size: 12pt;
        line-height: 1.55;
      }

      h1,
      h2,
      h3,
      h4 {
        font-family: "Space Grotesk", "Segoe UI", sans-serif;
        letter-spacing: -0.01em;
      }

      h1 {
        font-size: 26pt;
        margin-bottom: 6pt;
      }

      h2 {
        font-size: 16pt;
        margin-top: 22pt;
        margin-bottom: 8pt;
        padding-bottom: 6pt;
        border-bottom: 1px solid var(--cv-border);
      }

      h3 {
        font-size: 12.5pt;
        margin-top: 14pt;
        margin-bottom: 4pt;
      }

      p {
        margin: 0 0 8pt;
      }

      a {
        color: var(--cv-accent);
        text-decoration: none;
      }

      ul {
        margin: 6pt 0 10pt;
        padding-left: 18pt;
      }

      li {
        margin-bottom: 4pt;
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

// PDFService renders CV markdown into PDF and uploads it to object storage.
type PDFService struct {
	renderer *CVPDFRenderer
	store    *S3PDFStore
}

// NewPDFService creates a PDFService using the provided renderer and store.
func NewPDFService(renderer *CVPDFRenderer, store *S3PDFStore) (*PDFService, error) {
	if renderer == nil {
		return nil, errors.WithStack(errors.New("pdf renderer is nil"))
	}
	if store == nil {
		return nil, errors.WithStack(errors.New("pdf store is nil"))
	}

	return &PDFService{
		renderer: renderer,
		store:    store,
	}, nil
}

// RenderAndStore renders markdown into a PDF and persists it to object storage.
func (s *PDFService) RenderAndStore(ctx context.Context, content string) error {
	if err := ctx.Err(); err != nil {
		return errors.Wrap(err, "context done")
	}

	pdfBytes, err := s.renderer.Render(ctx, content)
	if err != nil {
		return errors.Wrap(err, "render cv pdf")
	}

	if err := s.store.Save(ctx, pdfBytes); err != nil {
		return errors.Wrap(err, "save cv pdf")
	}

	return nil
}

// CVPDFRenderer converts markdown content into a styled PDF.
type CVPDFRenderer struct {
	markdown goldmark.Markdown
	tmpl     *template.Template
}

// NewCVPDFRenderer creates a CVPDFRenderer with markdown and HTML templates configured.
func NewCVPDFRenderer() (*CVPDFRenderer, error) {
	tmpl, err := template.New("cv_pdf").Parse(cvPDFTemplate)
	if err != nil {
		return nil, errors.Wrap(err, "parse cv pdf template")
	}

	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
		goldmark.WithRendererOptions(html.WithUnsafe()),
	)

	return &CVPDFRenderer{
		markdown: md,
		tmpl:     tmpl,
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

	return renderHTMLToPDF(ctx, htmlDoc)
}

// renderMarkdown converts markdown into HTML.
func (r *CVPDFRenderer) renderMarkdown(content string) (string, error) {
	var buf bytes.Buffer
	if err := r.markdown.Convert([]byte(content), &buf); err != nil {
		return "", errors.Wrap(err, "render markdown")
	}

	return buf.String(), nil
}

// buildHTML wraps rendered markdown HTML in the PDF template.
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
	if err := ctx.Err(); err != nil {
		return nil, errors.Wrap(err, "context done")
	}

	encoded := base64.StdEncoding.EncodeToString([]byte(htmlContent))
	dataURL := "data:text/html;base64," + encoded

	renderCtx, cancel := context.WithTimeout(ctx, pdfRenderTimeout)
	defer cancel()

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(
		renderCtx,
		append(chromedp.DefaultExecAllocatorOptions[:],
			chromedp.NoDefaultBrowserCheck,
			chromedp.NoFirstRun,
			chromedp.Flag("headless", true),
			chromedp.Flag("disable-gpu", true),
			chromedp.Flag("disable-dev-shm-usage", true),
			chromedp.Flag("no-sandbox", true),
			chromedp.WindowSize(pdfViewportWidth, pdfViewportHeight),
		)...,
	)
	defer cancelAlloc()

	chromeCtx, cancelChrome := chromedp.NewContext(allocCtx)
	defer cancelChrome()

	var pdfData []byte
	if err := chromedp.Run(
		chromeCtx,
		chromedp.Navigate(dataURL),
		chromedp.WaitReady("body", chromedp.ByQuery),
		chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			pdfData, _, err = page.PrintToPDF().
				WithPrintBackground(true).
				WithPreferCSSPageSize(true).
				WithTransferMode(page.PrintToPDFTransferModeReturnAsBase64).
				Do(ctx)
			if err != nil {
				return err
			}
			if len(pdfData) == 0 {
				return errors.WithStack(errors.New("empty pdf output"))
			}
			return nil
		}),
	); err != nil {
		return nil, errors.Wrap(err, "run chromedp")
	}

	return pdfData, nil
}
