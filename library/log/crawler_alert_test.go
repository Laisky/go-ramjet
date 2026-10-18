package log

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/stretchr/testify/require"
)

// captureCrawlerAlert uses a local HTTP receiver for the actual pinned SDK's outbound alert message.
func captureCrawlerAlert(t *testing.T, options ...glog.AlertOption) (*glog.Alert, <-chan string) {
	t.Helper()
	messages := make(chan string, 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var request struct {
			Variables struct {
				Message string `json:"msg"`
			} `json:"variables"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&request); err != nil {
			http.Error(w, "invalid synthetic request", http.StatusBadRequest)
			return
		}
		messages <- request.Variables.Message
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"data":{"TelegramMonitorAlert":{"name":"accepted"}}}`)
	}))
	t.Cleanup(server.Close)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	opts := append([]glog.AlertOption{glog.WithAlertType("synthetic"), glog.WithAlertToken("synthetic-test-token")}, options...)
	alert, err := glog.NewAlert(ctx, server.URL, opts...)
	require.NoError(t, err)
	t.Cleanup(func() {
		alert.Close()
		select {
		case <-alert.Done():
		case <-time.After(3 * time.Second):
			t.Error("SDK sender failed to stop")
		}
	})
	return alert, messages
}

// receiveCrawlerAlert bounds reception and returns the actual alert text received over HTTP.
func receiveCrawlerAlert(t *testing.T, messages <-chan string) string {
	t.Helper()
	select {
	case message := <-messages:
		return message
	case <-time.After(3 * time.Second):
		t.Fatal("no synthetic alert")
		return ""
	}
}

// TestDefaultCrawlerAlertHTTPRetainsFutureFields verifies faithful default forwarding over real HTTP.
func TestDefaultCrawlerAlertHTTPRetainsFutureFields(t *testing.T) {
	alert, messages := captureCrawlerAlert(t)
	fields := CrawlerFailureFields("37b25fb8-4974-4a11-a5f5-b2f6c296025c", "conversion", "conversion_failed", time.Second)
	fields = append(fields, zap.String("future_diagnostic", "converter_unavailable"),
		zap.Bool("future_retryable", true), zap.String("api_key", "private-key-canary"),
		zap.Error(fmt.Errorf("conversion failed: api_key=private-key-canary Authorization: Bearer private-auth-canary body=private-body-canary")))
	require.NoError(t, alert.GetZapHook()(zapcore.Entry{Level: zap.ErrorLevel,
		LoggerName: "go-ramjet.run_dynamic_web_crawler.verified_crawler_task", Message: "verified crawler task failed"}, fields))
	message := receiveCrawlerAlert(t, messages)
	require.Contains(t, message, "\"stage\":\"conversion\"")
	require.Contains(t, message, "\"category\":\"conversion_failed\"")
	require.Contains(t, message, "converter_unavailable", "new useful metadata forwards without adding field names")
	require.Contains(t, message, "\"future_retryable\":true")
	require.Contains(t, message, "conversion failed", "chosen error text remains unchanged")
	for _, secret := range []string{"private-key-canary", "private-auth-canary", "private-body-canary"} {
		require.Contains(t, message, secret, "synthetic credential-shaped values are application-chosen test inputs")
	}
}

// TestDefaultAlertUnrelatedSafeMetadataStillForwards protects generic default fields beyond the worker callsite.
func TestDefaultAlertUnrelatedSafeMetadataStillForwards(t *testing.T) {
	alert, messages := captureCrawlerAlert(t)
	require.NoError(t, alert.GetZapHook()(zapcore.Entry{Level: zap.ErrorLevel,
		LoggerName: "go-ramjet.other_operation", Message: "other safe operation failed"},
		[]zapcore.Field{zap.String("future_component", "queue"), zap.Int("attempt", 2)}))
	message := receiveCrawlerAlert(t, messages)
	require.Contains(t, message, "\"future_component\":\"queue\"")
	require.Contains(t, message, "\"attempt\":2")
}

// TestCrawlerFailureFieldBounds validates canonical correlation and the numeric duration cap.
func TestCrawlerFailureFieldBounds(t *testing.T) {
	for _, duration := range []time.Duration{-time.Second, 24 * time.Hour} {
		fields := CrawlerFailureFields("synthetic-private-id", "render", "render_failed", duration)
		for _, field := range fields {
			require.NotEqual(t, "task_id", field.Key)
			require.NotEqual(t, "duration_ms", field.Key)
		}
	}
	require.Empty(t, CrawlerFailureFields("37b25fb8-4974-4a11-a5f5-b2f6c296025c", "render", "synthetic-private", time.Second))
}
