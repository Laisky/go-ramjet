package tasks

import (
	"context"
	"os"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v6"
	"github.com/Laisky/go-ramjet/library/log"
	glog "github.com/Laisky/go-utils/v5/log"
	"github.com/stretchr/testify/require"
)

func Test_dynamicFetchWorker(t *testing.T) {
	ctx := context.Background()
	url := "https://blog.laisky.com/pages/0/"

	os.Setenv("CRAWLER_HTTP_PROXY", "http://100.97.189.32:17777")

	log.Logger.ChangeLevel(glog.LevelDebug)

	logger := log.Logger.Named("Test_dynamicFetchWorker")
	ctx = gmw.SetLogger(ctx, logger)

	content, err := dynamicFetchWorker(ctx, url)
	require.NoError(t, err)
	require.NotNil(t, content)

	t.Log(string(content))
	t.Error()
}
