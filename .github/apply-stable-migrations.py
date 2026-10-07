from pathlib import Path


def replace(path, old, new):
    """replace applies one reviewed migration only when its source matches exactly."""
    path = Path(path)
    source = path.read_text()
    if source.count(old) != 1:
        raise ValueError(f'Expected one migration target in {path}: {old[:80]}')
    path.write_text(source.replace(old, new))


replace('internal/tasks/cv/pdf.go', '\n\t"github.com/yuin/goldmark"', '')
replace('internal/tasks/cv/pdf.go', '\tmarkdown goldmark.Markdown\n\ttmpl     *template.Template', '\tparser   parser.Parser\n\trenderer html.Renderer\n\ttmpl     *template.Template')
replace('internal/tasks/cv/pdf.go', '''	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
		goldmark.WithRendererOptions(html.WithUnsafe()),
	)

	return &CVPDFRenderer{
		markdown: md,
		tmpl:     tmpl,
	}, nil''', '''	return &CVPDFRenderer{
		parser: parser.New(
			parser.WithExtensions(extension.GFMParser),
			parser.WithAutoHeadingID(),
		),
		renderer: html.New(
			html.WithExtensions(extension.GFMHTMLRenderer),
			html.WithUnsafe(),
		),
		tmpl: tmpl,
	}, nil''')
replace('internal/tasks/cv/pdf.go', 'if err := r.markdown.Convert([]byte(content), &buf); err != nil {', 'source := []byte(content)\n\tdocument := r.parser.Parse(source)\n\tif err := r.renderer.Render(&buf, source, document); err != nil {')
path = Path('internal/tasks/cv/pdf.go')
source = path.read_text()
start = source.index('\tvar pdfData []byte\n', source.index('func renderHTMLToPDF'))
path.write_text(source[:start] + '''	if err := chromedp.Do(chromeCtx,
		chromedp.Navigate(dataURL),
		chromedp.WaitReady(chromedp.CSS("body")),
	); err != nil {
		return nil, errors.Wrap(err, "navigate PDF document")
	}

	// Await the font promise before printing, including its JavaScript errors.
	fonts, err := chromedp.Call(chromeCtx, runtime.Evaluate, runtime.EvaluateParams{
		Expression:   `document.fonts.ready.then(() => true)`,
		AwaitPromise: new(true),
	})
	if err != nil {
		return nil, errors.Wrap(err, "await document.fonts.ready")
	}
	if fonts.ExceptionDetails != nil {
		return nil, errors.WithStack(errors.New(fonts.ExceptionDetails.Error()))
	}

	result, err := chromedp.Call(chromeCtx, page.PrintToPDF, page.PrintToPDFParams{
		PrintBackground:   new(true),
		PreferCSSPageSize: new(true),
		TransferMode:      page.PrintToPDFTransferModeReturnAsBase64,
	})
	if err != nil {
		return nil, errors.Wrap(err, "print PDF document")
	}
	if len(result.Data) == 0 {
		return nil, errors.WithStack(errors.New("empty pdf output"))
	}

	return result.Data, nil
}
''')
path = Path('internal/tasks/gptchat/tasks/crawler.go')
source = path.read_text()
start = source.index('\terr = chromedp.Run(chromeCtx, chromedp.Tasks{')
end = source.index('\n\treturn htmlContent, nil', start)
path.write_text(source[:start] + '''	if _, err = chromedp.Call(chromeCtx, network.Enable, network.EnableParams{}); err != nil {
		return "", errors.Wrapf(err, "enable browser network for %q", targetURL)
	}
	if _, err = chromedp.Call(chromeCtx, network.SetExtraHTTPHeaders, network.SetExtraHTTPHeadersParams{
		Headers: network.Headers(headers),
	}); err != nil {
		return "", errors.Wrapf(err, "set browser headers for %q", targetURL)
	}

	err = chromedp.Do(chromeCtx,
		chromedp.Navigate(targetURL),
		chromedp.WaitReady(chromedp.CSS("body")),
		chromedp.Poll[chromedp.Void](`document.readyState === "complete"`,
			chromedp.WithPollingInterval(100*time.Millisecond),
			chromedp.WithPollingTimeout(0)),
		chromedp.Sleep(2*time.Second),
	)
	if err != nil {
		return "", errors.Wrapf(err, "run chromedp for %q", targetURL)
	}
	htmlContent, err = chromedp.Run(chromeCtx, chromedp.InnerHTML(chromedp.CSS("html")))
	if err != nil {
		return "", errors.Wrapf(err, "read browser HTML for %q", targetURL)
	}
''' + source[end:])
replace('internal/tasks/cv/pdf_letters.go', 'api.MergeRaw(readers, &merged, false, nil)', 'api.MergeRaw(ctx, readers, &merged, false, nil)')
for name in ['pdf_test.go', 'pdf_cjk_integration_test.go']:
    path = Path('internal/tasks/cv') / name
    path.write_text(path.read_text().replace('api.PageCount(bytes.NewReader(', 'api.PageCount(t.Context(), bytes.NewReader('))
replace('internal/tasks/gptchat/http/payment.go', '\tpi, err := paymentintent.New(params)', '\tparams.Context = c.Request.Context()\n\tpi, err := paymentintent.New(params)')
