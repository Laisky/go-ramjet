package cmd

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	gconfig "github.com/Laisky/go-config/v2"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/stretchr/testify/require"

	appLog "github.com/Laisky/go-ramjet/library/log"
)

// startupAlertAdoptionFixture invokes actual startup against an owned synthetic HTTP receiver.
func startupAlertAdoptionFixture(t *testing.T) (*bytes.Buffer, *bytes.Buffer, <-chan string, *atomic.Int32) {
	t.Helper()
	messages := make(chan string, 4)
	hits := new(atomic.Int32)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var body struct{ Variables map[string]any }
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<20)).Decode(&body); err != nil {
			http.Error(w, "invalid synthetic request", 400)
			return
		}
		hits.Add(1)
		message, ok := body.Variables["msg"].(string)
		require.True(t, ok)
		messages <- message
		w.Header().Set("Content-Type", "application/json")
		_, err := fmt.Fprint(w, "{\"data\":{\"TelegramMonitorAlert\":{\"name\":\"synthetic\"}}}")
		require.NoError(t, err)
	}))
	t.Cleanup(server.Close)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	for key, value := range map[string]string{"logger.push_api": server.URL, "logger.alert_type": "synthetic", "logger.push_token": "synthetic-transport-token", "openai.rate_limiter_backend": "legacy"} {
		prior := gconfig.Shared.GetString(key)
		gconfig.Shared.Set(key, value)
		t.Cleanup(func() { gconfig.Shared.Set(key, prior) })
	}
	previousDebug := gconfig.Shared.GetBool("debug")
	gconfig.Shared.Set("debug", false)
	t.Cleanup(func() { gconfig.Shared.Set("debug", previousDebug) })
	local, failures := new(bytes.Buffer), new(bytes.Buffer)
	core := zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()), zapcore.AddSync(local), zapcore.DebugLevel)
	logger, err := glog.NewWithName("go-ramjet", glog.LevelDebug, zap.WrapCore(func(zapcore.Core) zapcore.Core { return core }), zap.ErrorOutput(zapcore.AddSync(failures)))
	require.NoError(t, err)
	previousLogger := appLog.Logger
	appLog.Logger = logger
	t.Cleanup(func() { appLog.Logger = previousLogger })
	setupLogger(ctx)
	return local, failures, messages, hits
}

// startupAlertJSON extracts the existing metadata-framed JSON without a new wire codec.
func startupAlertJSON(t *testing.T, message string) map[string]any {
	t.Helper()
	offset := strings.Index(message, "\n{")
	require.GreaterOrEqual(t, offset, 0)
	var record map[string]any
	require.NoError(t, json.Unmarshal([]byte(message[offset+1:]), &record))
	return record
}

// TestStartupAlertCorePreservesChosenContextAndLargeInputs proves the real startup
// admits a >16KiB record, preserves host/eager context/namespace ordering, and sends once.
func TestStartupAlertCorePreservesChosenContextAndLargeInputs(t *testing.T) {
	_, failures, messages, hits := startupAlertAdoptionFixture(t)
	original := []byte{0xff, 0, 0xfe, 'a'}
	expected := bytes.Clone(original)
	target := "https://example.invalid/original?api_key=SYNTHETIC_CHOSEN_QUERY#fragment"
	html := "<body>" + strings.Repeat("original chosen HTML; ", 1500) + "</body>"
	logger := appLog.Logger.Named("run_dynamic_web_crawler.verified_crawler_task").
		With(zap.Binary("bound_binary", original), zap.String("request_id", "synthetic-request")).
		With(zap.Namespace("request"), zap.String("url", target))
	original[0] = 'X'
	logger.Error("synthetic original failure", zap.String("html", html), zap.Binary("raw_bytes", []byte{0, 0xff, 0xc0, 0xaf}))
	var message string
	select {
	case message = <-messages:
	case <-time.After(3 * time.Second):
		t.Fatal("actual startup alert missing")
	}
	require.Greater(t, len(message), 16*1024, "application budget must override the SDK default")
	require.LessOrEqual(t, len(message), appLog.HTTPAlertMaxMessageBytes)
	record := startupAlertJSON(t, message)
	hostname, err := os.Hostname()
	require.NoError(t, err)
	require.Equal(t, hostname, record["host"])
	require.Equal(t, "synthetic-request", record["request_id"])
	bound, err := base64.StdEncoding.DecodeString(record["bound_binary"].(string))
	require.NoError(t, err)
	require.Equal(t, expected, bound, "With snapshots original mutable bytes eagerly")
	nested := record["request"].(map[string]any)
	require.Equal(t, target, nested["url"])
	require.Equal(t, html, nested["html"])
	raw, err := base64.StdEncoding.DecodeString(nested["raw_bytes"].(string))
	require.NoError(t, err)
	require.Equal(t, []byte{0, 0xff, 0xc0, 0xaf}, raw)
	require.Equal(t, 1, strings.Count(message, "\"host\":"), "no redundant hook/context duplication")
	require.Empty(t, failures.String())
	require.NotContains(t, message, "synthetic-transport-token")
	require.EqualValues(t, 1, hits.Load())
	select {
	case <-messages:
		t.Fatal("one entry produced duplicate alerts")
	case <-time.After(50 * time.Millisecond):
	}
}

// TestStartupAlertCoreRejectsWholeOversizeRecord keeps full local content while
// rejecting the complete >1MiB remote entry without clipping or sending a fragment.
func TestStartupAlertCoreRejectsWholeOversizeRecord(t *testing.T) {
	local, failures, messages, hits := startupAlertAdoptionFixture(t)
	html := strings.Repeat("x", appLog.HTTPAlertMaxMessageBytes)
	appLog.Logger.Named("run_dynamic_web_crawler.verified_crawler_task").Error("synthetic oversize failure", zap.String("html", html))
	require.Contains(t, failures.String(), glog.ErrAlertMessageTooLarge.Error())
	require.Contains(t, local.String(), html, "ordinary local logging remains complete")
	select {
	case <-messages:
		t.Fatal("oversize entry must not send a partial alert")
	case <-time.After(50 * time.Millisecond):
	}
	require.Zero(t, hits.Load())
}
