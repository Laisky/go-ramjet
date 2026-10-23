package log

import (
	"bytes"
	"context"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	collectorlog "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	"google.golang.org/protobuf/proto"
)

// captureExporter retains only synthetic records for lifecycle assertions.
type captureExporter struct {
	mu      sync.Mutex
	records []sdklog.Record
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

// Export optionally blocks until released and copies synthetic records.
func (e *captureExporter) Export(ctx context.Context, records []sdklog.Record) error {
	if e.started != nil {
		e.once.Do(func() { close(e.started) })
	}
	if e.release != nil {
		select {
		case <-e.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, record := range records {
		e.records = append(e.records, record.Clone())
	}
	return nil
}

// Shutdown releases no resources owned by the in-memory fixture.
func (e *captureExporter) Shutdown(context.Context) error { return nil }

// ForceFlush is unnecessary because the fixture stores records synchronously.
func (e *captureExporter) ForceFlush(context.Context) error { return nil }

// snapshot returns independent fixture record storage.
func (e *captureExporter) snapshot() []sdklog.Record {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]sdklog.Record(nil), e.records...)
}

// isolateOTLP installs a private synthetic pipeline and restores process globals.
func isolateOTLP(t *testing.T, p *otlpPipeline) {
	t.Helper()
	require.Nil(t, otlpState.current.Load())
	otlpState.Lock()
	previous := otlpState.retained
	otlpState.retained = p
	otlpState.current.Store(p)
	otlpState.Unlock()
	previousLogger := Logger
	Logger = Logger.WithOptions(privateOTLPOption())
	t.Cleanup(func() {
		require.NoError(t, ShutdownOTLP(context.Background()))
		otlpState.Lock()
		otlpState.retained = previous
		otlpState.Unlock()
		Logger = previousLogger
	})
}

// TestOTLPInitCapturedChildrenAndFidelity verifies startup-captured children retain supplied content once enabled.
func TestOTLPInitCapturedChildrenAndFidelity(t *testing.T) {
	t.Setenv("APP_LOG_OTLP_ENDPOINT", "https://synthetic.invalid")
	previousLogger := Logger
	Logger = Logger.WithOptions(privateOTLPOption())
	t.Cleanup(func() { Logger = previousLogger })
	captured := Logger.Named("worker token=private-dynamic-name-canary").With(zap.String("authorization", "bound-secret-canary"), zap.Int("attempt", 2))
	sink := &captureExporter{}
	p := newOTLPPipeline(sink, "b1", "production", 512, 128, time.Hour)
	isolateOTLP(t, p)
	captured.Error("cache refresh api_key=message-secret-canary", zap.String("password", "field-secret-canary"), zap.Any("count", map[string]string{"private": "object-secret-canary"}), zap.Int("status_code", 503), zap.Error(fmt.Errorf("unexpected EOF api_key=error-secret-canary")))
	require.NoError(t, ShutdownOTLP(context.Background()))
	records := sink.snapshot()
	require.Len(t, records, 1)
	record := records[0]
	require.Equal(t, "cache refresh api_key=message-secret-canary", record.Body().AsString())
	attrs := map[string]any{}
	record.WalkAttributes(func(kv attribute.KeyValue) bool { attrs[string(kv.Key)] = kv.Value.AsInterface(); return true })
	require.EqualValues(t, 2, attrs["attempt"])
	require.EqualValues(t, 503, attrs["status_code"])
	require.Contains(t, fmt.Sprint(attrs), "canary")
	require.Contains(t, attrs, "count")
	var service string
	for _, attr := range record.Resource().Attributes() {
		if attr.Key == "service.name" {
			service = attr.Value.AsString()
		}
	}
	require.Equal(t, "go-ramjet", service)
	require.Equal(t, "github.com/Laisky/go-ramjet/library/log", record.InstrumentationScope().Name)
	stats := LogOTLPStats()
	require.EqualValues(t, 1, stats.Emitted)
	require.EqualValues(t, 1, stats.Exported)
	require.Zero(t, stats.Unresolved)
}

// TestOTLPStdoutAndSingleHook preserves the original sink and one remote event.
func TestOTLPStdoutAndSingleHook(t *testing.T) {
	sink := &captureExporter{}
	isolateOTLP(t, newOTLPPipeline(sink, "", "", 512, 128, time.Hour))
	var local bytes.Buffer
	logger, err := glog.NewWithName("go-ramjet", glog.LevelInfo, zap.WrapCore(func(zapcore.Core) zapcore.Core {
		return zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()), zapcore.AddSync(&local), zapcore.InfoLevel)
	}), privateOTLPOption())
	require.NoError(t, err)
	logger.Info("local-useful-evidence", zap.String("local_only", "kept"))
	logger.Debug("suppressed")
	require.Contains(t, local.String(), "local-useful-evidence")
	require.Contains(t, local.String(), "kept")
	require.NoError(t, ShutdownOTLP(context.Background()))
	require.Len(t, sink.snapshot(), 1)
}

