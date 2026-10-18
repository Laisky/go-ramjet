# Optional private OTLP application logs

Stdout remains the local log source. No Docker log ingestion should be enabled for
these same records alongside OTLP. Existing alert delivery is independent.

Set APP_LOG_OTLP_ENDPOINT to an HTTPS base URL (no path/query/userinfo) to opt in.
APP_LOG_OTLP_TOKEN_FILE names a mounted plaintext bearer-token file; APP_LOG_OTLP_CA_FILE
names the trusted PEM CA file. The bearer-token file is private runtime material and must never enter the repository.
The CA file is public trust material; its authenticity must still be preserved. APP_LOG_OTLP_HOST and APP_LOG_OTLP_ENV must explicitly label the controlled host
and deployment environment. service.name is fixed to go-ramjet and service.namespace to laisky.

The OTLP branch preserves ordinary operational messages, sanitized logger identity,
source locations and generic primitive fields, including strings, integers, finite
floats, booleans, durations (seconds), timestamps and sanitized error text. There is
no event/message/diagnostic-field allowlist. It reuses go-utils SanitizeDiagnosticText
for recognizable credentials, authentication values, URLs and labeled raw payloads,
and also redacts the configured OTLP bearer token if echoed.

Credential/raw-request/response-body keys are excluded using the existing normalized
key deny contract plus actual MCP/search payload labels. Names such as token_count,
query_latency and password_reset_required remain ordinary diagnostics. Unlabeled
private prose cannot be recognized reliably: callers must not put user content into
messages. Arbitrary object/array/reflection/Stringer marshalers and raw bytes are not
executed or exported; summarize them with ordinary fields. Namespace fields stop
projection rather than flattening private nested context.

Projection bounds are 4 KiB body, 2 KiB strings, 128-byte keys, 24 attributes (including
reserved flags), 16 KiB approximate content budget and at most 256 examined source
fields. Duplicate keys do not grow output. app.log.attributes_dropped and
app.log.content_truncated make exclusions and bounds visible. Fixed resource identity,
logger/source metadata and these flags cannot be overwritten by source fields.
Stdout keeps the original message and fields unchanged.

One private SDK provider batches at most 512 queued records, 128 per batch, with a
one-second interval and five-second export timeout. The SDK overwrites oldest queued
records under pressure; its process-local buffer is not a durable spool. Collector
WAL durability begins only after collector acceptance. Retry outcomes can be ambiguous
and duplicate delivery remains possible. Hard process failure can lose buffered logs.

LogOTLPStats reports emitted, successfully returned exporter records, failed exporter
records and unresolved records. Unresolved includes queued/in-flight work and possible
SDK overflow; it is not exact queue size or a live drop count. After stopped producers
and a successful shutdown, remaining unresolved records were not exported. Exporter
success alone does not prove VictoriaLogs storage: validate the actual query separately.
No global OTel providers or error handlers are replaced by production setup.
The private HTTP client ignores proxy environment settings and refuses redirects,
so its bearer token is only sent to the configured trusted HTTPS collector.
Returned exporter errors are replaced with a constant diagnostic. This is not a
blanket promise about SDK-internal diagnostics: exact pinned exporter behavior is
covered separately, including receiver partial-success responses. Exported counts
mean the SDK exporter returned nil, not that every receiver item was accepted.

SIGINT/SIGTERM stop HTTP admission, allow up to ten seconds of HTTP draining, then
flush the log provider with a fresh ten-second context. Background tasks receive the
canceled process context where already supported; this change does not retrofit every
independent cron task with cancellation. Shutdown counters remain available locally.

ConfigureOTLP returns an idempotent shutdown callback. Reconfiguration after a provider
has been installed/shut down requires process restart. Missing endpoint reads no files
and preserves disabled behavior.

Focused tests: go test -race ./library/log -run TestOTLP
External storage qualification is separately gated by OTLP_LOG_ACCEPTANCE=1 and
TestOTLPExternalAcceptance. It emits one synthetic operational event using production
HTTPS configuration; run only against an authorized test collector and query the final
storage for service.name, configured environment/host, status_code=299 and success=true.

The pinned OTLP/HTTP v0.23.0 exporter reports a protobuf partial-success rejection
as an exporter error; the terminal-error wrapper sanitizes its receiver diagnostic.
Failed counts records in failed export batches, not an exact receiver-rejected item
count. Exported still means exporter return success rather than verified final storage.
No process-global handler is installed to rewrite SDK-internal configuration diagnostics.

The SDK merges resource environment attributes even with an explicit resource. Setup
therefore validates bounded resource-environment syntax before SDK detector diagnostics
and rejects any merged attributes beyond the application-owned identity. Environment
identity values cannot replace fixed application labels. Configuration errors are fixed
messages, and the environment is never rewritten.

Deployment must give Docker at least a 30-second stop_grace_period, covering the
10-second HTTP drain plus 10-second SDK shutdown and scheduling margin. The coordinated
VPS deployment change (PR #5) supplies this setting for both applications; Docker's
10-second default can terminate the process before the final log flush finishes.
