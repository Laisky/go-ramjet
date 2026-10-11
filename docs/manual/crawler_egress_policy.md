# Verified crawler egress and incident 20261010-02

This draft candidate consumes the versioned GraphQL crawler contract at immutable commit `4e24f2e075a0a151069cc3b74dd08ef0e13e9783`. The server policy remains enabled. The incident investigation and shared contract are recorded in [GraphQL PR 64](https://github.com/Laisky/laisky-blog-graphql/pull/64).

## Historical finding

The running worker was built from `bb26ec77f68fe5a28535f6def19635dc6014b8bb` and used a May 2026 GraphQL task model without Egress or RequestChain. Its JSON round-trip loses the newer policy and evidence fields. Synthetic tests reproduce secure-server rejection of this old-model result. The nearby worker task was saved successfully 443 ms before the reported GraphQL alert. GraphQL logs retained only data beginning 19.5 hours after the alert, so the exact request-level cause cannot be established. There was no observed DNS, timeout, or upstream worker error in the retained incident window.

The empty alert object is a separate confirmed logging-policy issue: the deployed alert SDK forwards no fields without an explicit allowlist and always excludes raw errors. The GraphQL companion adds reviewed scalar diagnostics while keeping secrets, targets, and user content out of alerts.

## Enforced transport

Chrome renders using CDP pipes and Fetch interception. Each allowed GET or HEAD is fulfilled through a separate Go transport that dials admitted public IP literals, verifies TLS using the requested hostname, preserves HTTP Host, and records the actual peer and decoded body size. Environment proxies, credentials, remote conversion, browser proxy fallbacks, and direct browser Internet sockets are not used by this path. Unrequested subresources are denied without outbound traffic. Allowed new origins must be separately admitted and pinned. Redirects and aggregate resource budgets remain bounded.

The launcher is mandatory on Linux amd64. Before executing the fixed Chrome binary, it checks the CDP pipe descriptors, closes higher inherited descriptors, sets no_new_privs, and installs a synchronized inherited seccomp filter. It permits Unix IPC but denies Internet and other non-Unix sockets, foreign ABIs, x32, and io_uring. This process filter affects Chrome descendants only; the parent Go fetcher retains its transport. Unsupported platforms, missing launchers, or installation failures fail closed. No host security or network setting is changed.

The runtime Docker image installs `crawler-browser-launcher` into `/usr/local/bin`. CI separately exercises the helper and the real renderer. The parent Go process needs normal outbound network access; Chrome needs its normal local runtime dependencies and CDP pipes. Kernel-denied socket attempts are not included in the application-level blocked-request count; that field is not a complete kernel audit.

## Behavioral acceptance

The original browser-flags-only candidate sent STUN UDP to a reachable local sentinel despite proxy and WebRTC flags. The same page with the launcher renders successfully and sends zero UDP. A launcher-disabled positive control proves the sentinel is reachable. Kernel tests verify IPv4/IPv6 TCP, UDP, raw sockets, packet/vsock, io_uring, x32 and a functioning foreign-ABI fixture are denied; Unix IPC remains usable. Existing threads and exec descendants inherit enforcement. Parent Go TCP remains usable.

Regression controls cover actual-peer receipts, host normalization, public redirects, blocked private targets, image/iframe/popup/worker/WebSocket/preconnect/WebTransport paths, delayed DOM work, body budgets and gzip decoding, TLS SNI/Host, wrong certificates, and cancellation during local rendering. Fixtures contain synthetic content only and use no production credentials or requests.

## Remaining integration gates

This candidate is not release-ready. Automatic approval review rejected an additional guarded patch to `crawler_verified.go`, `crawler_verified_producer.go`, and `crawler_verified_test.go`, even after the expanded human authorization was provided. Those three files remain at the accepted earlier candidate. Full race/coverage acceptance passed for 35 packages (117.39 seconds), and all 12 deployment-contract tests passed. Formatting, module consistency, vet and vulnerability checks passed. Lint still rejects the protected `crawlerTaskFailure` type name (`errname`); renaming it and its uses is another small blocked-file change. The unapplied patch would give the renderer an independent policy clone, retain an immutable submitted-policy snapshot, validate the echoed policy digest, reject empty raw HTML, use shared redirect constants, restrict the legacy URL chain to document requests, and add their regression tests. Do not treat current passing tests as proof of those unapplied changes.

A caller cancellation does not immediately cancel an already dequeued task in another process: there is no distributed cancellation/deadline field in the current task contract. Worker and browser local timeouts bound work, and accepted canceled work publishes a bounded terminal failure. Immediate cross-process cancellation requires a separate contract change and acceptance test.

Before release, resolve the blocked patch, run its red/green controls and the complete read-only lint/race suite, build and attest the paired GraphQL and worker runtime images (including the helper), and verify the production kernel/Chrome runtime through a synthetic unauthenticated fixture. Keep RequireVerified enabled. Review queue compatibility and deploy the worker compatible with the new receipt before routing new-contract server work to it. Roll back the compatible pair using attested image digests if acceptance fails. Merging, image publication, production canaries and deployment require separate authorization.
