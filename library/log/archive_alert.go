package log

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"time"

	"github.com/Laisky/errors/v2"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
)

// archiveLoggerScope is the archive worker's scope below the root logger.
const archiveLoggerScope = "arweave.localstorage"

// ArchiveLoggerName is the full logger name whose failures receive the bounded
// archive alert projection. Monitoring that filters by logger name should
// match this value or the "go-ramjet" prefix.
//
// The projection is a boundary of the outbound alert channel only. The same
// records keep their raw url and error fields in the ordinary log sinks: the
// local console and, when APP_LOG_OTLP_ENDPOINT opts in, the OTLP exporter, so
// operators can diagnose a failure from its target_ref. An OTLP destination
// must therefore be trusted with raw target URLs, query strings included.
const ArchiveLoggerName = rootLoggerName + "." + archiveLoggerScope

// ArchiveLogger returns the archive worker logger. It is derived on every call
// so it follows the production logger installed at startup, and it is the only
// supported way to obtain the reviewed archive alert scope.
func ArchiveLogger() glog.Logger { return Logger.Named(archiveLoggerScope) }

// FailureStageError is the explicit contract for errors that attribute a failure
// to a pipeline stage without exposing their text. Only the "admission" stage
// is exported by the archive alert projection.
type FailureStageError interface {
	error
	CrawlerFailureStage() string
}

// AdmissionReasonError is the explicit contract for typed URL admission refusals.
// The archive alert projection exports the reason only when it is in
// archiveAdmissionCategories; any other value becomes "admission_failed".
type AdmissionReasonError interface {
	error
	AdmissionReason() string
}

// archiveAdmissionCategories is the finite set of admission reasons an archive alert may carry.
var archiveAdmissionCategories = map[string]struct{}{
	"destination_denied": {}, "invalid_target": {}, "dns_not_found": {}, "dns_timeout": {}, "dns_failed": {},
}

// archiveTargetRefKey is a per-process random key. References correlate an
// alert with this process's local log line but cannot be recomputed from a URL
// or mapped to the SHA-256-derived S3 object key.
var archiveTargetRefKey = newArchiveTargetRefKey()

// newArchiveTargetRefKey returns 32 random bytes and panics when the system
// random source is unavailable, because no safe correlation key can exist then.
func newArchiveTargetRefKey() []byte {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		panic(errors.Wrap(err, "generate archive target reference key"))
	}
	return key
}

// ArchiveTargetRef returns a keyed, process-scoped correlation reference for target.
// The archive worker logs it locally next to the raw URL; alerts carry only the reference.
func ArchiveTargetRef(target string) string {
	mac := hmac.New(sha256.New, archiveTargetRefKey)
	if _, err := mac.Write([]byte(target)); err != nil {
		return ""
	}
	return hex.EncodeToString(mac.Sum(nil)[:16])
}

// alertFieldsFor selects the single reviewed alert policy for loggerName.
// faithful is true only for the reviewed verified-crawler scope, whose fields
// may be exported unchanged; every other scope receives the returned projection.
func alertFieldsFor(loggerName string, fields []zapcore.Field) (projected []zapcore.Field, faithful bool) {
	switch loggerName {
	case reviewedCrawlerLoggerName:
		return fields, true
	case ArchiveLoggerName:
		return archiveAlertFields(fields), false
	default:
		return nil, false
	}
}

// archiveAlertFields projects only bounded archive diagnostics; raw targets and errors stay local.
// The input must be call-site fields from the exact reviewed archive logger.
func archiveAlertFields(fields []zapcore.Field) []zapcore.Field {
	stage := "unknown"
	var target string
	var cause error
	var duration time.Duration = -1
	for _, field := range fields {
		switch field.Key {
		case "stage":
			if field.Type == zapcore.StringType && (field.String == "schedule" || field.String == "fetch" || field.String == "save") {
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
	stage, category := archiveFailureCategory(stage, cause)
	out := []zapcore.Field{zap.String("stage", stage), zap.String("category", category)}
	if target != "" {
		if ref := ArchiveTargetRef(target); ref != "" {
			out = append(out, zap.String("target_ref", ref))
		}
	}
	if duration >= 0 && duration <= 5*time.Minute {
		out = append(out, zap.Int64("duration_ms", duration.Milliseconds()))
	}
	return out
}

// archiveFailureCategory returns the finite stage and category for cause.
// Admission attribution wins over context identities so a resolver timeout is
// reported as dns_timeout rather than as the task budget expiring.
func archiveFailureCategory(stage string, cause error) (string, string) {
	var reasoned AdmissionReasonError
	var staged FailureStageError
	var network net.Error
	switch {
	case errors.As(cause, &reasoned):
		if _, ok := archiveAdmissionCategories[reasoned.AdmissionReason()]; ok {
			return "admission", reasoned.AdmissionReason()
		}
		return "admission", "admission_failed"
	case errors.As(cause, &staged) && staged.CrawlerFailureStage() == "admission":
		return "admission", contextCategory(cause, "admission_failed")
	case errors.Is(cause, context.Canceled), errors.Is(cause, context.DeadlineExceeded):
		return stage, contextCategory(cause, "")
	case errors.As(cause, &network):
		if network.Timeout() {
			return stage, "timeout"
		}
		return stage, "network_error"
	case stage == "schedule":
		return stage, "lookup_failed"
	case stage == "fetch":
		return stage, "fetch_failed"
	case stage == "save":
		return stage, "storage_failed"
	default:
		return stage, "operation_failed"
	}
}

// contextCategory returns "canceled" or "timeout" for an ended context, otherwise fallback.
func contextCategory(cause error, fallback string) string {
	switch {
	case errors.Is(cause, context.Canceled):
		return "canceled"
	case errors.Is(cause, context.DeadlineExceeded):
		return "timeout"
	default:
		return fallback
	}
}
