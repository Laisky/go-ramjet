package localstorage

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

	"github.com/Laisky/go-ramjet/library/log"
)

// TestSaveURLContentTaskAdmissionAlert exercises the real archive fetch/admission/logger/HTTP path.
// A literal loopback target must be refused before any request reaches its server.
func TestSaveURLContentTaskAdmissionAlert(t *testing.T) {
	messages := make(chan string, 1)
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var payload struct {
			Variables struct {
				Message string `json:"msg"`
			} `json:"variables"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		messages <- payload.Variables.Message
		_, err := fmt.Fprint(w, `{"data":{"TelegramMonitorAlert":{"name":"accepted"}}}`)
		require.NoError(t, err)
	}))
	defer receiver.Close()
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("private target must not be fetched") }))
	defer target.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	alert, err := glog.NewAlert(ctx, receiver.URL, glog.WithAlertType("synthetic"), glog.WithAlertToken("synthetic"))
	require.NoError(t, err)
	defer func() {
		alert.Close()
		select {
		case <-alert.Done():
		case <-time.After(3 * time.Second):
			t.Error("alert did not stop")
		}
	}()
	logger, err := glog.New(glog.WithName("go-ramjet"), glog.WithZapOptions(zap.WrapCore(func(zapcore.Core) zapcore.Core { return log.CrawlerDiagnosticAlertCore(alert.GetZapCore()) })))
	require.NoError(t, err)
	old := log.Logger
	log.Logger = logger
	defer func() { log.Logger = old }()
	saveURLContentTask(ctx, taskItem{url: target.URL + "/private-path?token=private-query"})
	select {
	case message := <-messages:
		require.Contains(t, message, `"stage":"admission"`)
		require.Contains(t, message, `"category":"admission_failed"`)
		require.Contains(t, message, `"target_sha256":`)
		require.NotContains(t, message, "private-query")
		require.NotContains(t, message, "private-path")
		require.NotContains(t, message, target.URL)
	case <-time.After(3 * time.Second):
		t.Fatal("no archive alert")
	}
}
