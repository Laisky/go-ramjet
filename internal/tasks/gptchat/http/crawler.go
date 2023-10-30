package http

import (
	"context"
	"io"
	"net/http"
	"time"

	"github.com/Laisky/errors/v2"
	gutils "github.com/Laisky/go-utils/v4"
	"github.com/Laisky/zap"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"golang.org/x/sync/semaphore"

	"github.com/Laisky/go-ramjet/library/log"
)

var (
	// chromedpSema chromedp cost too much memory, so limit it
	chromedpSema = semaphore.NewWeighted(2)
)

// fetchDynamicURLContent fetch dynamic url content, will render js by chromedp
func fetchDynamicURLContent(ctx context.Context, url string) (content []byte, err error) {
	log.Logger.Debug("fetch dynamic url", zap.String("url", url))
	headers := map[string]any{
		"User-Agent": "go-ramjet-bot",
	}

	chromeCtx, cancel := chromedp.NewContext(ctx)
	defer cancel()

	if err = chromedpSema.Acquire(ctx, 1); err != nil {
		return nil, errors.Wrap(err, "acquire chromedp sema")
	} else {
		defer chromedpSema.Release(1)
	}

	var htmlContent string
	if err = chromedp.Run(chromeCtx, chromedp.Tasks{
		network.Enable(),
		chromedp.Navigate(url),
		network.SetExtraHTTPHeaders(network.Headers(headers)),
		chromedp.Sleep(5 * time.Second), // Wait for the page to load
		chromedp.InnerHTML("html", &htmlContent, chromedp.ByQuery),
	}); err != nil {
		return nil, errors.Wrapf(err, "run chrome task %q", url)
	}
	content = []byte(htmlContent)

	if bodyContent, err := extractHTMLBody(content); err != nil {
		log.Logger.Warn("extract html body", zap.Error(err))
	} else {
		content = bodyContent
	}

	urlContentCache.Store(url, content) // save cache
	return content, nil
}

// fetchStaticURLContent fetch static url content
func fetchStaticURLContent(ctx context.Context, url string) (content []byte, err error) {
	log.Logger.Debug("fetch static url", zap.String("url", url))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "new request %q", url)
	}
	req.Header.Set("User-Agent", "go-ramjet-bot")
	req.Header.Del("Accept-Encoding")

	resp, err := httpcli.Do(req) // nolint:bodyclose
	if err != nil {
		return nil, errors.Wrapf(err, "do request %q", url)
	}
	defer gutils.LogErr(resp.Body.Close, log.Logger)

	if resp.StatusCode != http.StatusOK {
		return nil, errors.Errorf("[%d]%s", resp.StatusCode, url)
	}

	if content, err = io.ReadAll(resp.Body); err != nil {
		return nil, errors.Wrap(err, "read response body")
	}

	return content, nil
}
