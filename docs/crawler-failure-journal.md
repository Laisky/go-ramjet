# Private crawler failure logs and offline reproduction

The worker writes an application-native private failure log before emitting a failure notification. These JSON files are the authoritative reproduction logs; alert delivery and Docker stdout retention are separate, potentially lossy transports. A notification carries source service, hostname, process-run identity, capture ID, status, file SHA-256 and size, followed by the operator path. Find the named capture on that emitting instance. No user reconstruction of missing diagnostic input is required for a complete new capture.

## Exactly what is retained

The explicitly selected queued URL is retained byte for byte. A caller capture additionally retains the original declared URL before trimming, submitted URL, declared `output_markdown` presence and selected JSON value, and its resolved boolean. The worker does not pretend to recover the caller's original HTTP/MCP encoding.

`rendered_html` is the exact document string returned by the browser. `converter.input` is the single immutable serialized BODY string passed to the pinned `html-to-markdown/v2` converter, including scripts and empty input. These are distinct from the original HTTP response. Each byte value records `captured` or `not_reached`, byte length, SHA-256 and base64, so empty, invalid UTF-8, Unicode and newlines are recoverable without filtering, redaction or truncation.

A renderer read failure can retain the first failed exchange's URL, selected status/framing metadata and available decoded response prefix. Its content-encoded byte count is after Go's HTTP dechunking, not TCP/framing bytes. A partial response cannot reconstruct missing bytes, exact gzip wire data or the remote server's state. Failure stage is the emitting process phase; caller-reported worker stage/code are separate optional evidence. These observations do not prove the historical upstream cause.

Only declared reproduction inputs and selected process/build identity are serialized. There is no task, context, unknown-argument, API-key, authentication-header or configuration serialization. The selected URL or page itself may contain private application data, so these are private logs, not public artifacts.

## Storage and completeness

`CRAWLER_FAILURE_LOG_DIR` selects the private directory (the worker reads it once per process). The worker default is `/tmp/go-ramjet-crawler-failure-logs`; the caller uses its own service-specific leaf. Defaults are container-local and retention-limited: recreation or host loss may remove them. There is no automatic eviction, cleanup or deletion; operators must provision and manage capacity. Disk or permission failures are visible as `write_failed`, never a complete capture.

On supported Unix filesystems, the archive leaf and newly created directories are owner-private 0700 and files are 0600. Existing trusted ancestors may be 0755, and sticky shared temporary ancestors are supported. Symlink traversal, unsafe owners and writable non-sticky ancestors are rejected. Restrictive masks that remove directory owner access require an already provisioned usable 0700 root; the application does not repair arbitrary existing permissions. Same-filesystem hardlink support and directory fsync are required.

A pending file is streamed in base64 without a whole-payload JSON buffer, fsynced, then atomically linked to a new non-overwriting `.json` name. The staging link is removed and the directory synced before the reference becomes `complete`. A linked file with failed cleanup/directory sync is `durability_uncertain`, with its existing final path, not an invitation to retry or overwrite. Crashes may leave pending files or no capture; there is no guarantee that every abrupt process failure can journal itself. Pending/unsealed files are rejected by the replay reader and are not automatically removed.

Payload and metadata hashes detect accidental corruption and inconsistent pairing. They are not cryptographic authentication against an actor able to rewrite private files and hashes.

## Release gate: persistence and capacity

This source-only Draft does not establish production retention. Merging the application can auto-deploy; before any such release, the approved selected-service Compose configuration must already provide a reviewed persistent private bind and `CRAWLER_FAILURE_LOG_DIR` pointing inside it. For example (planning only, not an installed configuration):

```yaml
environment:
  CRAWLER_FAILURE_LOG_DIR: /var/log/crawler-failures
volumes:
  - type: bind
    source: /approved/private/go-ramjet-crawler-failures
    target: /var/log/crawler-failures
    read_only: false
    bind:
      create_host_path: false
```

The host directory must be provisioned by the responsible operator for the actual application UID, 0700, with sufficient dedicated capacity, filesystem hardlink/directory-fsync support, monitoring and an explicit human retention owner. Files remain 0600. No directory creation, mount, settings or production change is part of this source work. An immutable-image rollback must retain the archive bind and captured data. There is no automatic eviction to disguise exhaustion.

## Offline replay

Build the standalone command from the same qualified source/dependencies:

```sh
go build -o /tmp/crawler-replay ./cmd/crawler-replay
/tmp/crawler-replay --caller /private/caller-capture.json --worker /private/worker-capture.json
```

The command imports no application initializer, Redis/provider configuration or browser. It reads only the named captures, verifies completeness and hashes, checks task/URL/output-option correspondence and any caller-reported worker classification, and invokes the local pinned converter on the captured exact argument. It performs no target request or retry.

The report identifies both source instances, the original URL (base64 is authoritative for invalid UTF-8), converter-version qualification, observed input state and replay classification/output hashes. Missing converter provenance is explicitly unqualified; known mismatches are rejected. Publication/render failures with no converter input cannot be reproduced as conversion failures. A local match demonstrates the recorded converter behavior, not a historical origin, network cause or production rollout.
