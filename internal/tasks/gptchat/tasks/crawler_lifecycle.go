package tasks

import (
	"context"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
)

// crawlerLoggedError preserves an emitted error's identity while preventing duplicate outer-boundary logging.
type crawlerLoggedError struct{ cause error }

// Error returns the already-sanitized failure text for an emitted crawler error.
func (e *crawlerLoggedError) Error() string {
	return boundCrawlerDiagnosticText(crawlerCauseText(e.cause))
}

// Unwrap preserves errors.Is and errors.As for an emitted crawler error.
func (e *crawlerLoggedError) Unwrap() error { return e.cause }

// crawlerErrorLogged reports whether the reviewed worker boundary already emitted this error.
func crawlerErrorLogged(err error) bool {
	var logged *crawlerLoggedError
	return errors.As(err, &logged)
}

// crawlerLifecycleFields returns finite lifecycle metadata without inspecting task contents or caller identity.
func crawlerLifecycleFields(ctx context.Context, taskID, event, status, outcome string, started time.Time) []zap.Field {
	switch event {
	case "processing_completed", "result_published", "result_publication_failed", "audit_persistence_failed":
	default:
		return nil
	}
	fields := []zap.Field{zap.String("event", event)}
	switch status {
	case "running", "failed", "success":
		fields = append(fields, zap.String("result_status", status))
	}
	switch outcome {
	case "success", "failed":
		fields = append(fields, zap.String("publication_outcome", outcome))
	}
	state := "active"
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		state = "deadline_exceeded"
	} else if errors.Is(ctx.Err(), context.Canceled) {
		state = "canceled"
	}
	fields = append(fields, zap.String("worker_context_state", state))
	if id := crawlerLogTaskID(taskID); id != "invalid" {
		fields = append(fields, zap.String("task_id", id))
	}
	if elapsed := time.Since(started); elapsed >= 0 && elapsed <= 5*time.Minute {
		fields = append(fields, zap.Int64("duration_ms", elapsed.Milliseconds()))
	}
	return fields
}

// crawlerHTTPStatusError preserves a document response status when a later exchange operation fails.
type crawlerHTTPStatusError struct {
	status int
	cause  error
}

// Error returns bounded sanitized cause text without the response body or target.
func (e *crawlerHTTPStatusError) Error() string {
	return boundCrawlerDiagnosticText(crawlerCauseText(e.cause))
}

// Unwrap preserves the concrete exchange error for internal classification.
func (e *crawlerHTTPStatusError) Unwrap() error { return e.cause }

// UpstreamStatusCode returns the observed document HTTP status, without asserting it caused the failure.
func (e *crawlerHTTPStatusError) UpstreamStatusCode() int { return e.status }

// crawlerUpstreamStatusFields exports only status evidence captured by the document exchange boundary.
func crawlerUpstreamStatusFields(err error) []zap.Field {
	var observed *crawlerHTTPStatusError
	if errors.As(err, &observed) && observed.status >= 100 && observed.status <= 599 {
		return []zap.Field{zap.Int("upstream_status", observed.status)}
	}
	return nil
}
