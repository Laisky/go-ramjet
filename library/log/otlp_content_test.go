package log

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdklog "go.opentelemetry.io/otel/sdk/log"
)

// recordAttributes projects synthetic attributes into ordinary values for assertions.
func recordAttributes(record sdklog.Record) map[string]any {
	attrs := map[string]any{}
	record.WalkAttributes(func(kv attribute.KeyValue) bool { attrs[string(kv.Key)] = otlpTestValue(kv.Value); return true })
	return attrs
}

// otlpTestValue converts native nested OTLP fixture values into comparable test values.
func otlpTestValue(value attribute.Value) any {
	switch value.Type() {
	case attribute.MAP:
		fields := map[string]any{}
		for _, field := range value.AsMap() {
			fields[string(field.Key)] = otlpTestValue(field.Value)
		}
		return fields
	case attribute.SLICE:
		values := []any{}
		for _, child := range value.AsSlice() {
			values = append(values, otlpTestValue(child))
		}
		return values
	default:
		return value.AsInterface()
	}
}

// TestOTLPContentFidelity retains ordinary operational fields and synthetic credential-shaped values.
func TestOTLPContentFidelity(t *testing.T) {
	sink := &captureExporter{}
	isolateOTLP(t, newOTLPPipeline(sink, "b1", "test", 512, 128, time.Hour))
	Logger.With(zap.String("stage", "cache")).Info("synthetic operational refresh completed",
		zap.String("future_revision", "parser-v2"), zap.Int("channel_count", 17), zap.Float64("duration_ms", 12.5), zap.Bool("cache_ready", true),
		zap.Int("token_count", 42), zap.Float64("query_latency", 25.5), zap.Bool("password_reset_required", true), zap.Error(fmt.Errorf("refresh retained diagnostics: unexpected EOF")),
		zap.String("api_key", "synthetic-api-canary"), zap.String("authorization", "Bearer synthetic-auth-canary"), zap.String("target_url", "https://user:synthetic-url-canary@example.test/private?token=synthetic-query-canary"), zap.String("request_body", "synthetic-body-canary"))
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
	require.Equal(t, "synthetic-api-canary", attrs["api_key"])
	require.Equal(t, "Bearer synthetic-auth-canary", attrs["authorization"])
	require.Equal(t, "synthetic-body-canary", attrs["request_body"])
	require.Contains(t, attrs["target_url"], "synthetic-query-canary")
}

// TestOTLPExistingTickerMessageAndDuration preserves the actual TaskStore logging contract.
func TestOTLPExistingTickerMessageAndDuration(t *testing.T) {
	sink := &captureExporter{}
	isolateOTLP(t, newOTLPPipeline(sink, "b1", "test", 512, 128, time.Hour))
	Logger.Info("Ticker", zap.Duration("interval", 2500*time.Millisecond))
	require.NoError(t, ShutdownOTLP(context.Background()))
	records := sink.snapshot()
	require.Len(t, records, 1)
	require.Equal(t, "Ticker", records[0].Body().AsString())
	require.EqualValues(t, 2500000000, recordAttributes(records[0])["interval"])
}

// fidelityObject models an application-owned structured encoder.
type fidelityObject struct{}

// MarshalLogObject supplies normal nested fields without bridge-specific annotations.
func (fidelityObject) MarshalLogObject(encoder zapcore.ObjectEncoder) error {
	encoder.AddString("api_key", "synthetic-object-canary")
	encoder.AddInt("count", 7)
	return nil
}

// fidelityArray models an application-owned heterogeneous array encoder.
type fidelityArray struct{}

// MarshalLogArray supplies primitive and nested values using ordinary Zap semantics.
func (fidelityArray) MarshalLogArray(encoder zapcore.ArrayEncoder) error {
	encoder.AppendString("synthetic-array-canary")
	encoder.AppendInt(9)
	return encoder.AppendObject(fidelityObject{})
}

// fidelityStringer models an application diagnostic string.
type fidelityStringer struct{}

// String returns synthetic content whose exact text must survive transport.
func (fidelityStringer) String() string { return "api_key=synthetic-stringer-canary" }

