package tasks

import (
	"context"
	"os"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v6"
	gconfig "github.com/Laisky/go-config/v2"
	"github.com/Laisky/go-ramjet/library/log"
	glog "github.com/Laisky/go-utils/v5/log"
	"github.com/stretchr/testify/require"
)

func setupHTMLCrawler(t *testing.T) {
	os.Setenv("CRAWLER_HTTP_PROXY", "http://100.97.189.32:17777")

	gconfig.S.Set("redis.addr", "100.122.41.16:6379")
	gconfig.S.Set("redis.db", 0)
}

func Test_dynamicFetchWorker(t *testing.T) {
	setupHTMLCrawler(t)

	ctx := context.Background()
	url := "https://blog.laisky.com/pages/0/"

	log.Logger.ChangeLevel(glog.LevelDebug)

	logger := log.Logger.Named("Test_dynamicFetchWorker")
	ctx = gmw.SetLogger(ctx, logger)

	content, err := dynamicFetchWorker(ctx, url)
	require.NoError(t, err)
	require.NotNil(t, content)

	t.Log(string(content))
	t.Error()
}

func Test_fetchWorker(t *testing.T) {
	setupHTMLCrawler(t)

	err := runDynamicWebCrawler()
	require.NoError(t, err)
}
