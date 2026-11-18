package log

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
)

const reviewedArchiveLoggerName = "go-ramjet.arweave.localstorage"

// archiveAlertFields projects only bounded archive diagnostics; raw targets and errors stay local.
// The input must be call-site fields from the exact reviewed archive logger.
func archiveAlertFields(fields []zapcore.Field) []zapcore.Field {
	stage, category := "unknown", "operation_failed"
	var target string
	var cause error
	var duration time.Duration = -1
	for _, field := range fields {
		switch field.Key {
		case "stage":
			if field.Type == zapcore.StringType && (field.String == "fetch" || field.String == "save") {
				stage = field.String
			}
		case "url":
			if field.Type == zapcore.StringType {
				target = field.String
			}
		case "error":
			if field.Type == zapcore.ErrorType {
				cause, _ = field.Interface.(error)
			}
		case "duration":
			if field.Type == zapcore.DurationType {
				duration = time.Duration(field.Integer)
			}
		}
	}
	var admission interface{ CrawlerFailureStage() string }
	var network net.Error
	switch {
	case errors.Is(cause, context.Canceled):
		category = "canceled"
	case errors.Is(cause, context.DeadlineExceeded):
		category = "timeout"
	case errors.As(cause, &admission) && admission.CrawlerFailureStage() == "admission":
		stage, category = "admission", "admission_failed"
	case errors.As(cause, &network):
		category = "network_error"
		if network.Timeout() {
			category = "timeout"
		}
	case stage == "fetch":
		category = "fetch_failed"
	case stage == "save":
		category = "storage_failed"
	}
	out := []zapcore.Field{zap.String("stage", stage), zap.String("category", category)}
	if target != "" {
		sum := sha256.Sum256([]byte(target))
		out = append(out, zap.String("target_sha256", hex.EncodeToString(sum[:])))
	}
	if duration >= 0 && duration <= 5*time.Minute {
		out = append(out, zap.Int64("duration_ms", duration.Milliseconds()))
	}
	return out
}
