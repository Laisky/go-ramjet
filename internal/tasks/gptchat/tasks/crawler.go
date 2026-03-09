package tasks

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	md "github.com/JohannesKaufmann/html-to-markdown"
	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	rlibs "github.com/Laisky/laisky-blog-graphql/library/db/redis"
	"github.com/Laisky/zap"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/redis/go-redis/v9"
	"golang.org/x/net/html"
	"golang.org/x/sync/semaphore"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
	"github.com/Laisky/go-ramjet/library/log"
	"github.com/Laisky/go-ramjet/library/openai"
	rutils "github.com/Laisky/go-ramjet/library/redis"
)

func crawlerResultKey(taskID string) string {
	return rlibs.KeyPrefixTaskHTMLCrawlerResult + taskID
}

var (
	defaultChromedpSemaLimit = 2
	// chromedpSema chromedp cost too much memory, so limit it
	chromedpSema *semaphore.Weighted
	// fetchDynamicHTMLContent renders a page and returns the raw HTML for the current crawl attempt.
	fetchDynamicHTMLContent = fetchDynamicHTMLByChromedp
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
					log.Logger.Error("run dynamic web crawler", zap.Error(err))
				}

				time.Sleep(time.Second)
			}
		}()
	}
}

func runDynamicWebCrawler() error {
	ctxCrawler, cancelCrawler := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancelCrawler()

	task, err := popHTMLCrawlerTask(ctxCrawler)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil
		}
		if errors.Is(err, redis.Nil) {
			return nil
		}

		return errors.Wrap(err, "get html crawler task")
	}

	if task == nil {
		return errors.New("html crawler task is nil")
	}

	logger := log.Logger.Named("run_dynamic_web_crawler").With(
		zap.Bool("output_markdown", task.OutputMarkdown),
		zap.String("task_id", task.TaskID),
		zap.String("url", task.Url))
	logger.Info("get task")
	ctxCrawler = gmw.SetLogger(ctxCrawler, logger)

	// mark running
	if err := setHTMLCrawlerTaskResult(ctxCrawler, task); err != nil {
		logger.Warn("set running state", zap.Error(err))
	}

	rawBody, markdown, err := dynamicFetchWorker(ctxCrawler, task.Url, task.APIKey, task.OutputMarkdown)
	if err != nil {
		now := time.Now().UTC()
		reason := fmt.Sprintf("fetch url %q", task.Url)

		task.Status = rlibs.TaskStatusFailed
		task.FinishedAt = &now
		task.FailedReason = &reason
		logger.Error("dynamic fetch url", zap.Error(err))
	} else {
		now := time.Now().UTC()
		task.ResultHTML = rawBody
		if task.OutputMarkdown && strings.TrimSpace(markdown) != "" {
			task.ResultMarkdown = []byte(markdown)
		}
		task.Status = rlibs.TaskStatusSuccess
		task.FinishedAt = &now
		logger.Info("success dynamic fetch url")

		var markdownPtr *string
		if strings.TrimSpace(markdown) != "" {
			markdownPtr = &markdown
		}

		record := &CrawlRecord{
			TaskID:       task.TaskID,
			CrawledAt:    now,
			APIKeyPrefix: apiKeyPrefix(task.APIKey),
			URL:          task.Url,
			RawBody:      rawBody,
			Markdown:     markdownPtr,
		}
		if err := SaveCrawlRecord(ctxCrawler, record); err != nil {
			logger.Warn("persist crawl record", zap.Error(err))
		}
	}

	if err := setHTMLCrawlerTaskResult(ctxCrawler, task); err != nil {
		return errors.Wrap(err, "set task result")
	}

	return nil
}

func popHTMLCrawlerTask(ctx context.Context) (*rlibs.HTMLCrawlerTask, error) {
	client := rutils.GetCli().GetDB().Client

	vals, err := client.BLPop(ctx, 5*time.Second, rlibs.KeyTaskHTMLCrawlerPending).Result()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if len(vals) != 2 {
		return nil, errors.Errorf("invalid blpop response size %d", len(vals))
	}

	task, err := rlibs.NewHTMLCrawlerTaskFromString(vals[1])
	if err != nil {
		return nil, errors.Wrap(err, "parse task")
	}

	task.Status = rlibs.TaskStatusRunning
	return task, nil
}

