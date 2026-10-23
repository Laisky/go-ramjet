# Optional private OTLP application logs

Stdout remains the local log source. No Docker log ingestion should be enabled for
these same records alongside OTLP. Existing alert delivery is independent.

Set APP_LOG_OTLP_ENDPOINT to an HTTPS base URL (no path/query/userinfo) to opt in.
APP_LOG_OTLP_TOKEN_FILE names a mounted plaintext bearer-token file; APP_LOG_OTLP_CA_FILE
names the trusted PEM CA file. The bearer-token file is private runtime material and must never enter the repository.
The CA file is public trust material; its authenticity must still be preserved. APP_LOG_OTLP_HOST and APP_LOG_OTLP_ENV must explicitly label the controlled host
and deployment environment. service.name is fixed to go-ramjet and service.namespace to laisky.

The OTLP branch preserves the original application message and supplied fields by
default. It performs no automatic sanitization, token replacement, key filtering,
omission or truncation. Existing application-level logging and alert policies remain
the application's responsibility; this bridge adds no optional redaction setting.

Fields use the fork's native typed Zap encoder contract. Durations are signed
nanoseconds; timestamps use RFC3339Nano, retaining precision and the selected offset.
Binary values remain native OTLP bytes. Valid Unicode strings remain strings; invalid
Unicode message/field strings retain their original bytes as BytesValue. Floats retain
native precision, sign and nonfinite values. Complex values use their standard Go
string representation. Object, Array, Stringer and Error invoke their ordinary
application encoders; Reflect alone follows application JSON tags/custom marshaling
before recursive native mapping. Nulls, empty containers and unknown names survive.

The bridge follows [OpenTelemetry's arbitrary-value mapping](https://opentelemetry.io/docs/specs/otel/common/attribute-type-mapping/):
integers outside signed int64 and reflected JSON decimals beyond double range or
shortest-roundtrip precision become exact decimal strings. A multimap lists every key
once, with an ordered array of all values for each key; singleton keys in that object
also become one-element arrays. An array supplied as one source value stays one array
element. No duplicate-key override or SDK-specific duplicate extension is required.

Bound fields are snapshotted once per opted-in output branch at With, preserving
namespace and binding order. The normal stdout branch still owns its own encoding,
as with any additional logger sink; callbacks should be deterministic. Bound callbacks
are not rerun for each event. Disabled logger creation returns the original Core and
adds no callbacks. APP_LOG_OTLP_ENDPOINT must be set before logger initialization;
changing the environment later does not retrofit earlier disabled logger branches.
The base Core's acceptance, sampling, alerts and writes remain authoritative.

SDK attribute count, string-length and nesting limits are explicitly disabled so SDK
defaults or environment settings cannot silently rewrite content. Empty root attribute keys or non-Unicode map keys, malformed reflected Unicode such as a lone surrogate, or private
encoder failures reject the whole remote record with a fixed hook diagnostic and an
observable Rejected count. Normal application JSON marshal errors retain Zap's normal
error-field behavior. User encoders run before SDK admission, so shutdown does not wait
for a stalled callback; a callback completing after admission closes is explicitly
rejected and counted. Local stdout retains its existing behavior. Receiver limits can
still reject records; exporter-return counters and final queries distinguish outcomes.

Record attributes and resource attributes remain separate native OTLP scopes even when
names coincide; application fields take precedence over optional bridge-added
logger/source metadata. The sink's indexing and native OTLP mapping may not retain all
of these distinctions. The isolated pinned VictoriaLogs mapping crashed on colliding
resource/record stream-label names, so this source must use a separately qualified
lossless downstream representation before deployment. Transport success alone is not
a lossless-storage promise. No application-side mirror or content filter is added. The experimental downstream
JSON-array representation retains the ordinary JSON-compatible subset, but collapses
some integer/double and bytes/string distinctions; nonfinite values also produced
invalid JSON in the pinned sink. It is not qualified as a general lossless backend.

One private SDK provider batches at most 512 queued records, 128 per batch, with a
one-second interval and five-second export timeout. The SDK overwrites oldest queued
records under pressure; its process-local buffer is not a durable spool. Collector
WAL durability begins only after collector acceptance. Retry outcomes can be ambiguous
and duplicate delivery remains possible. Hard process failure can lose buffered logs.

LogOTLPStats reports emitted, successfully returned exporter records, failed exporter
records, whole records rejected before SDK admission, and unresolved records. Unresolved includes queued/in-flight work and possible
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

The SDK preserves normal custom OTEL_RESOURCE_ATTRIBUTES. Setup validates resource
syntax before SDK parsing and reports malformed configuration explicitly; it does not
filter keys by content. Explicit APP identity labels override environment identity
values through normal resource merge precedence. Configuration errors are fixed
messages, and the environment is never rewritten.

Deployment must give Docker at least a 30-second stop_grace_period, covering the
10-second HTTP drain plus 10-second SDK shutdown and scheduling margin. The coordinated
VPS deployment change (PR #5) supplies this setting for both applications; Docker's
10-second default can terminate the process before the final log flush finishes.
