package tasks

import (
	"context"
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
