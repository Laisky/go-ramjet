package tasks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/laisky-blog-graphql/library/crawleregress"
	rlibs "github.com/Laisky/laisky-blog-graphql/library/db/redis"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/stretchr/testify/require"

	appLog "github.com/Laisky/go-ramjet/library/log"
)

// TestVerifiedWorkerActualAlertAndCuratedJSON exercises the real worker, default SDK hook and local JSON encoder.
func TestVerifiedWorkerActualAlertAndCuratedJSON(t *testing.T) {
	preserveVerifiedWorkerSeams(t)
	messages := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		defer req.Body.Close()
		var payload struct {
			Variables struct {
				Message string `json:"msg"`
			} `json:"variables"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, req.Body, 1<<20)).Decode(&payload); err != nil {
			http.Error(w, "invalid synthetic request", http.StatusBadRequest)
			return
		}
		messages <- payload.Variables.Message
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"data":{"TelegramMonitorAlert":{"name":"accepted"}}}`)
	}))
	t.Cleanup(server.Close)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	alert, err := glog.NewAlert(ctx, server.URL, glog.WithAlertType("synthetic"),
		glog.WithAlertToken("synthetic-only"))
	require.NoError(t, err)
	t.Cleanup(func() {
		alert.Close()
		select {
		case <-alert.Done():
		case <-time.After(3 * time.Second):
			t.Error("alert sender failed to stop")
		}
	})
	var encoded bytes.Buffer
	core := zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
		zapcore.AddSync(&encoded), zap.DebugLevel)
	base, err := glog.NewWithName("go-ramjet", glog.LevelDebug,
		zap.WrapCore(func(zapcore.Core) zapcore.Core { return core }),
		zap.HooksWithFields(appLog.CrawlerDiagnosticAlertHook(alert.GetZapHook())))
	require.NoError(t, err)
	originalLogger := appLog.Logger
	appLog.Logger = base
	t.Cleanup(func() { appLog.Logger = originalLogger })
	private := base.Named("run_dynamic_web_crawler").With(
		zap.String("url", "private-context-url-canary"), zap.String("user_id", "private-context-user-canary"),
		zap.String("api_key", "private-context-key-canary"))
	private.Debug("synthetic inherited-context control")
	require.Contains(t, encoded.String(), "private-context-url-canary")
	encoded.Reset()
	fetchVerifiedHTMLContent = func(context.Context, *rlibs.HTMLCrawlerTask) (string, *crawleregress.Receipt, error) {
		return "", nil, &crawlerDiagnosticCanaryCause{}
	}
	publications := 0
	publishHTMLCrawlerTask = func(publishCtx context.Context, task *rlibs.HTMLCrawlerTask) error {
		publications++
		if task.Status == rlibs.TaskStatusFailed {
			payload, err := encodeHTMLCrawlerTaskResult(publishCtx, task)
			require.NoError(t, err)
			require.Contains(t, payload, "\"failure_stage\":\"render\"")
			require.Contains(t, payload, "\"failure_category\":\"render_failed\"")
		}
		return nil
	}
	task := verifiedTestTask()
	task.TaskID = "37b25fb8-4974-4a11-a5f5-b2f6c296025c"
	require.NoError(t, processHTMLCrawlerTask(gmw.SetLogger(ctx, private), task))
	require.Equal(t, 2, publications)
	require.NotEmpty(t, encoded.String())
	require.Contains(t, encoded.String(), "\"stage\":\"render\"")
	require.Contains(t, encoded.String(), task.TaskID)
	var message string
	select {
	case message = <-messages:
	case <-time.After(3 * time.Second):
		t.Fatal("no actual worker SDK alert")
	}
	require.False(t, strings.HasSuffix(message, "{}"), "approved failure fields must survive actual HTTP serialization")
	require.Contains(t, message, "\"stage\":\"render\"")
	require.Contains(t, message, "\"category\":\"render_failed\"")
	require.Contains(t, message, "\"error_code\":\"render_failed\"")
	require.Contains(t, message, "navigation failed", "the sanitized cause remains useful")
	require.Contains(t, message, task.TaskID)
	for _, canary := range []string{"private-context", "private.invalid", "private-query-canary", "private-key-canary", "private-body-canary", "private-auth-canary"} {
		require.NotContains(t, message, canary)
		require.NotContains(t, encoded.String(), canary)
	}
}
