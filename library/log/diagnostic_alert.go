package log

import "github.com/Laisky/zap/zapcore"

// CrawlerDiagnosticAlertHook exposes default fields from the reviewed worker
// logger only. Legacy crawler and unrelated services can log raw user/provider
// payloads and retain metadata-only alert export. No field-name allowlist is used.
func CrawlerDiagnosticAlertHook(hook func(zapcore.Entry, []zapcore.Field) error) func(zapcore.Entry, []zapcore.Field) error {
	return func(entry zapcore.Entry, fields []zapcore.Field) error {
		if entry.LoggerName == "go-ramjet.run_dynamic_web_crawler.verified_crawler_task" {
			return hook(entry, fields)
		}
		return hook(entry, nil)
	}
}
