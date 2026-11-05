package log

import (
	"context"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/zap/zapcore"
	otellog "go.opentelemetry.io/otel/log"
)

// otlpHook emits application log records faithfully through the installed provider.
// The original console logger owns level selection, so no second tee duplicates records.
func otlpHook(entry zapcore.Entry, fields []zapcore.Field) error {
	return otlpHookWithContext(entry, fields, nil)
}

// otlpHookWithContext emits a callsite entry with its immutable branch context.
func otlpHookWithContext(entry zapcore.Entry, fields []zapcore.Field, inherited *otlpObjectEncoder) error {
	p := otlpState.current.Load()
	if p == nil {
		return nil
	}
	attrs, err := projectOTLPContext(entry, fields, inherited)
	if err != nil {
		p.rejected.Add(1)
		return err
	}
	// User encoders run outside admission so they cannot hold SDK shutdown open.
	p.admission.RLock()
	defer p.admission.RUnlock()
	if p.stopped {
		p.rejected.Add(1)
		return errors.New("OTLP log pipeline stopped before record admission")
	}
	var record otellog.Record
	record.SetTimestamp(entry.Time)
	record.SetBody(otlpStringValue(entry.Message))
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
