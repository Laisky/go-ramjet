package log

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Laisky/zap/zapcore"

	"github.com/Laisky/zap"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdklog "go.opentelemetry.io/otel/sdk/log"
)

// recordAttributes projects synthetic attributes for fidelity and privacy assertions.
func recordAttributes(record sdklog.Record) map[string]any {
	attrs := map[string]any{}
	record.WalkAttributes(func(kv attribute.KeyValue) bool { attrs[string(kv.Key)] = kv.Value.AsInterface(); return true })
	return attrs
}

// TestOTLPContentFidelity reproduces operational content lost by the former constant-body mapper.
func TestOTLPContentFidelity(t *testing.T) {
	sink := &captureExporter{}
	isolateOTLP(t, newOTLPPipeline(sink, "b1", "test", 512, 128, time.Hour))
	Logger.With(zap.String("stage", "cache")).Info("synthetic operational refresh completed",
		zap.String("future_revision", "parser-v2"), zap.Int("channel_count", 17), zap.Float64("duration_ms", 12.5), zap.Bool("cache_ready", true),
		zap.Int("token_count", 42), zap.Float64("query_latency", 25.5), zap.Bool("password_reset_required", true), zap.Error(fmt.Errorf("refresh retained diagnostics: unexpected EOF")),
		zap.String("api_key", "private-api-canary"), zap.String("authorization", "Bearer private-auth-canary"), zap.String("target_url", "https://user:private-url-canary@example.test/private?token=private-query-canary"), zap.String("request_body", "private-body-canary"))
	require.NoError(t, ShutdownOTLP(context.Background()))
	records := sink.snapshot()
	require.Len(t, records, 1)
	require.Equal(t, "synthetic operational refresh completed", records[0].Body().AsString())
	attrs := recordAttributes(records[0])
	require.Equal(t, "cache", attrs["stage"])
	require.Equal(t, "parser-v2", attrs["future_revision"])
	require.EqualValues(t, 17, attrs["channel_count"])
	require.Equal(t, 12.5, attrs["duration_ms"])
	require.Equal(t, true, attrs["cache_ready"])
	require.EqualValues(t, 42, attrs["token_count"])
	require.Equal(t, 25.5, attrs["query_latency"])
	require.Equal(t, true, attrs["password_reset_required"])
	require.Contains(t, attrs["error"], "unexpected EOF")
	require.NotContains(t, fmt.Sprint(attrs), "private-")
	require.NotContains(t, fmt.Sprint(attrs), "example.test")
}

// TestOTLPExistingTickerMessageAndDuration preserves an actual TaskStore logging contract.
func TestOTLPExistingTickerMessageAndDuration(t *testing.T) {
	sink := &captureExporter{}
	isolateOTLP(t, newOTLPPipeline(sink, "b1", "test", 512, 128, time.Hour))
	// TaskStore.Ticker emits this literal and zap.Duration field before scheduling.
	Logger.Info("Ticker", zap.Duration("interval", 2500*time.Millisecond))
	require.NoError(t, ShutdownOTLP(context.Background()))
	records := sink.snapshot()
	require.Len(t, records, 1)
	require.Equal(t, "Ticker", records[0].Body().AsString())
	require.Equal(t, 2.5, recordAttributes(records[0])["interval"])
}

// TestOTLPTextRedactionAndFalsePositives preserves future diagnostics and metrics without recognizable secrets.
func TestOTLPTextRedactionAndFalsePositives(t *testing.T) {
	body, attrs := projectOTLPContent(zapcore.Entry{Message: "refresh failed api_key=private-message-canary; unexpected EOF; token_count=42 query_latency=25ms"}, []zapcore.Field{
		zap.String("future_diagnostic_name", "future category api_key=private-field-canary; HTTP 429"),
		zap.String("details", "connection failed opaque-token-canary"), zap.String("args_truncated", "private-arguments-canary"), zap.String("response_truncated", "private-response-canary"),
		zap.Int("auth_failure_count_v2", 2), zap.Bool("password_reset_required", true), zap.Any("unsupported", explosiveValue{}), zap.Error(panickingOTLPError{}),
	}, "opaque-token-canary")
	joined := body + fmt.Sprint(attrs)
	require.NotContains(t, joined, "canary")
	require.Contains(t, body, "unexpected EOF")
	require.Contains(t, body, "token_count=42 query_latency=25ms")
	require.Contains(t, joined, "future category")
	require.Contains(t, joined, "HTTP 429")
	require.Contains(t, joined, "auth_failure_count_v2")
	require.Contains(t, joined, "password_reset_required")
	require.Contains(t, joined, "app.log.attributes_dropped")
	require.Contains(t, joined, "error text unavailable")
}

// panickingOTLPError ensures unsafe error implementations cannot escape projection.
type panickingOTLPError struct{}

// Error models a diagnostic formatter panic without exposing its panic value remotely.
func (panickingOTLPError) Error() string { panic("private-error-panic-canary") }

// TestOTLPBoundsAndIdentityProtection marks content limits and rejects poisoned resource metadata.
func TestOTLPBoundsAndIdentityProtection(t *testing.T) {
	fields := []zapcore.Field{zap.String("service.name", "poisoned-service"), zap.String("host.name", "poisoned-host"), zap.String("deployment.environment.name", "poisoned-env"), zap.Int("app.log.attributes_dropped", -1), zap.String("invalid\nkey", "poisoned-control"), zap.String(string([]byte{255}), "poisoned-utf8"), zap.String("future_revision", strings.Repeat("x", 3000))}
	for index := 0; index < 100; index++ {
		fields = append(fields, zap.Int(fmt.Sprintf("future_%d", index), index))
	}
	body, attrs := projectOTLPContent(zapcore.Entry{Message: strings.Repeat("b", 5000)}, fields, "")
	require.LessOrEqual(t, len(body), maxOTLPBodyBytes)
	require.LessOrEqual(t, len(attrs), maxOTLPAttributes)
	joined := fmt.Sprint(attrs)
	require.NotContains(t, joined, "poisoned-")
	require.Contains(t, joined, "app.log.content_truncated")
	require.Contains(t, joined, "app.log.attributes_dropped")
	for _, attr := range attrs {
		if attr.Key == "future_revision" {
			require.LessOrEqual(t, len(attr.Value.AsString()), maxOTLPStringBytes)
		}
	}
}
