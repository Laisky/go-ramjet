package log

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/Laisky/zap/zaptest/observer"
	"github.com/stretchr/testify/require"
)

// TestArchiveFailureAlertHTTP retains safe source diagnostics through the actual SDK transport.
// The correlation reference is keyed, so it is neither the URL digest nor the S3 object key.
func TestArchiveFailureAlertHTTP(t *testing.T) {
	alert, messages := captureCrawlerAlert(t)
	target := "https://user:private-password@example.com/path?token=private-query"
	logger := zap.New(CrawlerDiagnosticAlertCore(alert.GetZapCore())).Named(ArchiveLoggerName).With(zap.String("bound", "private-bound"))
	logger.Error("failed to fetch and save url content", zap.String("stage", "fetch"),
		zap.String("url", target), zap.String("target_ref", "private-caller-ref"),
		zap.Error(fmt.Errorf("fetch private-error: %w", context.DeadlineExceeded)), zap.Duration("duration", time.Second),
		zap.String("unreviewed", "private-field"))
	message := receiveCrawlerAlert(t, messages)
	require.Contains(t, message, `"stage":"fetch"`)
	require.Contains(t, message, `"category":"timeout"`)
	require.Contains(t, message, `"duration_ms":1000`)
	require.Contains(t, message, `"target_ref":"`+ArchiveTargetRef(target)+`"`)
	digest := sha256.Sum256([]byte(target))
	for _, secret := range []string{"private-password", "private-query", "private-error", "private-field", "private-bound",
		"private-caller-ref", "example.com", "target_sha256", hex.EncodeToString(digest[:])} {
		require.NotContains(t, message, secret)
	}
	select {
	case duplicate := <-messages:
		t.Fatalf("duplicate alert: %s", duplicate)
	case <-time.After(100 * time.Millisecond):
	}
}

// syntheticAdmissionError implements both explicit admission contracts for projection tests.
type syntheticAdmissionError struct {
	reason string
	cause  error
}

// Error returns private text that must never reach an alert.
func (e *syntheticAdmissionError) Error() string { return "private admission text" }

// Unwrap returns the synthetic cause.
func (e *syntheticAdmissionError) Unwrap() error { return e.cause }

// AdmissionReason returns the synthetic reason, which may be outside the reviewed set.
func (e *syntheticAdmissionError) AdmissionReason() string { return e.reason }

// syntheticStageError implements only the stage contract, like the crawler wrapper.
type syntheticStageError struct{ cause error }

// Error returns private text that must never reach an alert.
func (e *syntheticStageError) Error() string { return "private stage text" }

// Unwrap returns the synthetic cause.
func (e *syntheticStageError) Unwrap() error { return e.cause }

// CrawlerFailureStage attributes the failure to admission.
func (e *syntheticStageError) CrawlerFailureStage() string { return "admission" }

// TestArchiveAlertClassifications preserves distinct finite categories without exposing raw errors.
// Typed admission reasons win over context identities, and unknown reasons never leak.
func TestArchiveAlertClassifications(t *testing.T) {
	for _, tc := range []struct {
		name, stage, wantStage, category string
		err                              error
	}{
		{"fetch_failed", "fetch", "fetch", "fetch_failed", fmt.Errorf("private fetch cause")},
		{"storage_failed", "save", "save", "storage_failed", fmt.Errorf("private storage cause")},
		{"canceled", "fetch", "fetch", "canceled", fmt.Errorf("wrapped: %w", context.Canceled)},
		{"destination_denied", "fetch", "admission", "destination_denied",
			&syntheticStageError{cause: &syntheticAdmissionError{reason: "destination_denied", cause: fmt.Errorf("private")}}},
		{"dns_timeout_not_task_timeout", "fetch", "admission", "dns_timeout",
			&syntheticStageError{cause: &syntheticAdmissionError{reason: "dns_timeout", cause: context.DeadlineExceeded}}},
		{"dns_failed", "fetch", "admission", "dns_failed", &syntheticAdmissionError{reason: "dns_failed", cause: fmt.Errorf("private")}},
		{"unknown_reason", "fetch", "admission", "admission_failed", &syntheticAdmissionError{reason: "https://private.example/", cause: fmt.Errorf("private")}},
		{"stage_only", "fetch", "admission", "admission_failed", &syntheticStageError{cause: fmt.Errorf("private")}},
		{"stage_canceled", "fetch", "admission", "canceled", &syntheticStageError{cause: context.Canceled}},
		{"invalid_stage", "private-stage", "unknown", "operation_failed", fmt.Errorf("private")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fields := archiveAlertFields([]zapcore.Field{zap.String("stage", tc.stage), zap.Error(tc.err)})
			require.Equal(t, []zapcore.Field{zap.String("stage", tc.wantStage), zap.String("category", tc.category)}, fields)
		})
	}
}

