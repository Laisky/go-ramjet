package tasks

import (
	"context"
	"runtime/debug"
	"unicode/utf8"

	rlibs "github.com/Laisky/laisky-blog-graphql/library/db/redis"
	"github.com/Laisky/zap"

	"github.com/Laisky/go-ramjet/library/buildidentity"
)

const pinnedCrawlerConverterVersion = "v2.5.2"

// crawlerCaptureKey separates selected reproduction inputs from arbitrary request context.
type crawlerCaptureKey struct{}

// crawlerFailureCapture keeps immutable chosen inputs in memory until the existing failure log.
type crawlerFailureCapture struct {
	queuedURL         string
	outputMarkdown    bool
	renderedHTML      string
	renderedObserved  bool
	converterInput    string
	converterObserved bool
	renderer          *rendererFailureObservation
	source            buildidentity.Source
	sourceErr         error
}

// withCrawlerFailureCapture snapshots queued inputs before task mutation without I/O or persistence.
func withCrawlerFailureCapture(ctx context.Context, task *rlibs.HTMLCrawlerTask) (context.Context, *crawlerFailureCapture) {
	ctx, renderer := withRendererFailureObservation(ctx)
	source, sourceErr := buildidentity.ProcessSource("go-ramjet")
	capture := &crawlerFailureCapture{queuedURL: task.Url, outputMarkdown: task.OutputMarkdown, renderer: renderer, source: source, sourceErr: sourceErr}
	return context.WithValue(ctx, crawlerCaptureKey{}, capture), capture
}

// crawlerConverterVersion selects resolved public module provenance with a guarded test fallback.
func crawlerConverterVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, dep := range info.Deps {
			if dep.Path != "github.com/JohannesKaufmann/html-to-markdown/v2" {
				continue
			}
			if dep.Replace != nil {
				dep = dep.Replace
			}
			if dep.Version == "" {
				return "unavailable"
			}
			return dep.Version
		}
	}
	return pinnedCrawlerConverterVersion
}

// observeCrawlerCaptureRendered retains the exact browser-returned DOM string, not HTTP wire bytes.
func observeCrawlerCaptureRendered(ctx context.Context, content string) {
	capture, _ := ctx.Value(crawlerCaptureKey{}).(*crawlerFailureCapture)
	if capture != nil {
		capture.renderedHTML = content
		capture.renderedObserved = true
	}
}

// observeCrawlerCaptureConverter retains the identical immutable string passed to the converter.
func observeCrawlerCaptureConverter(ctx context.Context, content string) {
	capture, _ := ctx.Value(crawlerCaptureKey{}).(*crawlerFailureCapture)
	if capture != nil {
		capture.converterInput = content
		capture.converterObserved = true
	}
}

// crawlerChosenInputFields uses ordinary Zap values without truncation, filtering, or normalization.
// Invalid UTF-8 uses Zap's standard binary/base64 representation with an explicit encoding field.
func crawlerChosenInputFields(name, value string, observed bool) []zap.Field {
	fields := []zap.Field{zap.Bool(name+"_observed", observed)}
	if !observed {
		return fields
	}
	fields = append(fields, zap.Int(name+"_bytes", len(value)))
	if utf8.ValidString(value) {
		return append(fields, zap.String(name+"_encoding", "utf8"), zap.String(name, value))
	}
	return append(fields, zap.String(name+"_encoding", "base64"), zap.Binary(name, []byte(value)))
}

// fields returns selected original inputs and source identity for the existing failure log.
// It never serializes a task, API key, request headers, configuration, or external storage reference.
func (c *crawlerFailureCapture) fields() []zap.Field {
	fields := []zap.Field{
		zap.String("source_service", c.source.Service), zap.String("source_hostname", c.source.Hostname),
		zap.String("source_process_run_id", c.source.ProcessRunID), zap.String("source_revision", c.source.Revision),
		zap.Bool("source_modified", c.source.Modified), zap.String("source_go_version", c.source.GoVersion),
		zap.String("source_module_version", c.source.ModuleVersion), zap.Bool("output_markdown", c.outputMarkdown),
		zap.String("converter_version", crawlerConverterVersion()),
	}
	if c.sourceErr != nil {
		fields = append(fields, zap.NamedError("source_identity_error", c.sourceErr))
	}
	fields = append(fields, crawlerChosenInputFields("queued_target_url", c.queuedURL, true)...)
	fields = append(fields, crawlerChosenInputFields("rendered_html", c.renderedHTML, c.renderedObserved)...)
	fields = append(fields, crawlerChosenInputFields("conversion_input", c.converterInput, c.converterObserved)...)
	response := c.renderer.Snapshot()
	fields = append(fields, zap.Bool("renderer_response_observed", response != nil))
	if response != nil {
		fields = append(fields, response.fields()...)
	}
	return fields
}
