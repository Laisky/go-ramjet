package tasks

import (
	"context"
	"runtime/debug"
	"sync"

	"github.com/Laisky/errors/v2"
	rlibs "github.com/Laisky/laisky-blog-graphql/library/db/redis"
	"github.com/Laisky/zap"

	"github.com/Laisky/go-ramjet/library/crawlercapture"
)

const pinnedCrawlerConverterVersion = "v2.5.2"

// defaultCrawlerFailureJournal reads the selected private directory once per worker process.
var defaultCrawlerFailureJournal = sync.OnceValues(func() (*crawlercapture.Journal, error) {
	return crawlercapture.JournalFromEnvironment("go-ramjet")
})

// newCrawlerFailureJournal is a fixture seam; production uses the process-stable journal.
var newCrawlerFailureJournal = func() (*crawlercapture.Journal, error) { return defaultCrawlerFailureJournal() }

// crawlerCaptureKey separates reproduction inputs from arbitrary request context values.
type crawlerCaptureKey struct{}

// crawlerFailureCapture retains only explicit, immutable reproduction inputs for this task.
type crawlerFailureCapture struct {
	record    crawlercapture.Record
	renderer  *rendererFailureObservation
	once      sync.Once
	reference crawlercapture.Reference
	err       error
}

// withCrawlerFailureCapture snapshots the admitted queue inputs before any task mutation.
func withCrawlerFailureCapture(ctx context.Context, task *rlibs.HTMLCrawlerTask) (context.Context, *crawlerFailureCapture) {
	ctx, renderer := withRendererFailureObservation(ctx)
	capture := &crawlerFailureCapture{renderer: renderer, record: crawlercapture.Record{
		Correlation:  crawlercapture.Correlation{TaskID: task.TaskID},
		Request:      crawlercapture.Request{OriginalURL: crawlercapture.CapturedString(task.Url), SubmittedURL: crawlercapture.CapturedString(task.Url), OutputMarkdown: task.OutputMarkdown},
		Converter:    crawlercapture.Converter{Name: "html-to-markdown/v2", Version: crawlerConverterVersion(), Input: crawlercapture.NotReached()},
		RenderedHTML: crawlercapture.NotReached(),
	}}
	return context.WithValue(ctx, crawlerCaptureKey{}, capture), capture
}

// crawlerConverterVersion selects resolved public module provenance, with a guarded source pin for Go tests.
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

// observeCrawlerCaptureRendered retains the exact browser-returned document, not HTTP bytes.
func observeCrawlerCaptureRendered(ctx context.Context, content string) {
	capture, _ := ctx.Value(crawlerCaptureKey{}).(*crawlerFailureCapture)
	if capture != nil {
		capture.record.RenderedHTML = crawlercapture.CapturedString(content)
	}
}

// observeCrawlerCaptureConverter retains the same immutable argument used for local conversion.
func observeCrawlerCaptureConverter(ctx context.Context, content string) {
	capture, _ := ctx.Value(crawlerCaptureKey{}).(*crawlerFailureCapture)
	if capture != nil {
		capture.record.Converter.Input = crawlercapture.CapturedString(content)
	}
}

// fields persists once before notification, even under cancellation, and returns compact lookup metadata.
func (c *crawlerFailureCapture) fields(stage, code string) []zap.Field {
	c.once.Do(func() {
		c.record.Failure = crawlercapture.Failure{Stage: stage, Code: code}
		c.record.Renderer = c.renderer.Snapshot()
		journal, err := newCrawlerFailureJournal()
		if err != nil {
			source, sourceErr := crawlercapture.ProcessSource("go-ramjet")
			c.reference = crawlercapture.Reference{Status: "write_failed", Source: source}
			c.err = errors.Join(err, sourceErr)
			return
		}
		c.reference, c.err = journal.Write(c.record)
	})
	reference := c.reference
	fields := []zap.Field{
		zap.String("failure_capture_service", reference.Source.Service),
		zap.String("failure_capture_hostname", reference.Source.Hostname),
		zap.String("failure_capture_process_run_id", reference.Source.ProcessRunID),
		zap.String("failure_capture_status", reference.Status),
		zap.String("failure_capture_id", reference.CaptureID),
		zap.String("failure_capture_sha256", reference.SHA256),
		zap.Int64("failure_capture_bytes", reference.Bytes),
	}
	if c.err != nil {
		fields = append(fields, zap.NamedError("failure_capture_error", c.err))
	}
	return fields
}

// pathField is appended after all compact correlation fields in a notification.
func (c *crawlerFailureCapture) pathField() zap.Field {
	return zap.String("failure_capture_path", c.reference.Path)
}
