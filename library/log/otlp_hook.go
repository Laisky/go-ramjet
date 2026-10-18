package log

import (
	"context"

	"github.com/Laisky/zap/zapcore"
	"go.opentelemetry.io/otel/attribute"
	otellog "go.opentelemetry.io/otel/log"
)

// otlpHook emits bounded sanitized operational diagnostics through the installed provider.
// The original console logger owns level selection, so no second tee duplicates records.
func otlpHook(entry zapcore.Entry, fields []zapcore.Field) error {
	p := otlpState.current.Load()
	if p == nil {
		return nil
	}
	p.admission.RLock()
	defer p.admission.RUnlock()
	if p.stopped {
		return nil
	}
	var record otellog.Record
	record.SetTimestamp(entry.Time)
	body, attrs := projectOTLPContent(entry, fields, p.redactionSecret)
	record.SetBody(attribute.StringValue(body))
	record.SetSeverity(otlpSeverity(entry.Level))
	record.SetSeverityText(entry.Level.String())
	record.AddAttributes(attrs...)

	p.emitted.Add(1)
	p.logger.Emit(context.Background(), record)
	return nil
}

// otlpSeverity maps the fork's severity values onto standard OpenTelemetry levels.
func otlpSeverity(level zapcore.Level) otellog.Severity {
	switch level {
	case zapcore.DebugLevel:
		return otellog.SeverityDebug
	case zapcore.InfoLevel:
		return otellog.SeverityInfo
	case zapcore.WarnLevel:
		return otellog.SeverityWarn
	case zapcore.ErrorLevel:
		return otellog.SeverityError
	case zapcore.DPanicLevel:
		return otellog.SeverityFatal1
	case zapcore.PanicLevel:
		return otellog.SeverityFatal2
	case zapcore.FatalLevel:
		return otellog.SeverityFatal3
	default:
		return otellog.SeverityUndefined
	}
}