// TestArchiveAlertBoundsMalformedFields drops wrong types and out-of-range durations.
func TestArchiveAlertBoundsMalformedFields(t *testing.T) {
	fields := archiveAlertFields([]zapcore.Field{
		zap.Int("stage", 1), zap.Int("url", 2), zap.String("error", "private"),
		zap.Duration("duration", 6*time.Minute),
	})
	require.Equal(t, []zapcore.Field{zap.String("stage", "unknown"), zap.String("category", "operation_failed")}, fields)
	fields = archiveAlertFields([]zapcore.Field{zap.String("stage", "save"), zap.Duration("duration", -time.Second)})
	require.Len(t, fields, 2)
}

// TestArchiveTargetRefIsKeyed proves the alert reference cannot be derived from
// the URL alone and is not the S3 object key, while staying stable in-process.
func TestArchiveTargetRefIsKeyed(t *testing.T) {
	target := "https://example.com/path?q=1"
	digest := sha256.Sum256([]byte(target))
	ref := ArchiveTargetRef(target)
	require.Len(t, ref, 32)
	require.NotContains(t, hex.EncodeToString(digest[:]), ref)
	require.Equal(t, ref, ArchiveTargetRef(target))
	require.NotEqual(t, ref, ArchiveTargetRef(target+"#other"))
}

// TestArchiveLoggerUsesReviewedScope binds the worker constructor to the alert
// policy, so renaming the scope cannot silently revert alerts to metadata only.
func TestArchiveLoggerUsesReviewedScope(t *testing.T) {
	observed, entries := observer.New(zapcore.DebugLevel)
	logger, err := glog.New(glog.WithName(rootLoggerName),
		glog.WithZapOptions(zap.WrapCore(func(zapcore.Core) zapcore.Core { return CrawlerDiagnosticAlertCore(observed) })))
	require.NoError(t, err)
	old := Logger
	Logger = logger
	t.Cleanup(func() { Logger = old })
	ArchiveLogger().Error("failed to fetch and save url content", zap.String("stage", "save"),
		zap.String("url", "https://private.example/"), zap.Error(fmt.Errorf("private")))
	require.Equal(t, 1, entries.Len())
	entry := entries.All()[0]
	require.Equal(t, ArchiveLoggerName, entry.LoggerName)
	require.Equal(t, map[string]any{"stage": "save", "category": "storage_failed",
		"target_ref": ArchiveTargetRef("https://private.example/")}, entry.ContextMap())
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
	require.NoError(t, hook(zapcore.Entry{LoggerName: ArchiveLoggerName}, []zapcore.Field{zap.String("stage", "save"), zap.Error(fmt.Errorf("private-save-error"))}))
	require.Equal(t, []zapcore.Field{zap.String("stage", "save"), zap.String("category", "storage_failed")}, captured)
	require.NoError(t, hook(zapcore.Entry{LoggerName: "go-ramjet"}, []zapcore.Field{zap.String("private", "private-value")}))
	require.Empty(t, captured)
}