// TestOTLPDisabledAndInvalidConfig verifies opt-in, sanitized errors and HTTPS enforcement.
func TestOTLPDisabledAndInvalidConfig(t *testing.T) {
	t.Setenv("APP_LOG_OTLP_ENDPOINT", "")
	t.Setenv("APP_LOG_OTLP_TOKEN_FILE", "/nonexistent-secret-canary")
	shutdown, err := ConfigureOTLP(context.Background())
	require.NoError(t, err)
	require.NoError(t, shutdown(context.Background()))
	require.False(t, LogOTLPStats().Enabled)
	for _, endpoint := range []string{"http://127.0.0.1:1234", "https://secret-canary@example.com", "https://example.com/?secret-canary=1", "https://example.com/v1/logs"} {
		t.Setenv("APP_LOG_OTLP_ENDPOINT", endpoint)
		_, err := ConfigureOTLP(context.Background())
		require.Error(t, err)
		require.NotContains(t, err.Error(), "secret-canary")
	}
}

// TestOTLPTLSConfigIdempotenceAndShutdown validates the real SDK HTTP serializer and opaque files.
func TestOTLPTLSConfigIdempotenceAndShutdown(t *testing.T) {
	requests := make(chan []byte, 4)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer synthetic-token" {
			w.WriteHeader(401)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(400)
			return
		}
		requests <- body
		w.Header().Set("Content-Type", "application/x-protobuf")
		w.WriteHeader(200)
	}))
	defer server.Close()
	dir := t.TempDir()
	tokenFile := filepath.Join(dir, "token")
	caFile := filepath.Join(dir, "ca.pem")
	require.NoError(t, os.WriteFile(tokenFile, []byte("synthetic-token\n"), 0600))
	require.NoError(t, os.WriteFile(caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600))
	for k, v := range map[string]string{"APP_LOG_OTLP_ENDPOINT": server.URL, "APP_LOG_OTLP_TOKEN_FILE": tokenFile, "APP_LOG_OTLP_CA_FILE": caFile, "APP_LOG_OTLP_HOST": "b1", "APP_LOG_OTLP_ENV": "production", "OTEL_EXPORTER_OTLP_HEADERS": "Authorization=secret-canary", "OTEL_EXPORTER_OTLP_INSECURE": "true"} {
		t.Setenv(k, v)
	}
	otlpState.Lock()
	previous := otlpState.retained
	otlpState.retained = nil
	otlpState.Unlock()
	t.Cleanup(func() {
		require.NoError(t, ShutdownOTLP(context.Background()))
		otlpState.Lock()
		otlpState.retained = previous
		otlpState.Unlock()
	})
	shutdown, err := ConfigureOTLP(context.Background())
	require.NoError(t, err)
	previousLogger := Logger
	Logger = Logger.WithOptions(privateOTLPOption())
	t.Cleanup(func() { Logger = previousLogger })
	original := otlpState.current.Load()
	_, err = ConfigureOTLP(context.Background())
	require.NoError(t, err)
	require.Same(t, original, otlpState.current.Load())
	Logger.Info("cache refresh token=private-message-canary", zap.Int("attempt", 1), zap.String("configured_token_echo", "synthetic-token"))
	require.NoError(t, shutdown(context.Background()))
	select {
	case body := <-requests:
		require.Contains(t, string(body), "private-message-canary")
		require.Contains(t, string(body), "synthetic-token")
		var req collectorlog.ExportLogsServiceRequest
		require.NoError(t, proto.Unmarshal(body, &req))
		require.Len(t, req.ResourceLogs, 1)
		require.Len(t, req.ResourceLogs[0].ScopeLogs[0].LogRecords, 1)
	case <-time.After(time.Second):
		t.Fatal("missing SDK request")
	}
	require.Empty(t, requests)
	require.NoError(t, shutdown(context.Background()))
}

