package tasks

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v6"
	"github.com/Laisky/go-ramjet/library/log"
	rutils "github.com/Laisky/go-ramjet/library/redis"
	gredis "github.com/Laisky/go-redis/v2"
	rlibs "github.com/Laisky/laisky-blog-graphql/library/db/redis"
	"github.com/Laisky/zap"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"golang.org/x/net/html"
	"golang.org/x/sync/semaphore"
)

var (
	defaultChromedpSemaLimit = 2
	// chromedpSema chromedp cost too much memory, so limit it
	chromedpSema *semaphore.Weighted
)

func init() {
	// set chromedp sema limit
	if limit := os.Getenv("CHROMEDP_SEMA_LIMIT"); limit != "" {
		if v, err := strconv.Atoi(limit); err == nil {
			defaultChromedpSemaLimit = v
		}
	}
	chromedpSema = semaphore.NewWeighted(int64(defaultChromedpSemaLimit))
	log.Logger.Info("init chromedp sema", zap.Int("limit", defaultChromedpSemaLimit))
}

// RunDynamicWebCrawler is the entry for dynamic web crawler
func RunDynamicWebCrawler() {
	for range defaultChromedpSemaLimit {
		go func() {
			log.Logger.Named("dynamic_web_crawler").Info("start")
			for {
				if err := runDynamicWebCrawler(); err != nil {
					if !gredis.IsNil(err) {
						log.Logger.Error("run dynamic web crawler", zap.Error(err))
					}
				}

				time.Sleep(time.Second)
			}
		}()
	}
}

func runDynamicWebCrawler() error {
	ctxCrawler, cancelCrawler := context.WithTimeout(context.Background(), time.Minute)
	defer cancelCrawler()

	task, err := rutils.GetCli().GetHTMLCrawlerTask(ctxCrawler)
	if err != nil {
		return errors.Wrap(err, "get html crawler task")
	}

	if task == nil {
		return errors.New("html crawler task is nil")
	}

	logger := log.Logger.Named("run_dynamic_web_crawler").With(
		zap.String("task_id", task.TaskID),
		zap.String("url", task.Url))
	logger.Info("get task")

	resultKey := rlibs.KeyPrefixTaskHTMLCrawlerResult + task.TaskID

	content, err := dynamicFetchWorker(ctxCrawler, task.Url)
	if err != nil {
		now := time.Now()
		reason := fmt.Sprintf("fetch url %q", task.Url)

		task.Status = rlibs.TaskStatusFailed
		task.FinishedAt = &now
		task.FailedReason = &reason
		logger.Error("dynamic fetch url", zap.Error(err))
	} else {
		now := time.Now()
		task.ResultHTML = content
		task.Status = rlibs.TaskStatusSuccess
		task.FinishedAt = &now
		logger.Info("success dynamic fetch url")
	}

	payload, err := task.ToString()
	if err != nil {
		return errors.Wrap(err, "serialize task")
	}

	ctxPublish, cancelPublish := context.WithTimeout(context.Background(), time.Second*30)
	defer cancelPublish()

	err = rutils.GetCli().GetDB().
		Set(ctxPublish, resultKey, payload, time.Hour*24*7).Err()
	if err != nil {
		return errors.Wrapf(err, "set task result %q", resultKey)
	}

	return nil
}

type fetchURLOption struct {
	duration time.Duration
}

func (o *fetchURLOption) apply(opts ...FetchURLOption) (*fetchURLOption, error) {
	// set default
	o.duration = 10 * time.Second

	// apply options
	for _, opt := range opts {
		if err := opt(o); err != nil {
			return nil, err
		}
	}

	return o, nil
}

type FetchURLOption func(*fetchURLOption) error

func WithDuration(duration time.Duration) FetchURLOption {
	return func(opt *fetchURLOption) error {
		opt.duration = duration
		return nil
	}
}

