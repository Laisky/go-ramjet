package log

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/stretchr/testify/require"
)

// TestArchiveFailureAlertHTTP retains safe source diagnostics through the actual SDK transport.
func TestArchiveFailureAlertHTTP(t *testing.T) {
	alert, messages := captureCrawlerAlert(t)
	logger := zap.New(CrawlerDiagnosticAlertCore(alert.GetZapCore())).Named("go-ramjet.arweave.localstorage").With(zap.String("bound", "private-bound"))
	logger.Error("failed to fetch and save url content", zap.String("stage", "fetch"),
		zap.String("url", "https://user:private-password@example.com/path?token=private-query"),
		zap.Error(fmt.Errorf("fetch private-error: %w", context.DeadlineExceeded)), zap.Duration("duration", time.Second),
		zap.String("unreviewed", "private-field"))
	message := receiveCrawlerAlert(t, messages)
	require.Contains(t, message, `"stage":"fetch"`)
	require.Contains(t, message, `"category":"timeout"`)
	require.Contains(t, message, `"duration_ms":1000`)
	require.Contains(t, message, `"target_sha256":`)
	for _, secret := range []string{"private-password", "private-query", "private-error", "private-field", "private-bound", "example.com"} {
		require.NotContains(t, message, secret)
	}
	select {
	case duplicate := <-messages:
		t.Fatalf("duplicate alert: %s", duplicate)
	case <-time.After(100 * time.Millisecond):
	}
}

// TestArchiveAlertClassifications preserves distinct finite categories without exposing raw errors.
func TestArchiveAlertClassifications(t *testing.T) {
	for _, tc := range []struct {
		stage, category string
		err             error
	}{
		{"fetch", "fetch_failed", fmt.Errorf("private fetch cause")},
		{"save", "storage_failed", fmt.Errorf("private storage cause")},
		{"fetch", "canceled", fmt.Errorf("wrapped: %w", context.Canceled)},
	} {
		t.Run(tc.category, func(t *testing.T) {
			fields := archiveAlertFields([]zapcore.Field{zap.String("stage", tc.stage), zap.Error(tc.err)})
			require.Equal(t, tc.stage, fields[0].String)
			require.Equal(t, tc.category, fields[1].String)
		})
	}
}

// TestArchiveAlertLeavesUnrelatedRootMetadataOnly verifies neither bound nor call-site private fields leak.
func TestArchiveAlertLeavesUnrelatedRootMetadataOnly(t *testing.T) {
	alert, messages := captureCrawlerAlert(t)
	logger := zap.New(CrawlerDiagnosticAlertCore(alert.GetZapCore())).Named("go-ramjet").With(zap.String("bound", "private-bound"))
	logger.Error("unrelated failure", zap.String("url", "private-target"), zap.Error(fmt.Errorf("private-error")))
	message := receiveCrawlerAlert(t, messages)
	require.Contains(t, message, "{}")
	require.NotContains(t, message, "private-")
}

// TestArchiveAlertHookMatchesCoreProjection preserves the legacy hook adapter's reviewed boundary.
func TestArchiveAlertHookMatchesCoreProjection(t *testing.T) {
	var captured []zapcore.Field
	hook := CrawlerDiagnosticAlertHook(func(_ zapcore.Entry, fields []zapcore.Field) error { captured = fields; return nil })
	require.NoError(t, hook(zapcore.Entry{LoggerName: reviewedArchiveLoggerName}, []zapcore.Field{zap.String("stage", "save"), zap.Error(fmt.Errorf("private-save-error"))}))
	require.Equal(t, []zapcore.Field{zap.String("stage", "save"), zap.String("category", "storage_failed")}, captured)
	require.NoError(t, hook(zapcore.Entry{LoggerName: "go-ramjet"}, []zapcore.Field{zap.String("private", "private-value")}))
	require.Empty(t, captured)
}