// TestOTLPBoundedQueueAndRestartBoundary reproduces overflow without blocking application producers.
func TestOTLPBoundedQueueAndRestartBoundary(t *testing.T) {
	sink := &captureExporter{started: make(chan struct{}), release: make(chan struct{})}
	p := newOTLPPipeline(sink, "", "", 2, 1, time.Hour)
	isolateOTLP(t, p)
	Logger.Info("first")
	select {
	case <-sink.started:
	case <-time.After(time.Second):
		t.Fatal("export never started")
	}
	before := time.Now()
	for i := 0; i < 20; i++ {
		Logger.Info("queued", zap.Int("attempt", i))
	}
	require.Less(t, time.Since(before), time.Second)
	close(sink.release)
	require.NoError(t, ShutdownOTLP(context.Background()))
	require.Len(t, sink.snapshot(), 3)
	stats := LogOTLPStats()
	require.EqualValues(t, 21, stats.Emitted)
	require.EqualValues(t, 18, stats.Unresolved)
	// A new process/provider has no access to the prior SDK buffer or counters.
	next := &captureExporter{}
	fresh := newOTLPPipeline(next, "", "", 2, 1, time.Hour)
	require.Zero(t, fresh.emitted.Load())
	require.NoError(t, fresh.provider.Shutdown(context.Background()))
	require.Empty(t, next.snapshot())
}

// TestOTLPHTTP503Retry exercises actual exporter retry rather than a mocked success path.
func TestOTLPHTTP503Retry(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(503)
			_, _ = io.WriteString(w, "response-secret-canary")
			return
		}
		w.Header().Set("Content-Type", "application/x-protobuf")
		w.WriteHeader(200)
	}))
	defer server.Close()
	exporter, err := otlploghttp.New(context.Background(), otlploghttp.WithEndpointURL(server.URL), otlploghttp.WithInsecure(), otlploghttp.WithHeaders(map[string]string{}), otlploghttp.WithTimeout(time.Second), otlploghttp.WithRetry(otlploghttp.RetryConfig{Enabled: true, InitialInterval: time.Millisecond, MaxInterval: 5 * time.Millisecond, MaxElapsedTime: time.Second}))
	require.NoError(t, err)
	p := newOTLPPipeline(exporter, "", "", 2, 1, time.Hour)
	isolateOTLP(t, p)
	Logger.Info("retry")
	require.NoError(t, ShutdownOTLP(context.Background()))
	require.EqualValues(t, 2, calls.Load())
	require.EqualValues(t, 1, LogOTLPStats().Exported)
	require.Zero(t, LogOTLPStats().Failed)
}

