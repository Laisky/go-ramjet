package tasks

import (
	"context"
	stderrors "errors"
	"os"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	gconfig "github.com/Laisky/go-config/v2"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/library/log"
)

func setupHTMLCrawler(t *testing.T) {
	// os.Setenv("CRAWLER_HTTP_PROXY", "http://100.97.189.32:17777")

	gconfig.S.Set("redis.addr", "100.122.41.16:6379")
	gconfig.S.Set("redis.db", 0)
}

func Test_dynamicFetchWorker(t *testing.T) {
	// if os.Getenv("RUN_GPT_HTTP_IT") == "" {
	// 	t.Skip("integration test disabled: set RUN_GPT_HTTP_IT to run")
	// }
	setupHTMLCrawler(t)

	ctx := context.Background()
	url := "https://platform.openai.com/docs/models"

	log.Logger.ChangeLevel(glog.LevelDebug)

	logger := log.Logger.Named("Test_dynamicFetchWorker")
	ctx = gmw.SetLogger(ctx, logger)

	content, _, err := dynamicFetchWorker(ctx, url, "xxx", true)
	if err != nil {
		require.Contains(t, err.Error(), "cloudflare challenge detected")
		return
	}
	require.NotNil(t, content)

	t.Log(string(content))
}

func Test_fetchWorker(t *testing.T) {
	if os.Getenv("RUN_GPT_HTTP_IT") == "" {
		t.Skip("integration test disabled: set RUN_GPT_HTTP_IT to run")
	}
	setupHTMLCrawler(t)

	err := runDynamicWebCrawler()
	require.NoError(t, err)
}

func Test_dynamicFetchWorkerFallsBackToProxyOnDirectFetchError(t *testing.T) {
	originalFetcher := fetchDynamicHTMLContent
	originalProxies := webFetchProxies
	fetchDynamicHTMLContent = func(context.Context, string) (string, error) {
		return "", stderrors.New("chromedp boom")
	}
	webFetchProxies = []webFetchProxy{
		stubWebFetchProxy{name: "jina", body: "proxy-body"},
	}
	t.Cleanup(func() {
		fetchDynamicHTMLContent = originalFetcher
		webFetchProxies = originalProxies
	})

	ctx := gmw.SetLogger(context.Background(), log.Logger.Named("Test_dynamicFetchWorkerFallsBackToProxyOnDirectFetchError"))

	content, markdown, err := dynamicFetchWorker(ctx, "https://example.com", "", false)
	require.NoError(t, err)
	require.Equal(t, []byte("proxy-body"), content)
	require.Empty(t, markdown)
}

func Test_dynamicFetchWorkerFallsBackToProxyOnCloudflareChallenge(t *testing.T) {
	originalFetcher := fetchDynamicHTMLContent
	originalProxies := webFetchProxies
	fetchDynamicHTMLContent = func(context.Context, string) (string, error) {
		return `<html><body><div id="cf-chl-widget"></div></body></html>`, nil
	}
	webFetchProxies = []webFetchProxy{
		stubWebFetchProxy{name: "jina", body: "proxy-markdown"},
	}
	t.Cleanup(func() {
		fetchDynamicHTMLContent = originalFetcher
		webFetchProxies = originalProxies
	})

	ctx := gmw.SetLogger(context.Background(), log.Logger.Named("Test_dynamicFetchWorkerFallsBackToProxyOnCloudflareChallenge"))

	content, markdown, err := dynamicFetchWorker(ctx, "https://example.com", "", true)
	require.NoError(t, err)
	require.Equal(t, []byte("proxy-markdown"), content)
	require.Equal(t, "proxy-markdown", markdown)
}

func Test_dynamicFetchWorkerReturnsWrappedErrorWhenFallbackFails(t *testing.T) {
	originalFetcher := fetchDynamicHTMLContent
	originalProxies := webFetchProxies
	fetchDynamicHTMLContent = func(context.Context, string) (string, error) {
		return "", stderrors.New("chromedp boom")
	}
	webFetchProxies = []webFetchProxy{
		stubWebFetchProxy{name: "jina", err: stderrors.New("jina fail")},
	}
	t.Cleanup(func() {
		fetchDynamicHTMLContent = originalFetcher
		webFetchProxies = originalProxies
	})

	ctx := gmw.SetLogger(context.Background(), log.Logger.Named("Test_dynamicFetchWorkerReturnsWrappedErrorWhenFallbackFails"))

	content, markdown, err := dynamicFetchWorker(ctx, "https://example.com", "", false)
	require.Error(t, err)
	require.Nil(t, content)
	require.Empty(t, markdown)
	require.Contains(t, err.Error(), "chromedp boom")
	require.Contains(t, err.Error(), "web fetch proxy fallback after direct_fetch_failed")
}

func TestExtractHTMLBodyConvertsToMarkdownWithoutAPIKey(t *testing.T) {
	body, markdown, err := ExtractHTMLBody(
		gmw.SetLogger(context.Background(), log.Logger.Named("TestExtractHTMLBodyConvertsToMarkdownWithoutAPIKey")),
		"",
		[]byte(`<html><body><h1>Title</h1><p>Hello world</p></body></html>`),
		"",
		true,
	)
	require.NoError(t, err)
	require.Contains(t, string(body), "<body>")
	require.Contains(t, markdown, "Title")
	require.Contains(t, markdown, "Hello world")
}

func Test_fetchDynamicURLContentOptionDefaultsToMarkdown(t *testing.T) {
	opt, err := new(fetchDynamicURLContentOption).apply()
	require.NoError(t, err)
	require.True(t, opt.outputMarkdown)

	opt, err = new(fetchDynamicURLContentOption).apply(WithMarkdownConversion("", false))
	require.NoError(t, err)
	require.False(t, opt.outputMarkdown)
}
