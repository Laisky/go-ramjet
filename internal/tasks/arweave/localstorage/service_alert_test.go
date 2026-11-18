package localstorage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/Laisky/zap/zaptest/observer"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/library/egress"
	"github.com/Laisky/go-ramjet/library/log"
)

// installArchiveTestLogger replaces log.Logger with a root logger that tees a
// local observer and the production alert core exporting to a local receiver.
// It returns the local entries and the alert messages the receiver accepted.
func installArchiveTestLogger(t *testing.T) (*observer.ObservedLogs, <-chan string) {
	t.Helper()
	messages := make(chan string, 4)
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var payload struct {
			Variables struct {
				Message string `json:"msg"`
			} `json:"variables"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
			return
		}
		messages <- payload.Variables.Message
		if _, err := fmt.Fprint(w, `{"data":{"TelegramMonitorAlert":{"name":"accepted"}}}`); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(receiver.Close)
	ctx, cancel := context.WithCancel(context.Background())
	alert, err := glog.NewAlert(ctx, receiver.URL, glog.WithAlertType("synthetic"), glog.WithAlertToken("synthetic"),
		glog.WithAlertHookLevel(zap.ErrorLevel))
	require.NoError(t, err)
	t.Cleanup(func() {
		alert.Close()
		cancel()
		select {
		case <-alert.Done():
		case <-time.After(3 * time.Second):
			t.Error("alert did not stop")
		}
	})
	local, entries := observer.New(zapcore.DebugLevel)
	logger, err := glog.New(glog.WithName("go-ramjet"), glog.WithZapOptions(zap.WrapCore(func(zapcore.Core) zapcore.Core {
		return zapcore.NewTee(local, log.CrawlerDiagnosticAlertCore(alert.GetZapCore()))
	})))
	require.NoError(t, err)
	old := log.Logger
	log.Logger = logger
	t.Cleanup(func() { log.Logger = old })
	return entries, messages
}

// TestSaveURLContentTaskExpectedRefusalStaysLocal exercises the real archive
// fetch and crawler admission path. A loopback target is refused before any
// request, recorded locally at WARN with its correlation reference, and never
// consumes the shared ERROR alert budget.
func TestSaveURLContentTaskExpectedRefusalStaysLocal(t *testing.T) {
	entries, messages := installArchiveTestLogger(t)
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("private target must not be fetched")
	}))
	t.Cleanup(target.Close)
	raw := target.URL + "/private-path?token=private-query"

	saveURLContentTask(t.Context(), taskItem{url: raw})

	select {
	case message := <-messages:
		t.Fatalf("expected destination refusal must not alert: %s", message)
	case <-time.After(300 * time.Millisecond):
	}
	logged := entries.FilterLoggerName(log.ArchiveLoggerName).All()
	require.Len(t, logged, 1)
	require.Equal(t, zapcore.WarnLevel, logged[0].Level)
	fields := logged[0].ContextMap()
	require.Equal(t, raw, fields["url"], "local logs keep the raw target for operators")
	require.Equal(t, log.ArchiveTargetRef(raw), fields["target_ref"])
	require.Equal(t, "fetch", fields["stage"])
}

// TestSaveURLContentTaskOperationalFailureAlerts keeps resolver failures at
// ERROR and exports a safe, attributed alert whose reference matches the local log.
func TestSaveURLContentTaskOperationalFailureAlerts(t *testing.T) {
	entries, messages := installArchiveTestLogger(t)
	old := fetchArchiveContent
	t.Cleanup(func() { fetchArchiveContent = old })
	fetchArchiveContent = func(ctx context.Context, target string) ([]byte, error) {
		_, err := egress.Admit(ctx, target, func(context.Context, string) ([]net.IPAddr, error) {
			return nil, &net.DNSError{Err: "i/o timeout", Name: "private-host.example", IsTimeout: true}
		})
		return nil, err
	}
	raw := "https://private-host.example/private-path?token=private-query"

	saveURLContentTask(t.Context(), taskItem{url: raw})

	var message string
	select {
	case message = <-messages:
	case <-time.After(3 * time.Second):
		t.Fatal("operational admission failure must alert")
	}
	require.Contains(t, message, `"stage":"admission"`)
	require.Contains(t, message, `"category":"dns_timeout"`)
	require.Contains(t, message, `"target_ref":"`+log.ArchiveTargetRef(raw)+`"`)
	digest := sha256.Sum256([]byte(raw))
	for _, secret := range []string{"private-host", "private-path", "private-query", "i/o timeout",
		hex.EncodeToString(digest[:]), url2objkey(raw)} {
		require.NotContains(t, message, secret)
	}
	logged := entries.FilterLoggerName(log.ArchiveLoggerName).All()
	require.Len(t, logged, 1)
	require.Equal(t, zapcore.ErrorLevel, logged[0].Level)
	require.Equal(t, log.ArchiveTargetRef(raw), logged[0].ContextMap()["target_ref"])
}
