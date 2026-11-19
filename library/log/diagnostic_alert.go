package log

import "github.com/Laisky/zap/zapcore"

// CrawlerDiagnosticAlertHook exposes default fields from the reviewed worker
// logger only. Legacy crawler and unrelated services can log raw user/provider
// payloads and retain metadata-only alert export. Archive failures receive the
// same bounded safe projection as the core; verified worker fields remain faithful.
func CrawlerDiagnosticAlertHook(hook func(zapcore.Entry, []zapcore.Field) error) func(zapcore.Entry, []zapcore.Field) error {
	return func(entry zapcore.Entry, fields []zapcore.Field) error {
		projected, _ := alertFieldsFor(entry.LoggerName, fields)
		return hook(entry, projected)
	}
}