// TestOTLPStructuredValuesUseApplicationEncoder retains nested, reflected, binary and namespace values.
func TestOTLPStructuredValuesUseApplicationEncoder(t *testing.T) {
	sink := &captureExporter{}
	isolateOTLP(t, newOTLPPipeline(sink, "b1", "test", 512, 128, time.Hour))
	at := time.Date(2026, 10, 18, 4, 0, 1, 123, time.FixedZone("fixture", 3600))
	Logger.Info("structured", zap.Object("object", fidelityObject{}), zap.Array("array", fidelityArray{}),
		zap.Reflect("reflected", struct {
			Value string `json:"request_body"`
		}{"synthetic-reflected-canary"}),
		zap.Stringer("stringer", fidelityStringer{}), zap.Binary("binary", []byte{0, 255}), zap.ByteString("bytes", []byte("synthetic-bytes-canary")),
		zap.Time("at", at), zap.Uint64("unsigned", math.MaxUint64), zap.Float64("nonfinite", math.Inf(1)),
		zap.Reflect("null", nil), zap.Reflect("empty_object", map[string]any{}), zap.Reflect("empty_array", []any{}), zap.Namespace("namespace"), zap.String("payload", "synthetic-namespace-canary"))
	require.NoError(t, ShutdownOTLP(context.Background()))
	records := sink.snapshot()
	require.Len(t, records, 1)
	attrs := recordAttributes(records[0])
	object := map[string]any{"api_key": "synthetic-object-canary", "count": int64(7)}
	require.Equal(t, object, attrs["object"])
	require.Equal(t, []any{"synthetic-array-canary", int64(9), object}, attrs["array"])
	require.Equal(t, map[string]any{"request_body": "synthetic-reflected-canary"}, attrs["reflected"])
	require.Equal(t, "api_key=synthetic-stringer-canary", attrs["stringer"])
	require.Equal(t, []byte{0, 255}, attrs["binary"])
	require.Equal(t, "synthetic-bytes-canary", attrs["bytes"])
	require.Equal(t, "2026-10-18T04:00:01.000000123+01:00", attrs["at"])
	require.Equal(t, "18446744073709551615", attrs["unsigned"])
	require.True(t, math.IsInf(attrs["nonfinite"].(float64), 1))
	require.Contains(t, attrs, "null")
	require.Nil(t, attrs["null"])
	require.Equal(t, map[string]any{}, attrs["empty_object"])
	require.Equal(t, []any{}, attrs["empty_array"])
	require.Equal(t, map[string]any{"payload": "synthetic-namespace-canary"}, attrs["namespace"])
}

// TestOTLPIdentityNamedFieldsRemainApplicationAttributes keeps resource and record scopes separate.
func TestOTLPIdentityNamedFieldsRemainApplicationAttributes(t *testing.T) {
	sink := &captureExporter{}
	isolateOTLP(t, newOTLPPipeline(sink, "resource-host", "test", 512, 128, time.Hour))
	Logger.Info("identity", zap.String("service.name", "application-service"), zap.String("host.name", "application-host"), zap.String("_msg", "application-field"), zap.String("logger.name", "application-logger"))
	require.NoError(t, ShutdownOTLP(context.Background()))
	records := sink.snapshot()
	require.Len(t, records, 1)
	attrs := recordAttributes(records[0])
	require.Equal(t, "application-service", attrs["service.name"])
	require.Equal(t, "application-host", attrs["host.name"])
	require.Equal(t, "application-field", attrs["_msg"])
	require.Equal(t, "application-logger", attrs["logger.name"])
	for _, attr := range records[0].Resource().Attributes() {
		if attr.Key == "service.name" {
			require.Equal(t, "go-ramjet", attr.Value.AsString())
		}
	}
}

