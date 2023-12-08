package http

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"regexp"
	"sync"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v5"
	gutils "github.com/Laisky/go-utils/v4"
	"github.com/Laisky/zap"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"golang.org/x/net/html"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"

	"github.com/Laisky/go-ramjet/library/log"
)

var (
	// chromedpSema chromedp cost too much memory, so limit it
	chromedpSema = semaphore.NewWeighted(2)
)

// fetchDynamicURLContent fetch dynamic url content, will render js by chromedp
func fetchDynamicURLContent(ctx context.Context, url string) (content []byte, err error) {
	logger := gmw.GetLogger(ctx).Named("fetch_dynamic_url_content").
		With(zap.String("url", url))
	logger.Debug("fetch dynamic url")
	headers := map[string]any{
		"User-Agent":      "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/58.0.3029.110 Safari/537",
		"Accept":          "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8",
		"Accept-Language": "en-US,en;q=0.8",
		"Accept-Encoding": "gzip, deflate, sdch",
		"Connection":      "keep-alive",
	}

	// give chrome more time to run in background
	chromeCtx, cancel := chromedp.NewContext(context.Background())
	defer cancel()

	if err = chromedpSema.Acquire(ctx, 1); err != nil {
		return nil, errors.Wrap(err, "acquire chromedp sema")
	} else {
		defer chromedpSema.Release(1)
	}

	var (
		finishCh = make(chan struct{})
		mu       sync.Mutex
	)
	go func() {
		defer close(finishCh)
		var htmlContent string
		if err = chromedp.Run(chromeCtx, chromedp.Tasks{
			network.Enable(),
			chromedp.Navigate(url),
			network.SetExtraHTTPHeaders(network.Headers(headers)),
			chromedp.Sleep(5 * time.Second), // Wait for the page to load
			chromedp.InnerHTML("html", &htmlContent, chromedp.ByQuery),
		}); err != nil {
			logger.Debug("fetch url first time failed, will try next time", zap.Error(err))
			if err = chromedp.Run(chromeCtx, chromedp.Tasks{
				network.Enable(),
				chromedp.Navigate(url),
				network.SetExtraHTTPHeaders(network.Headers(headers)),
				chromedp.Sleep(time.Second * 30), // Wait longer
				chromedp.InnerHTML("html", &htmlContent, chromedp.ByQuery),
			}); err != nil {
				logger.Warn("fetch url failed", zap.Error(err))
				return
			}
		}

		mu.Lock()
		content = []byte(htmlContent)
		mu.Unlock()

		logger.Info("fetch url success")
		urlContentCache.Store(url, content) // save cache
	}()

	select {
	case <-ctx.Done():
	case <-finishCh:
	}

	mu.Lock()
	defer mu.Unlock()

	if len(content) == 0 {
		return nil, errors.Errorf("no content find by chromedp")
	}

	if bodyContent, err := extractHTMLBody(content); err != nil {
		log.Logger.Warn("extract html body", zap.Error(err))
	} else {
		content = bodyContent
	}

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

	if content, err = _extractHtmlBody(resp.Body); err != nil {
		return nil, errors.Wrapf(err, "extract html body %q", url)
	}

	return content, nil
}

func googleSearch(ctx context.Context, query string) (content []byte, err error) {
	logger := gmw.GetLogger(ctx).Named("google_search")
	searchContent, err := fetchDynamicURLContent(ctx, "https://www.google.com/search?q="+query)
	if err != nil {
		return nil, errors.Wrapf(err, "fetch %q", query)
	}

	doc, err := html.Parse(bytes.NewReader(searchContent))
	if err != nil {
		return nil, errors.Wrap(err, "parse html")
	}

	ok, urls, err := _googleExtractor(doc)
	if err != nil {
		return nil, errors.Wrap(err, "extract google search result")
	}
	if !ok || len(urls) == 0 {
		return nil, errors.Errorf("no search result")
	}

	var (
		mu   sync.Mutex
		pool errgroup.Group
	)
	for _, url := range urls {
		url := url
		pool.Go(func() error {
			pageCnt, err := fetchStaticURLContent(ctx, url)
			if err != nil {
				return errors.Wrapf(err, "fetch %q", url)
			}

			mu.Lock()
			content = append(content, pageCnt...)
			content = append(content, '\n')
			defer mu.Unlock()

			return nil
		})
	}

	if err = pool.Wait(); err != nil {
		logger.Warn("fetch google search result", zap.Error(err))
	}

	if len(content) == 0 {
		return nil, errors.Errorf("no content find by google search")
	}

	return content, nil
}

var (
	regexpHref = regexp.MustCompile(`href="([^"]*)"`)
)

func _googleExtractor(n *html.Node) (ok bool, urls []string, err error) {
	if n.Type == html.ElementNode && n.Data == "div" {
		for _, attr := range n.Attr {
			if attr.Key == "id" && attr.Val == "search" {
				var buf bytes.Buffer
				if err = html.Render(&buf, n); err != nil {
					return false, nil, errors.WithStack(err)
				}

				matches := regexpHref.FindAllStringSubmatch(buf.String(), -1)
				for _, match := range matches {
					urls = append(urls, match[1])
				}

				return true, urls, nil
			}
		}
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if ok, urls, err = _googleExtractor(c); err != nil {
			return false, nil, errors.WithStack(err)
		} else if ok {
			return true, urls, nil
		}
	}

	return false, nil, nil
}

func _extractHtmlBody(body io.Reader) (bodyContent []byte, err error) {
	doc, err := html.Parse(body)
	if err != nil {
		return nil, errors.Wrap(err, "parse html")
	}

	var (
		f        func(*html.Node)
		bodyNode *html.Node
	)
	f = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "body" {
			bodyNode = n
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(doc)

	if bodyNode == nil {
		return nil, errors.New("no body node")
	}

	var buf bytes.Buffer
	if err = html.Render(&buf, bodyNode); err != nil {
		return nil, errors.Wrap(err, "render html")
	}

	return buf.Bytes(), nil
}
