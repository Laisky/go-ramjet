package tasks

import (
	"context"
	"encoding/json"

	"github.com/Laisky/errors/v2"
	rlibs "github.com/Laisky/laisky-blog-graphql/library/db/redis"

	"github.com/Laisky/go-ramjet/library/log"
)

// crawlerTaskError preserves typed causes and renders useful sanitized diagnostic text.
type crawlerTaskError struct {
	code, stage string
	cause       error
}

// Error retains meaningful sanitized cause text while excluding credentials and raw payloads.
func (e *crawlerTaskError) Error() string {
	message := "crawler " + e.stage + ": " + e.code
	if detail := crawlerCauseText(e.cause); detail != "" {
		message += ": " + detail
	}
	return boundCrawlerDiagnosticText(message)
}

// Unwrap retains the underlying typed cause for internal errors.Is and errors.As checks.
func (e *crawlerTaskError) Unwrap() error { return e.cause }

// crawlerLifecycleOnly reports whether every concrete leaf describes only a lifecycle failure.
func crawlerLifecycleOnly(err error) bool {
	if err != nil {
		if joined, ok := err.(interface{ Unwrap() []error }); ok {
			found := false
			for _, cause := range joined.Unwrap() {
				if cause != nil {
					found = true
					if !crawlerLifecycleOnly(cause) {
						return false
					}
				}
			}
			return found
		}
		if wrapped, ok := err.(interface{ Unwrap() error }); ok {
			if cause := wrapped.Unwrap(); cause != nil {
				return crawlerLifecycleOnly(cause)
			}
		}
		return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
	}
	return false
}

// crawlerFailure classifies reviewed causes and preserves ordinary errors for sanitized diagnostics.
func crawlerFailure(code, stage string, err error) error {
	if crawlerLifecycleOnly(err) {
		if errors.Is(err, context.Canceled) {
			code = "canceled"
		} else if errors.Is(err, context.DeadlineExceeded) {
			code = "timeout"
		}
	}
	if !log.CrawlerFailurePair(stage, code) && !(stage == "verification" && code == "result_invalid") {
		code, stage = "worker_failed", "worker"
	}
	return errors.WithStack(&crawlerTaskError{code: code, stage: stage, cause: err})
}

// crawlerDiagnostic returns only reviewed code and stage without reading error text.
func crawlerDiagnostic(err error) (string, string) {
	var failure *crawlerTaskError
	if errors.As(err, &failure) {
		return failure.code, failure.stage
	}
	return "worker_failed", "worker"
}

type crawlerFailureKey struct{}
type crawlerFailureMetadata struct{ Stage, Category string }

// withCrawlerFailure carries only the reviewed pair to the terminal publication encoder.
func withCrawlerFailure(ctx context.Context, err error) context.Context {
	category, stage := crawlerDiagnostic(err)
	return context.WithValue(ctx, crawlerFailureKey{}, crawlerFailureMetadata{Stage: stage, Category: category})
}

// encodeHTMLCrawlerTaskResult extends the pinned DTO with optional reviewed failure metadata only.
func encodeHTMLCrawlerTaskResult(ctx context.Context, task *rlibs.HTMLCrawlerTask) (string, error) {
	payload, err := task.ToString()
	if err != nil {
		return "", errors.Wrap(err, "serialize crawler task")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(payload), &fields); err != nil {
		return "", errors.Wrap(err, "decode crawler envelope")
	}
	delete(fields, "failure_stage")
	delete(fields, "failure_category")
	metadata, ok := ctx.Value(crawlerFailureKey{}).(crawlerFailureMetadata)
	if task.Status == rlibs.TaskStatusFailed && ok && log.CrawlerFailurePair(metadata.Stage, metadata.Category) {
		stage, err := json.Marshal(metadata.Stage)
		if err != nil {
			return "", errors.Wrap(err, "encode crawler stage")
		}
		category, err := json.Marshal(metadata.Category)
		if err != nil {
			return "", errors.Wrap(err, "encode crawler category")
		}
		fields["failure_stage"], fields["failure_category"] = stage, category
	}
	encoded, err := json.Marshal(fields)
	if err != nil {
		return "", errors.Wrap(err, "encode crawler envelope")
	}
	return string(encoded), nil
}

// combineRendererFailure preserves the primary failure instead of cancellation induced by renderer shutdown.
func combineRendererFailure(ctx context.Context, primary, capture error) error {
	if primary != nil {
		if err := ctx.Err(); err != nil {
			return errors.Join(primary, err)
		}
		return primary
	}
	return capture
}
