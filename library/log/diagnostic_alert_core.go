package log

import "github.com/Laisky/zap/zapcore"

// HTTPAlertMaxMessageBytes is the complete common-alert budget, including metadata and JSON expansion.
// Larger records reject whole; this does not change the ordinary local logging sink.
const HTTPAlertMaxMessageBytes = 1 << 20

const reviewedCrawlerLoggerName = "go-ramjet.run_dynamic_web_crawler.verified_crawler_task"

// CrawlerDiagnosticAlertCore preserves the application's existing exact logger scope.
// Pass a fresh unbound SDK core. The reviewed worker gets eagerly bound and call-site fields;
// other names retain metadata only through an independent empty-context clone.
func CrawlerDiagnosticAlertCore(core zapcore.Core) zapcore.Core {
	return &crawlerDiagnosticAlertCore{full: core, metadata: core.With(nil)}
}

// crawlerDiagnosticAlertCore selects exactly one alert write without wrapping a redundant hook.
type crawlerDiagnosticAlertCore struct{ full, metadata zapcore.Core }

// Enabled retains the shared SDK alert-level gate.
func (c *crawlerDiagnosticAlertCore) Enabled(level zapcore.Level) bool { return c.full.Enabled(level) }

// With snapshots reviewed context eagerly without binding it to the unrelated metadata route.
func (c *crawlerDiagnosticAlertCore) With(fields []zapcore.Field) zapcore.Core {
	return &crawlerDiagnosticAlertCore{full: c.full.With(fields), metadata: c.metadata}
}

// Check adds this routing core once when the SDK alert level admits the entry.
func (c *crawlerDiagnosticAlertCore) Check(entry zapcore.Entry, checked *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(entry.Level) {
		return checked.AddCore(entry, c)
	}
	return checked
}

// Write selects one existing application policy without changing the chosen entry message.
func (c *crawlerDiagnosticAlertCore) Write(entry zapcore.Entry, fields []zapcore.Field) error {
	if entry.LoggerName == reviewedCrawlerLoggerName {
		return c.full.Write(entry, fields)
	}
	return c.metadata.Write(entry, nil)
}

// Sync synchronizes the shared alert transport once; the SDK does not promise delivery flushing.
func (c *crawlerDiagnosticAlertCore) Sync() error { return c.full.Sync() }

// Fields returns the reviewed core's immutable context for the fork's ordinary core interface.
// No alert hook should be attached around this core or its enclosing tee.
func (c *crawlerDiagnosticAlertCore) Fields() []zapcore.Field { return c.full.Fields() }
