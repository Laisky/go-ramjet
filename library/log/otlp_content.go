package log

import (
	"math"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap/zapcore"
	"go.opentelemetry.io/otel/attribute"
)

const (
	maxOTLPBodyBytes   = 4096
	maxOTLPStringBytes = 2048
	maxOTLPAttributes  = 24
	maxOTLPRecordBytes = 16 * 1024
	maxOTLPInputFields = 256
)

// projectOTLPContent preserves ordinary messages and primitive fields with targeted redaction.
// It never invokes arbitrary marshalers, Stringer callbacks, or reflection.
func projectOTLPContent(entry zapcore.Entry, fields []zapcore.Field, secret string) (string, []attribute.KeyValue) {
	body, truncated := boundedOTLPText(entry.Message, maxOTLPBodyBytes, secret)
	attrs := make([]attribute.KeyValue, 0, maxOTLPAttributes)
	seen := make(map[string]struct{}, maxOTLPAttributes)
	budget := maxOTLPRecordBytes - len(body) - 128
	dropped := 0
	add := func(key string, value attribute.Value) bool {
		if _, exists := seen[key]; exists {
			dropped++
			return false
		}
		size := len(key) + 16
		if value.Type() == attribute.STRING {
			size += len(value.AsString())
		}
		if len(attrs) >= maxOTLPAttributes-2 || size > budget {
			dropped++
			return false
		}
		attrs = append(attrs, attribute.KeyValue{Key: attribute.Key(key), Value: value})
		seen[key] = struct{}{}
		budget -= size
		return true
	}
	if entry.LoggerName != "" {
		value, cut := boundedOTLPText(entry.LoggerName, maxOTLPStringBytes, secret)
		truncated = truncated || cut
		add("logger.name", attribute.StringValue(value))
	}
	if entry.Caller.Defined {
		value, cut := boundedOTLPText(entry.Caller.File, maxOTLPStringBytes, secret)
		truncated = truncated || cut
		add("code.file.path", attribute.StringValue(value))
		add("code.line.number", attribute.IntValue(entry.Caller.Line))
	}
	for index, field := range fields {
		if index >= maxOTLPInputFields || len(attrs) >= maxOTLPAttributes-2 {
			dropped += len(fields) - index
			break
		}
		if field.Type == zapcore.NamespaceType {
			dropped += len(fields) - index
			break
		}
		if field.Type == zapcore.SkipType {
			continue
		}
		if !validOTLPKey(field.Key) || sensitiveOTLPKey(field.Key) || glog.SanitizeDiagnosticText(field.Key) != field.Key || (secret != "" && strings.Contains(field.Key, secret)) {
			dropped++
			continue
		}
		if reservedOTLPKey(field.Key) {
			dropped++
			continue
		}
		if _, exists := seen[field.Key]; exists {
			dropped++
			continue
		}
		value, ok, cut := primitiveOTLPValue(field, secret)
		truncated = truncated || cut
		if !ok {
			dropped++
			continue
		}
		add(field.Key, value)
	}
	if dropped > 0 {
		attrs = append(attrs, attribute.Int("app.log.attributes_dropped", dropped))
	}
	if truncated {
		attrs = append(attrs, attribute.Bool("app.log.content_truncated", true))
	}
	return body, attrs
}

// primitiveOTLPValue converts supported inline values without executing custom encoders.
func primitiveOTLPValue(field zapcore.Field, secret string) (attribute.Value, bool, bool) {
	switch field.Type {
	case zapcore.StringType:
		value, cut := boundedOTLPText(field.String, maxOTLPStringBytes, secret)
		return attribute.StringValue(value), true, cut
	case zapcore.BoolType:
		return attribute.BoolValue(field.Integer != 0), true, false
	case zapcore.Int8Type, zapcore.Int16Type, zapcore.Int32Type, zapcore.Int64Type:
		return attribute.Int64Value(field.Integer), true, false
	case zapcore.Uint8Type, zapcore.Uint16Type, zapcore.Uint32Type, zapcore.Uint64Type:
		value := uint64(field.Integer)
		if value > math.MaxInt64 {
			return attribute.StringValue(strconv.FormatUint(value, 10)), true, false
		}
		return attribute.Int64Value(int64(value)), true, false
	case zapcore.Float64Type:
		value := math.Float64frombits(uint64(field.Integer))
		return finiteOTLPFloat(value)
	case zapcore.Float32Type:
		value := float64(math.Float32frombits(uint32(field.Integer)))
		return finiteOTLPFloat(value)
	case zapcore.DurationType:
		return attribute.Float64Value(float64(field.Integer) / float64(time.Second)), true, false
	case zapcore.TimeType:
		return attribute.StringValue(time.Unix(0, field.Integer).UTC().Format(time.RFC3339Nano)), true, false
	case zapcore.ErrorType:
		cause, ok := field.Interface.(error)
		if !ok || cause == nil {
			return attribute.Value{}, false, false
		}
		value, cut := boundedOTLPText(otlpErrorText(cause), maxOTLPStringBytes, secret)
		return attribute.StringValue(value), true, cut
	default:
		return attribute.Value{}, false, false
	}
}

// finiteOTLPFloat rejects nonfinite values instead of emitting ambiguous numeric diagnostics.
func finiteOTLPFloat(value float64) (attribute.Value, bool, bool) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return attribute.Value{}, false, false
	}
	return attribute.Float64Value(value), true, false
}

// boundedOTLPText reuses the existing diagnostic privacy contract before applying byte limits.
// Unlabeled private prose cannot be recognized; callers must keep request content out of messages.
func boundedOTLPText(text string, limit int, secret string) (string, bool) {
	cut := len(text) > limit
	text = glog.SanitizeDiagnosticText(text)
	if secret != "" {
		text = strings.ReplaceAll(text, secret, "[redacted]")
	}
	if len(text) > limit {
		text = text[:limit]
		for !utf8.ValidString(text) {
			text = text[:len(text)-1]
		}
		cut = true
	}
	return text, cut
}

// otlpErrorText permits ordinary error diagnostics while preventing formatter panics from escaping.
func otlpErrorText(cause error) (text string) {
	defer func() {
		if recover() != nil {
			text = "[error text unavailable]"
		}
	}()
	return cause.Error()
}

// reservedOTLPKey protects resource identity and bridge-owned diagnostic metadata from source fields.
func reservedOTLPKey(key string) bool {
	switch key {
	case "_msg", "_time", "_stream", "service.namespace", "service.name", "host.name", "deployment.environment.name", "logger.name", "code.file.path", "code.line.number", "app.log.attributes_dropped", "app.log.content_truncated":
		return true
	default:
		return false
	}
}

// validOTLPKey rejects invalid UTF-8 and control characters before key matching or export.
func validOTLPKey(key string) bool {
	return len(key) > 0 && len(key) <= 128 && utf8.ValidString(key) && strings.IndexFunc(key, unicode.IsControl) < 0
}