// TestOTLPProtocolRejectionIsObservable rejects whole invalid records instead of silently rewriting fields.
func TestOTLPProtocolRejectionIsObservable(t *testing.T) {
	for _, fixture := range []struct {
		message string
		fields  []zapcore.Field
	}{
		{message: "invalid-key", fields: []zapcore.Field{zap.String(string([]byte{255}), "synthetic-value")}}, {message: "empty-key", fields: []zapcore.Field{zap.String("", "synthetic-value")}},
	} {
		t.Run(fmt.Sprintf("fixture-%d", len(fixture.fields)), func(t *testing.T) {
			sink := &captureExporter{}
			isolateOTLP(t, newOTLPPipeline(sink, "b1", "test", 512, 128, time.Hour))
			require.Error(t, otlpHook(zapcore.Entry{Message: fixture.message}, fixture.fields))
			require.NoError(t, ShutdownOTLP(context.Background()))
			require.Empty(t, sink.snapshot())
			require.EqualValues(t, 1, LogOTLPStats().Rejected)
			require.Zero(t, LogOTLPStats().Emitted)
		})
	}
}

// TestOTLPDefaultContentFidelity preserves application-supplied content without bridge policy.
func TestOTLPDefaultContentFidelity(t *testing.T) {
	t.Setenv("OTEL_LOGRECORD_ATTRIBUTE_COUNT_LIMIT", "0")
	t.Setenv("OTEL_LOGRECORD_ATTRIBUTE_VALUE_LENGTH_LIMIT", "1")
	sink := &captureExporter{}
	isolateOTLP(t, newOTLPPipeline(sink, "b1", "test", 512, 128, time.Hour))
	body := "application diagnostic api_key=synthetic-message-canary " + strings.Repeat("m", 5000)
	fields := []zapcore.Field{zap.String(strings.Repeat("long_", 40), "synthetic-long-key"), zap.String("api_key", "synthetic-field-canary"), zap.String("request_body", "synthetic-request-canary"), zap.String("target_url", "https://example.test/?token=synthetic-query-canary"), zap.String("future_unknown", strings.Repeat("v", 3000))}
	for i := range 160 {
		fields = append(fields, zap.Int(fmt.Sprintf("future_%d", i), i))
	}
	Logger.Info(body, fields...)
	require.NoError(t, ShutdownOTLP(context.Background()))
	records := sink.snapshot()
	require.Len(t, records, 1)
	require.Equal(t, body, records[0].Body().AsString())
	attrs := recordAttributes(records[0])
	require.Equal(t, "synthetic-field-canary", attrs["api_key"])
	require.Equal(t, "synthetic-long-key", attrs[strings.Repeat("long_", 40)])
	require.Equal(t, "synthetic-request-canary", attrs["request_body"])
	require.Equal(t, "https://example.test/?token=synthetic-query-canary", attrs["target_url"])
	require.Equal(t, strings.Repeat("v", 3000), attrs["future_unknown"])
	require.EqualValues(t, 159, attrs["future_159"])
}

// TestOTLPDeepValuesRemainIntact disables implicit SDK nesting limits explicitly.
func TestOTLPDeepValuesRemainIntact(t *testing.T) {
	sink := &captureExporter{}
	isolateOTLP(t, newOTLPPipeline(sink, "b1", "test", 512, 128, time.Hour))
	var nested any = "synthetic-deep-canary"
	for range 96 {
		nested = map[string]any{"child": nested}
	}
	Logger.Info("deep", zap.Reflect("nested", nested))
	require.NoError(t, ShutdownOTLP(context.Background()))
	records := sink.snapshot()
	require.Len(t, records, 1)
	value := recordAttributes(records[0])["nested"]
	for range 96 {
		value = value.(map[string]any)["child"]
	}
	require.Equal(t, "synthetic-deep-canary", value)
}