func setHTMLCrawlerTaskResult(ctx context.Context, task *rlibs.HTMLCrawlerTask) error {
	if task == nil {
		return errors.New("task is nil")
	}

	payload, err := task.ToString()
	if err != nil {
		return errors.Wrap(err, "serialize task")
	}

	ctxPublish, cancelPublish := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelPublish()

	key := crawlerResultKey(task.TaskID)
	if err := rutils.GetCli().GetDB().Client.Set(ctxPublish, key, payload, 7*24*time.Hour).Err(); err != nil {
		return errors.Wrapf(err, "set task result %q", key)
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

// fetchDynamicHTMLByChromedp renders targetURL through chromedp and returns the full HTML document.
func fetchDynamicHTMLByChromedp(ctx context.Context, targetURL string) (htmlContent string, err error) {
	logger := gmw.GetLogger(ctx).Named("fetch_dynamic_html_by_chromedp").With(
		zap.String("url", targetURL),
	)

	headers := map[string]any{
		//nolint: lll
		"User-Agent":      "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/58.0.3029.110 Safari/537",
		"Accept":          "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8",
		"Accept-Language": "en-US,en;q=0.8",
		"Accept-Encoding": "gzip, deflate, sdch",
		"Connection":      "keep-alive",
	}

	if err = chromedpSema.Acquire(ctx, 1); err != nil {
		return "", errors.Wrap(err, "acquire chromedp sema")
	}
	defer chromedpSema.Release(1)

	chromeOpts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.NoDefaultBrowserCheck,
		chromedp.NoFirstRun,
		chromedp.WindowSize(1920, 1080),
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-setuid-sandbox", true),
	)
	if os.Getenv("CRAWLER_HTTP_PROXY") != "" {
		logger.Debug("set proxy", zap.String("proxy", os.Getenv("CRAWLER_HTTP_PROXY")))
		chromeOpts = append(chromeOpts, chromedp.ProxyServer(os.Getenv("CRAWLER_HTTP_PROXY")))
	}

	allocCtx, cancel := chromedp.NewExecAllocator(ctx, chromeOpts...)
	defer cancel()

	chromeCtx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	err = chromedp.Run(chromeCtx, chromedp.Tasks{
		network.Enable(),
		network.SetExtraHTTPHeaders(network.Headers(headers)),
		chromedp.Navigate(targetURL),
		chromedp.WaitReady("body", chromedp.ByQuery),
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
		chromedp.Sleep(2 * time.Second),
		chromedp.InnerHTML("html", &htmlContent, chromedp.ByQuery),
	})
	if err != nil {
		return "", errors.Wrapf(err, "run chromedp for %q", targetURL)
	}

	return htmlContent, nil
}

// fallbackDynamicFetchWithProxy attempts configured web fetch proxies after direct crawling fails.
func fallbackDynamicFetchWithProxy(
	ctx context.Context,
	targetURL string,
	outputMarkdown bool,
	trigger string,
	primaryErr error,
) (rawBody []byte, markdown string, err error) {
	logger := gmw.GetLogger(ctx).Named("fallback_dynamic_fetch_with_proxy").With(
		zap.String("url", targetURL),
		zap.String("trigger", trigger),
		zap.Bool("output_markdown", outputMarkdown),
	)
	logger.Debug("attempt web fetch proxy fallback")

	proxyBody, err := fetchByWebFetchProxyRace(ctx, targetURL)
	if err != nil {
		fields := []zap.Field{zap.Error(err)}
		if primaryErr != nil {
			fields = append(fields, zap.String("primary_error", primaryErr.Error()))
		}
		logger.Warn("web fetch proxy fallback failed", fields...)
		if primaryErr != nil {
			return nil, "", errors.Wrapf(primaryErr, "web fetch proxy fallback after %s", trigger)
		}

		return nil, "", errors.Wrapf(err, "web fetch proxy fallback after %s", trigger)
	}

	proxyBody = strings.TrimSpace(proxyBody)
	if proxyBody == "" {
		err = errors.Errorf("web fetch proxy returned empty body after %s", trigger)
		fields := []zap.Field{zap.Error(err)}
		if primaryErr != nil {
			fields = append(fields, zap.String("primary_error", primaryErr.Error()))
		}
		logger.Warn("web fetch proxy fallback returned empty body", fields...)
		if primaryErr != nil {
			return nil, "", errors.Wrapf(primaryErr, "web fetch proxy fallback after %s", trigger)
		}

		return nil, "", err
	}

	logger.Info("web fetch proxy fallback succeeded", zap.Int("len", len(proxyBody)))
	if outputMarkdown {
		return []byte(proxyBody), proxyBody, nil
	}

	return []byte(proxyBody), "", nil
}

// dynamicFetchWorker fetch dynamic url content, will render js by chromedp
func dynamicFetchWorker(ctx context.Context, url, apiKey string, outputMarkdown bool, opts ...FetchURLOption) (rawBody []byte, markdown string, err error) {
	startAt := time.Now()
	logger := gmw.GetLogger(ctx).Named("fetch_dynamic_url_content").
		With(zap.String("url", url))

	// opt, err := new(fetchURLOption).apply(opts...)
	// if err != nil {
	// 	return nil, errors.Wrap(err, "apply options")
	// }

	logger.Debug("fetch dynamic url", zap.Bool("output_markdown", outputMarkdown))

	htmlContent, err := fetchDynamicHTMLContent(ctx, url)
	if err != nil {
		logger.Warn("direct dynamic fetch failed, try web fetch proxies", zap.Error(err))
		if rawBody, markdown, err = fallbackDynamicFetchWithProxy(ctx, url, outputMarkdown, "direct_fetch_failed", err); err == nil {
			return rawBody, markdown, nil
		}

		return nil, "", err
	}

	if isCloudflareChallenge(htmlContent) {
		challengeErr := errors.Errorf("cloudflare challenge detected for %q", url)
		logger.Warn("cloudflare challenge detected", zap.String("trigger", "cloudflare_challenge"))
		if rawBody, markdown, err = fallbackDynamicFetchWithProxy(ctx, url, outputMarkdown, "cloudflare_challenge", challengeErr); err == nil {
			return rawBody, markdown, nil
		}

		return nil, "", err
	}

	content := []byte(htmlContent)
	if len(content) == 0 {
		emptyErr := errors.Errorf("no content found by chromedp for %q", url)
		logger.Warn("empty content from direct dynamic fetch", zap.String("trigger", "empty_direct_content"))
		if rawBody, markdown, err = fallbackDynamicFetchWithProxy(ctx, url, outputMarkdown, "empty_direct_content", emptyErr); err == nil {
			return rawBody, markdown, nil
		}

		return nil, "", err
	}

	bodyContent, markdownText, err := ExtractHTMLBody(ctx, url, content, apiKey, outputMarkdown)
	if err != nil {
		log.Logger.Warn("extract html body", zap.Error(err))
		return content, "", nil
	}

	rawBody = bodyContent
	markdown = markdownText

	logger.Info("succeed fetch dynamic url",
		zap.Int("len", len(rawBody)),
		zap.Duration("cost_secs", time.Since(startAt)))

	return rawBody, markdown, nil
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
func ExtractHTMLBody(ctx context.Context, targetURL string, content []byte, apiKey string, outputMarkdown bool) (bodyContent []byte, markdown string, err error) {
	parsedHTML, err := html.Parse(bytes.NewReader(content))
	if err != nil {
		return nil, "", errors.Wrap(err, "parse html")
	}

	body := findHTMLBody(parsedHTML)
	if body == nil {
		return nil, "", errors.New("no body found")
	}

	var out bytes.Buffer
	if err := html.Render(&out, body); err != nil {
		return nil, "", errors.Wrap(err, "render html")
	}

	var inner bytes.Buffer
	for child := body.FirstChild; child != nil; child = child.NextSibling {
		if err := html.Render(&inner, child); err != nil {
			return nil, "", errors.Wrap(err, "render body")
		}
	}

	bodyContent = out.Bytes()
	if !outputMarkdown {
		return bodyContent, "", nil
	}

	logger := gmw.GetLogger(ctx).Named("extract_html_body")

	// 1) local conversion first
	converter := md.NewConverter("", true, nil)
	innerHTML := inner.Bytes()
	localInput := innerHTML
	if len(localInput) == 0 {
		localInput = bodyContent
	}
	localMarkdown, localErr := converter.ConvertString(string(localInput))
	if localErr == nil {
		localMarkdown = strings.TrimSpace(localMarkdown)
		if localMarkdown != "" {
			return bodyContent, localMarkdown, nil
		}
	}
	if localErr != nil {
		logger.Debug("local html-to-markdown failed", zap.Error(localErr))
	}

	// 2) fallback to web fetch proxies
	if targetURL != "" {
		proxyMarkdown, proxyErr := fetchByWebFetchProxyRace(ctx, targetURL)
		if proxyErr == nil {
			proxyMarkdown = strings.TrimSpace(proxyMarkdown)
			if proxyMarkdown != "" {
				return bodyContent, proxyMarkdown, nil
			}
		} else {
			logger.Debug("web fetch proxies failed", zap.Error(proxyErr))
		}
	}

	// 3) fallback to LLM conversion
	if strings.TrimSpace(apiKey) == "" {
		return bodyContent, "", nil
	}

	llmInput := innerHTML
	if len(llmInput) == 0 {
		llmInput = bodyContent
	}
	llmMarkdown, llmErr := openai.HTMLBodyToMarkdown(ctx, config.Config.API, apiKey, llmInput)
	if llmErr != nil {
		logger.Warn("convert html to markdown", zap.Error(llmErr))
		return bodyContent, "", nil
	}
	llmMarkdown = strings.TrimSpace(llmMarkdown)
	if llmMarkdown == "" {
		return bodyContent, "", nil
	}

	return bodyContent, llmMarkdown, nil
}

// isCloudflareChallenge checks if the content is a Cloudflare challenge page
func isCloudflareChallenge(htmlContent string) bool {
	// Check for Cloudflare Turnstile/Challenge indicators
	indicators := []string{
		"cf-turnstile-response",
		"cf-chl-widget",
		"_cf_chl_opt",
		"challenge-platform",
		"challenge-error-text",
		"Just a moment...",
		"Verification is taking longer than expected",
	}
	for _, indicator := range indicators {
		if strings.Contains(htmlContent, indicator) {
			return true
		}
	}
	return false
}

// FetchDynamicURLContent is a wrapper for submit & fetch dynamic url content
func FetchDynamicURLContent(ctx context.Context, url string, opts ...FetchDynamicURLContentOption) ([]byte, error) {
	opt, err := new(fetchDynamicURLContentOption).apply(opts...)
	if err != nil {
		return nil, errors.Wrap(err, "apply options")
	}

	// DB cache lookup
	if record, ok, err := LoadLatestCrawlRecord(ctx, url); err != nil {
		return nil, errors.Wrap(err, "load cache")
	} else if ok {
		age := time.Since(record.CrawledAt)
		fresh := age >= 0 && age <= 3*24*time.Hour
		if fresh {
			if opt.outputMarkdown && record.Markdown != nil && strings.TrimSpace(*record.Markdown) != "" {
				return []byte(*record.Markdown), nil
			}
			return record.RawBody, nil
		}
	}

	// submit task
	taskID, err := addHTMLCrawlerTask(ctx, url, opt.apiKey, opt.outputMarkdown)
	if err != nil {
		return nil, errors.Wrap(err, "submit task")
	}

	// fetch task result
	for {
		task, err := getHTMLCrawlerTaskResult(ctx, taskID)
		if err != nil {
			return nil, errors.Wrap(err, "get task result")
		}

		switch task.Status {
		case rlibs.TaskStatusSuccess:
			return task.ResultHTML, nil
		case rlibs.TaskStatusPending,
			rlibs.TaskStatusRunning:
			time.Sleep(time.Second)
			continue
		case rlibs.TaskStatusFailed:
			if task.FinishedAt == nil || task.FailedReason == nil {
				return nil, errors.Errorf("task %q failed", taskID)
			}
			return nil, errors.Errorf("task failed at %s for reason %q", *task.FinishedAt, *task.FailedReason)
		default:
			return nil, errors.Errorf("unknown task status %q", task.Status)
		}
	}
}

type fetchDynamicURLContentOption struct {
	apiKey         string
	outputMarkdown bool
}

func (o *fetchDynamicURLContentOption) apply(opts ...FetchDynamicURLContentOption) (*fetchDynamicURLContentOption, error) {
	o.outputMarkdown = true

	for _, opt := range opts {
		if err := opt(o); err != nil {
			return nil, err
		}
	}
	return o, nil
}

// FetchDynamicURLContentOption customizes how FetchDynamicURLContent runs.
type FetchDynamicURLContentOption func(*fetchDynamicURLContentOption) error

// WithMarkdownConversion enables HTML body to Markdown conversion.
//
// When apiKey is empty, conversion will be skipped.
func WithMarkdownConversion(apiKey string, outputMarkdown bool) FetchDynamicURLContentOption {
	return func(opt *fetchDynamicURLContentOption) error {
		opt.apiKey = apiKey
		opt.outputMarkdown = outputMarkdown
		return nil
	}
}

func addHTMLCrawlerTask(ctx context.Context, url, apiKey string, outputMarkdown bool) (string, error) {
	task := rlibs.NewHTMLCrawlerTaskWithOptions(url, apiKey, outputMarkdown)
	payload, err := task.ToString()
	if err != nil {
		return "", errors.Wrap(err, "serialize task")
	}

	client := rutils.GetCli().GetDB().Client
	if err := client.Set(ctx, crawlerResultKey(task.TaskID), payload, 7*24*time.Hour).Err(); err != nil {
		return "", errors.Wrap(err, "init task result")
	}
	if err := client.RPush(ctx, rlibs.KeyTaskHTMLCrawlerPending, payload).Err(); err != nil {
		return "", errors.Wrap(err, "enqueue task")
	}

	return task.TaskID, nil
}

func getHTMLCrawlerTaskResult(ctx context.Context, taskID string) (*rlibs.HTMLCrawlerTask, error) {
	client := rutils.GetCli().GetDB().Client
	payload, err := client.Get(ctx, crawlerResultKey(taskID)).Result()
	if err != nil {
		return nil, errors.WithStack(err)
	}

	task, err := rlibs.NewHTMLCrawlerTaskFromString(payload)
	if err != nil {
		return nil, errors.Wrap(err, "parse task result")
	}

	return task, nil
}