// dynamicFetchWorker fetch dynamic url content, will render js by chromedp
func dynamicFetchWorker(ctx context.Context, url string, opts ...FetchURLOption) (content []byte, err error) {
	startAt := time.Now()
	logger := gmw.GetLogger(ctx).Named("fetch_dynamic_url_content").
		With(zap.String("url", url))

	// opt, err := new(fetchURLOption).apply(opts...)
	// if err != nil {
	// 	return nil, errors.Wrap(err, "apply options")
	// }

	logger.Debug("fetch dynamic url")
	headers := map[string]any{
		//nolint: lll
		"User-Agent":      "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/58.0.3029.110 Safari/537",
		"Accept":          "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8",
		"Accept-Language": "en-US,en;q=0.8",
		"Accept-Encoding": "gzip, deflate, sdch",
		"Connection":      "keep-alive",
	}

	if err = chromedpSema.Acquire(ctx, 1); err != nil {
		return nil, errors.Wrap(err, "acquire chromedp sema")
	} else {
		defer chromedpSema.Release(1)
	}

	// create chrome options with proxy settings
	chromeOpts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.NoDefaultBrowserCheck,
		chromedp.NoFirstRun,
		chromedp.WindowSize(1920, 1080),
	)
	if os.Getenv("CRAWLER_HTTP_PROXY") != "" {
		logger.Debug("set proxy", zap.String("proxy", os.Getenv("CRAWLER_HTTP_PROXY")))
		chromeOpts = append(chromeOpts, chromedp.ProxyServer(os.Getenv("CRAWLER_HTTP_PROXY")))
	}

	allocCtx, cancel := chromedp.NewExecAllocator(ctx, chromeOpts...)
	defer cancel()

	// create a chrome instance
	chromeCtx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	var htmlContent string
	err = chromedp.Run(chromeCtx, chromedp.Tasks{
		network.Enable(),
		// Set headers before navigation!
		network.SetExtraHTTPHeaders(network.Headers(headers)),
		chromedp.Navigate(url),
		chromedp.WaitReady("body", chromedp.ByQuery),
		// Wait for document.readyState to be complete
		chromedp.ActionFunc(func(ctx context.Context) error {
			var readyState string
			for {
				if err := chromedp.Evaluate("document.readyState", &readyState).Do(ctx); err != nil {
					return err
				}
				if readyState == "complete" {
					break
				}
				time.Sleep(100 * time.Millisecond)
			}
			return nil
		}),
		// Additional wait: Let dynamic JS scripts finish executing.
		// Adjust the sleep duration or use more advanced conditions as needed.
		chromedp.Sleep(2 * time.Second),
		// Get the full HTML after dynamic scripts have rendered
		chromedp.InnerHTML("html", &htmlContent, chromedp.ByQuery),
	})

	if err != nil {
		return nil, errors.Wrapf(err, "run chromedp for %q", url)
	}

	content = []byte(htmlContent)
	if len(content) == 0 {
		return nil, errors.Errorf("no content found by chromedp for %q", url)
	}

	if bodyContent, err := ExtractHTMLBody(content); err != nil {
		log.Logger.Warn("extract html body", zap.Error(err))
	} else {
		content = bodyContent
	}

	logger.Info("succeed fetch dynamic url",
		zap.Int("len", len(content)),
		zap.Duration("cost_secs", time.Since(startAt)))

	return content, nil
}

// findHTMLBody find html body recursively
func findHTMLBody(n *html.Node) *html.Node {
	if n.Type == html.ElementNode && n.Data == "body" {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if body := findHTMLBody(c); body != nil {
			return body
		}
	}
	return nil
}

// ExtractHTMLBody extract body from html
func ExtractHTMLBody(content []byte) (bodyContent []byte, err error) {
	parsedHTML, err := html.Parse(bytes.NewReader(content))
	if err != nil {
		return nil, errors.Wrap(err, "parse html")
	}

	body := findHTMLBody(parsedHTML)
	if body == nil {
		return nil, errors.New("no body found")
	}

	var out bytes.Buffer
	if err := html.Render(&out, body); err != nil {
		return nil, errors.Wrap(err, "render html")
	}

	return out.Bytes(), nil
}

// FetchDynamicURLContent is a wrapper for submit & fetch dynamic url content
func FetchDynamicURLContent(ctx context.Context, url string) ([]byte, error) {
	// submit task
	taskID, err := rutils.GetCli().AddHTMLCrawlerTask(ctx, url)
	if err != nil {
		return nil, errors.Wrap(err, "submit task")
	}

	// fetch task result
	for {
		task, err := rutils.GetCli().GetHTMLCrawlerTaskResult(ctx, taskID)
		if err != nil {
			return nil, errors.Wrap(err, "get task result")
		}

		switch task.Status {
		case rlibs.TaskStatusSuccess:
			return task.ResultHTML, nil
		case rlibs.KeyTaskHTMLCrawlerPending,
			rlibs.TaskStatusRunning:
			time.Sleep(time.Second)
			continue
		case rlibs.TaskStatusFailed:
			return nil, errors.Errorf("task failed at %s for reason %q",
				*task.FinishedAt, *task.FailedReason)
		default:
			return nil, errors.Errorf("unknown task status %q", task.Status)
		}
	}
}