// TestOTLPJSONNumericRepresentability follows official decimal-string and native floating-point mappings.
func TestOTLPJSONNumericRepresentability(t *testing.T) {
	sink := &captureExporter{}
	isolateOTLP(t, newOTLPPipeline(sink, "b1", "test", 512, 128, time.Hour))
	Logger.Info("numeric", zap.Float64("negative_zero", math.Copysign(0, -1)), zap.Reflect("zero_exponent", json.RawMessage("0e-1000")),
		zap.Reflect("tiny", json.RawMessage("1e-1000")), zap.Reflect("huge", json.RawMessage("1e1000")), zap.Reflect("precise", json.RawMessage("1.00000000000000000001")), zap.Reflect("ordinary", json.RawMessage("0.1")))
	require.NoError(t, ShutdownOTLP(context.Background()))
	records := sink.snapshot()
	require.Len(t, records, 1)
	attrs := recordAttributes(records[0])
	require.True(t, math.Signbit(attrs["negative_zero"].(float64)))
	require.Equal(t, float64(0), attrs["zero_exponent"])
	require.Equal(t, "1e-1000", attrs["tiny"])
	require.Equal(t, "1e1000", attrs["huge"])
	require.Equal(t, "1.00000000000000000001", attrs["precise"])
	require.Equal(t, 0.1, attrs["ordinary"])
	require.Zero(t, LogOTLPStats().Rejected)
	require.EqualValues(t, 1, LogOTLPStats().Exported)
}

// TestOTLPConfiguredErrorOutputReportsRejection exposes rejected records without echoing their content.
func TestOTLPConfiguredErrorOutputReportsRejection(t *testing.T) {
	sink := &captureExporter{}
	isolateOTLP(t, newOTLPPipeline(sink, "b1", "test", 512, 128, time.Hour))
	var output bytes.Buffer
	logger, err := glog.NewWithName("go-ramjet", glog.LevelInfo, zap.ErrorOutput(zapcore.AddSync(&output)), privateOTLPOption())
	require.NoError(t, err)
	logger.Info("synthetic-unrepresentable-body-canary", zap.String("", "synthetic-unrepresentable-field-canary"))
	require.Contains(t, output.String(), "OTLP log value cannot be represented")
	require.NotContains(t, output.String(), "canary")
	require.EqualValues(t, 1, LogOTLPStats().Rejected)
}

// TestOTLPRawReflectedUnicode preserves invalid source bytes and rejects lone surrogate escapes.
func TestOTLPRawReflectedUnicode(t *testing.T) {
	for _, raw := range []json.RawMessage{json.RawMessage(`"\ud800"`), json.RawMessage(`"\udc00"`)} {
		_, err := projectOTLPContent(zapcore.Entry{}, []zapcore.Field{zap.Reflect("raw", raw)})
		require.Error(t, err)
	}
	attrs, err := projectOTLPContent(zapcore.Entry{}, []zapcore.Field{zap.Reflect("raw", json.RawMessage([]byte{'"', 255, '"'})), zap.Reflect("pair", json.RawMessage(`"\ud83d\ude00"`)), zap.String("literal", `\ud800`), zap.String("ordinary", string([]byte{255}))})
	require.NoError(t, err)
	fields := map[string]any{}
	for _, field := range attrs {
		fields[string(field.Key)] = field.Value.AsInterface()
	}
	require.Equal(t, "😀", fields["pair"])
	require.Equal(t, `\ud800`, fields["literal"])
	require.Equal(t, []byte{255}, fields["ordinary"])
	require.Equal(t, []byte{255}, fields["raw"])
}

// TestOTLPInvalidMessageBytesRemainAvailable uses native bytes when the application message is not Unicode.
func TestOTLPInvalidMessageBytesRemainAvailable(t *testing.T) {
	sink := &captureExporter{}
	isolateOTLP(t, newOTLPPipeline(sink, "b1", "test", 512, 128, time.Hour))
	require.NoError(t, otlpHook(zapcore.Entry{Message: string([]byte{0, 255})}, nil))
	require.NoError(t, ShutdownOTLP(context.Background()))
	records := sink.snapshot()
	require.Len(t, records, 1)
	require.Equal(t, []byte{0, 255}, records[0].Body().AsByteSlice())
}

// TestOTLPReflectedMultimapRetainsArrayValues preserves source arrays as single multimap elements.
func TestOTLPReflectedMultimapRetainsArrayValues(t *testing.T) {
	attrs, err := projectOTLPContent(zapcore.Entry{}, []zapcore.Field{zap.Reflect("multi", json.RawMessage(`{"repeat":[1],"repeat":2,"single":3}`))})
	require.NoError(t, err)
	require.Len(t, attrs, 1)
	fields := otlpTestValue(attrs[0].Value).(map[string]any)
	require.Equal(t, []any{[]any{int64(1)}, int64(2)}, fields["repeat"])
	require.Equal(t, []any{int64(3)}, fields["single"])
}

