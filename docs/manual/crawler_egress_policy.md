# Verified crawler egress and incident 20261010-02

The candidate consumes the versioned GraphQL crawler contract through the immutable module pin recorded in `go.mod`. The server policy remains enabled. The incident investigation and shared contract are recorded in [GraphQL PR 64](https://github.com/Laisky/laisky-blog-graphql/pull/64).

## Historical finding

The running worker was built from `bb26ec77f68fe5a28535f6def19635dc6014b8bb` and used a May 2026 GraphQL task model without Egress or RequestChain. Its JSON round-trip loses the newer policy and evidence fields. Synthetic tests reproduce secure-server rejection of this old-model result. The nearby worker task was saved successfully 443 ms before the reported GraphQL alert. GraphQL logs retained only data beginning 19.5 hours after the alert, so the exact request-level cause cannot be established. There was no observed DNS, timeout, or upstream worker error in the retained incident window.

The empty alert object was separately reproduced with the deployed SDK revision: its default empty field filter exported no structured fields, and its alert hook excluded errors. The diagnostic repair uses the shared SDK default that forwards ordinary primitive fields and meaningful sanitized error text, with targeted credential, sensitive URL, and payload redaction. An explicit field filter remains optional; the crawler does not require a field-name allowlist. The worker uses a curated logger and bounded stage/category, duration, and canonical task ID fields, while the GraphQL companion retains safe failure metadata. Neither path logs targets, credentials, raw bodies, or user content.

## Enforced transport

Chrome renders using CDP pipes and Fetch interception. Each allowed GET or HEAD is fulfilled through a separate Go transport that dials admitted public IP literals, verifies TLS using the requested hostname, preserves HTTP Host, and records the actual peer and decoded body size. Environment proxies, credentials, remote conversion, browser proxy fallbacks, and direct browser Internet sockets are not used by this path. Unrequested subresources are denied without outbound traffic. Allowed new origins must be separately admitted and pinned. Redirects and aggregate resource budgets remain bounded.

The launcher is mandatory on Linux amd64. Before executing the fixed Chrome binary, it checks the CDP pipe descriptors, closes higher inherited descriptors, sets no_new_privs, and installs a synchronized inherited seccomp filter. It permits Unix IPC but denies Internet and other non-Unix sockets, foreign ABIs, x32, and io_uring. This process filter affects Chrome descendants only; the parent Go fetcher retains its transport. Unsupported platforms, missing launchers, or installation failures fail closed. No host security or network setting is changed.

The runtime Docker image installs `crawler-browser-launcher` into `/usr/local/bin`. CI separately exercises the helper and the real renderer. The parent Go process needs normal outbound network access; Chrome needs its normal local runtime dependencies and CDP pipes. Kernel-denied socket attempts are not included in the application-level blocked-request count; that field is not a complete kernel audit.

## Behavioral acceptance

The original browser-flags-only candidate sent STUN UDP to a reachable local sentinel despite proxy and WebRTC flags. The same page with the launcher renders successfully and sends zero UDP. A launcher-disabled positive control proves the sentinel is reachable. Kernel tests verify IPv4/IPv6 TCP, UDP, raw sockets, packet/vsock, io_uring, x32 and a functioning foreign-ABI fixture are denied; Unix IPC remains usable. Existing threads and exec descendants inherit enforcement. Parent Go TCP remains usable.

Regression controls cover actual-peer receipts, host normalization, public redirects, blocked private targets, image/iframe/popup/worker/WebSocket/preconnect/WebTransport paths, delayed DOM work, body budgets and gzip decoding, TLS SNI/Host, wrong certificates, and cancellation during local rendering. Fixtures contain synthetic content only and use no production credentials or requests.

## Remaining integration gates

The exact-file authorization is now confirmed and the guarded hardening is applied: independent policy snapshots, returned-policy digest checks, empty HTML rejection, shared redirect constants and document-only legacy chains. Serialization-aware red/green controls and the lint naming correction pass. Retained controls cover the currently deployed GraphQL decoder/verifier, allowing the compatible worker to be released first. Full repository checks and exact published runtime-image acceptance remain required.

A caller cancellation does not immediately cancel an already dequeued task in another process: there is no distributed cancellation/deadline field in the current task contract. Worker and browser local timeouts bound work, and accepted canceled work publishes a bounded terminal failure. Immediate cross-process cancellation requires a separate contract change and acceptance test.

Before release, run the full checks, build and attest the paired runtime images (including the helper), and qualify the actual published digests with synthetic unauthenticated fixtures. Keep RequireVerified enabled. Release policy updated 2026-10-17: eligible normal release pushes publish and automatically deploy the current `latest` image; `[publish only]` no longer suppresses deployment. Complete paired qualification before an eligible publishing push or merge. Existing branch/path filters and docs/doc/tests/test/ci commit-prefix skips remain unchanged. After qualification, deploy the backward-compatible worker before the new producer. Preserve pending tasks, the dirty base Compose file, configuration mounts and prior immutable image digests. Restore the producer first, then the worker, if paired rollback is necessary. Observe worker idleness and terminal completion before recreation; zero pending alone is insufficient. No host security/network change or authenticated user-request replay is required.

Startup alert scope: only the static reviewed
`go-ramjet.run_dynamic_web_crawler.verified_crawler_task` logger exports SDK default
fields. Legacy crawler and unrelated services retain metadata-only fields because
their errors may contain raw provider or user content. The existing Alert instance
and rate limiter remain shared, with one hook call per entry and no field-name
allowlist. Actual startup/HTTP receiver controls cover useful future fields,
sanitized causes, private user/payload values and logger-name lookalikes.