// TestOTLPShutdownConcurrentEmission serializes admission before final draining.
func TestOTLPShutdownConcurrentEmission(t *testing.T) {
	sink := &captureExporter{}
	isolateOTLP(t, newOTLPPipeline(sink, "", "", 512, 128, time.Millisecond))
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Go(func() {
			for j := 0; j < 20; j++ {
				_ = otlpHook(zapcore.Entry{Level: zapcore.InfoLevel, Time: time.Now()}, nil)
			}
		})
	}
	require.NoError(t, ShutdownOTLP(context.Background()))
	wg.Wait()
	require.Zero(t, LogOTLPStats().Unresolved)
	require.EqualValues(t, len(sink.snapshot()), LogOTLPStats().Emitted)
}

// TestOTLPSDKErrorSanitization captures actual SDK errors from a terminal HTTP failure.
func TestOTLPSDKErrorSanitization(t *testing.T) {
	reports := make(chan string, 4)
	previous := otel.GetErrorHandler()
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) { reports <- err.Error() }))
	t.Cleanup(func() { otel.SetErrorHandler(previous) })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, "provider-response-secret-canary")
	}))
	defer server.Close()
	exporter, err := otlploghttp.New(context.Background(), otlploghttp.WithEndpointURL(server.URL), otlploghttp.WithInsecure(), otlploghttp.WithHeaders(map[string]string{}), otlploghttp.WithTimeout(time.Second))
	require.NoError(t, err)
	isolateOTLP(t, newOTLPPipeline(exporter, "", "", 2, 1, time.Hour))
	Logger.Info("private-message-canary")
	select {
	case report := <-reports:
		require.Equal(t, "OTLP log export failed", report)
	case <-time.After(2 * time.Second):
		t.Fatal("missing SDK failure report")
	}
	require.NoError(t, ShutdownOTLP(context.Background()))
	require.EqualValues(t, 1, LogOTLPStats().Failed)
}

// TestOTLPExternalAcceptance sends one synthetic fidelity record through an explicitly opted-in HTTPS collector.
// Runtime credentials are consumed through the production file API and never printed.
func TestOTLPExternalAcceptance(t *testing.T) {
	if os.Getenv("OTLP_LOG_ACCEPTANCE") != "1" {
		t.Skip("external OTLP acceptance is opt-in")
	}
	require.Nil(t, otlpState.current.Load())
	otlpState.Lock()
	previous := otlpState.retained
	otlpState.retained = nil
	otlpState.Unlock()
	t.Cleanup(func() {
		require.NoError(t, ShutdownOTLP(context.Background()))
		otlpState.Lock()
		otlpState.retained = previous
		otlpState.Unlock()
	})
	shutdown, err := ConfigureOTLP(context.Background())
	require.NoError(t, err)
	require.True(t, LogOTLPStats().Enabled, "APP_LOG_OTLP_ENDPOINT must enable the production HTTPS path")
	Logger.Info("synthetic operational refresh completed", zap.String("stage", "cache"), zap.String("future_revision", "parser-v2"), zap.Int("channel_count", 17), zap.Float64("duration_ms", 12.5), zap.Bool("cache_ready", true), zap.String("api_key", "external-private-api-canary"), zap.String("authorization", "Bearer external-private-auth-canary"), zap.String("target_url", "https://user:external-private-url-canary@example.test/private?token=external-private-query-canary"), zap.String("request_body", "external-private-body-canary"), zap.Int("status_code", 299), zap.Bool("success", true),
		zap.Object("object", fidelityObject{}), zap.Array("array", fidelityArray{}), zap.Reflect("null_value", nil), zap.Reflect("empty_object", map[string]any{}), zap.Reflect("empty_array", []any{}),
		zap.String("service.name", "application-service"), zap.String("host.name", "application-host"), zap.String("_msg", "application-field"),
		zap.Namespace("namespace"), zap.String("payload", "synthetic-namespace-canary"))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, shutdown(ctx))
	stats := LogOTLPStats()
	require.EqualValues(t, 1, stats.Emitted)
	require.EqualValues(t, 1, stats.Exported)
	require.Zero(t, stats.Failed)
	require.Zero(t, stats.Rejected)
	require.Zero(t, stats.Unresolved)
}
