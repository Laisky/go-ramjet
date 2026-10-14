package log

import (
	"time"

	"github.com/Laisky/zap"
	"github.com/google/uuid"
)

const maxCrawlerDiagnosticDuration = 5 * time.Minute

// CrawlerFailurePair validates an exact reviewed stage/category pair without parsing error text.
func CrawlerFailurePair(stage, category string) bool {
	switch stage {
	case "admission", "render", "verification", "conversion":
		switch category {
		case "worker_failed", "canceled", "timeout":
			return true
		}
	}
	switch stage {
	case "admission":
		switch category {
		case "policy_missing", "policy_invalid", "unsupported_proxy", "task_missing":
			return true
		}
	case "render":
		switch category {
		case "render_failed", "challenge", "empty_body", "result_too_large":
			return true
		}
	case "verification":
		return category == "evidence_rejected"
	case "conversion":
		switch category {
		case "conversion_failed", "empty_body", "empty_markdown", "result_too_large":
			return true
		}
	}
	return false
}

// CrawlerFailureFields returns reviewed scalar fields for a worker failure.
// Noncanonical task IDs and durations outside zero through five minutes are omitted.
func CrawlerFailureFields(taskID, stage, category string, duration time.Duration) []zap.Field {
	if !CrawlerFailurePair(stage, category) {
		return nil
	}
	fields := []zap.Field{zap.String("error_code", category), zap.String("stage", stage),
		zap.String("category", category)}
	if duration >= 0 && duration <= maxCrawlerDiagnosticDuration {
		fields = append(fields, zap.Int64("duration_ms", duration.Milliseconds()))
	}
	if parsed, err := uuid.Parse(taskID); err != nil {
		return fields
	} else if parsed != uuid.Nil && len(taskID) == 36 && parsed.String() == taskID {
		fields = append(fields, zap.String("task_id", taskID))
	}
	return fields
}