// fidelityCountingObject records normal application callback invocation count.
type fidelityCountingObject struct{ calls *int }

// MarshalLogObject supplies a deterministic field while counting one projection invocation.
func (object fidelityCountingObject) MarshalLogObject(encoder zapcore.ObjectEncoder) error {
	*object.calls = *object.calls + 1
	encoder.AddString("value", "synthetic-callback-canary")
	return nil
}

// TestOTLPProjectionInvokesApplicationEncoderOnce avoids extra validation serialization passes.
func TestOTLPProjectionInvokesApplicationEncoderOnce(t *testing.T) {
	calls := 0
	attrs, err := projectOTLPContent(zapcore.Entry{}, []zapcore.Field{zap.Object("object", fidelityCountingObject{calls: &calls})})
	require.NoError(t, err)
	require.Equal(t, 1, calls)
	require.Len(t, attrs, 1)
	require.Equal(t, "synthetic-callback-canary", attrs[0].Value.AsMap()[0].Value.AsString())
}

// fidelityBlockingObject models an application callback whose completion is independently controlled.
type fidelityBlockingObject struct {
	started chan struct{}
	release chan struct{}
}

// MarshalLogObject pauses this callback without holding the pipeline's SDK admission lock.
func (object fidelityBlockingObject) MarshalLogObject(encoder zapcore.ObjectEncoder) error {
	close(object.started)
	<-object.release
	encoder.AddString("value", "synthetic-callback-canary")
	return nil
}

// TestOTLPShutdownDoesNotWaitForApplicationEncoder bounds shutdown independently of producer callbacks.
func TestOTLPShutdownDoesNotWaitForApplicationEncoder(t *testing.T) {
	sink := &captureExporter{}
	isolateOTLP(t, newOTLPPipeline(sink, "b1", "test", 512, 128, time.Hour))
	object := fidelityBlockingObject{started: make(chan struct{}), release: make(chan struct{})}
	result := make(chan error, 1)
	go func() {
		result <- otlpHook(zapcore.Entry{Message: "controlled callback"}, []zapcore.Field{zap.Object("object", object)})
	}()
	<-object.started
	stopped := make(chan error, 1)
	go func() { stopped <- ShutdownOTLP(context.Background()) }()
	select {
	case err := <-stopped:
		require.NoError(t, err)
	case <-time.After(time.Second):
		close(object.release)
		t.Fatal("producer encoder blocked SDK shutdown")
	}
	close(object.release)
	require.ErrorContains(t, <-result, "pipeline stopped before record admission")
	require.EqualValues(t, 1, LogOTLPStats().Rejected)
	require.Empty(t, sink.snapshot())
}

// fidelityEmptyKeyObject supplies a nested key that the native AnyValue map supports.
type fidelityEmptyKeyObject struct{}

// MarshalLogObject retains an empty nested key without confusing it with a root attribute key.
func (fidelityEmptyKeyObject) MarshalLogObject(encoder zapcore.ObjectEncoder) error {
	encoder.AddString("", "synthetic-empty-key-canary")
	return nil
}

// TestOTLPNestedEmptyKeysRemainAvailable verifies SDK acceptance for supported nested map keys.
func TestOTLPNestedEmptyKeysRemainAvailable(t *testing.T) {
	sink := &captureExporter{}
	isolateOTLP(t, newOTLPPipeline(sink, "b1", "test", 512, 128, time.Hour))
	Logger.Info("nested empty keys", zap.Object("object", fidelityEmptyKeyObject{}), zap.Reflect("reflected", map[string]any{"": "synthetic-reflected-empty-key"}))
	require.NoError(t, ShutdownOTLP(context.Background()))
	records := sink.snapshot()
	require.Len(t, records, 1)
	attrs := recordAttributes(records[0])
	require.Equal(t, map[string]any{"": "synthetic-empty-key-canary"}, attrs["object"])
	require.Equal(t, map[string]any{"": "synthetic-reflected-empty-key"}, attrs["reflected"])
	require.Zero(t, LogOTLPStats().Rejected)
}
