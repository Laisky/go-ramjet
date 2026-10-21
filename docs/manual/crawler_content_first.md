# Content-first crawler acquisition: issue 94

Task: **20261013-02**. Source correction for
[go-ramjet issue 94](https://github.com/Laisky/go-ramjet/issues/94), prepared
2026-10-21. This document describes source and synthetic acceptance; it does not
claim a release or production recovery.

## Problem and system correction

The verified worker introduced in PR 81 returned immediately when direct
rendering failed or returned an active challenge. Its existing Jina/Defuddle
adapters were never reached. Independently, both result consumers required a
direct-target receipt, which would reject an honestly identified provider result.
Changing only the worker's retry branch could therefore not restore MCP content.

The worker now uses one acquisition flow: try direct retrieval; classify failure
or unusable output; try the configured providers in their existing priority
order; return usable content with its actual provenance, or a terminal failure.
Existing adapter configuration is reused, including prefix, Scrapeless and
Firecrawl adapters. Priority ties retain the existing random ordering. No new
provider framework, credential, runtime setting or remote converter is added.

The minimal shared contract is owned by `laisky-blog-graphql`, pinned immutably
in this repository's `go.mod` at `f1aa1b9483de563cb036cea734168b763efa7827`
([companion PR 77](https://github.com/Laisky/laisky-blog-graphql/pull/77)).
Both consumers use its `VerifyAcquisition` helper.

| Condition | Result |
| --- | --- |
| Direct content and valid receipt | Return direct content; no provider request. |
| Active direct challenge, transport/header timeout, empty output or conversion failure | If the caller opted in and the worker context remains live, try configured providers. |
| Policy/private-target/unsafe-redirect/peer/evidence/resource-budget denial | Terminal failure; no provider request. |
| Empty provider output, active challenge, recognized Jina failure envelope, adapter error or oversized content | Try the next provider within the same context. |
| Usable provider content | Return provider provenance without a direct receipt or request chain. |
| Caller/worker context canceled or expired | Stop waiting/attempts under that context and reject late success. |

An ordinary HTTP 403 with useful content remains a direct success. Articles about
HTTP errors, HTML examples in `<pre>/<code>`, and fenced or inline Markdown code
are not classified as active challenges. HTML fragments and documents are
converted locally; Markdown is preserved as Markdown and rendered locally when
HTML output is requested. Empty structural HTML is unusable for opted-in tasks,
while image-only or canvas HTML retains its existing useful-content behavior.

## Provenance and enforcement boundaries

The existing task/result JSON envelope gains two optional fields:

```json
{
  "allow_provider_fallback": true,
  "acquisition": { "method": "provider", "provider": "jina" }
}
```

Only the producer's independent submitted snapshot grants the capability.
Returned metadata cannot opt a direct-only request into provider acceptance.
Absent acquisition metadata and explicit `direct` metadata still require the
existing receipt. A provider result requires an opted-in public target, echoed
capability, valid bounded provider identifier, no direct receipt, and no direct
request chain. Task ID, exact target, policy digest, output mode, byte limits and
nonempty requested output remain checked. Provider names are validated for
syntax rather than hard-coded in the shared contract.

Provider endpoint requests use a scoped transport: public-address admission,
pinned dialing, actual-peer checking, hostname-verified TLS, no environment
proxy, no automatic redirects, no cookies and bounded response reads. Wire and
decoded gzip bodies each retain the existing 16 MiB response budget. The scoped
client has a 30-second maximum and inherits the remaining worker context.
The legacy adapter path keeps its previous client behavior.

This verifies the connection to the configured provider endpoint. It does not
attest the provider's downstream target connection, DNS choice or redirect path.
Acquisition metadata is trusted-worker provenance. Provider content is never
represented as direct target connection evidence. No challenge solver, session
credential forwarding or browser-proxy bypass is introduced. Existing provider
credentials, where already configured, remain only on their provider API call.

The original worker timeout (three minutes) and renderer child timeout (one
minute) remain. A direct child timeout can leave time for provider recovery;
fallback does not obtain a fresh worker budget. The queue has no distributed
cancellation/deadline field: caller cancellation ends waiting and acceptance,
but does not immediately cancel an already dequeued worker in another process.
The independent worker context continues to bound that work.

## Diagnostics and stored-data compatibility

Provider attempt diagnostics contain the provider identifier and finite outcome
(`request_failed`, `unusable_content`, or configuration failure). Terminal logs
retain all attempted provider outcomes even when a typed direct transport error
dominates the rendered error text. Provider response/error bodies are excluded.
Provider conversion cannot overwrite the original direct-render diagnostic
captures. The selected original URL/HTML capture behavior delivered in PR 92 and
GraphQL PR 75 remains intact.

Provider success is logged by the caller as `result_accepted` with
`acquisition_method=provider` and its provider identifier. Direct success retains
`result_verified` and its original message. Stored `CrawlRecord` gains optional
`acquisition` metadata, including both task-specific and latest-record indexes.
Historical records without this field remain readable. No data migration or
rewrite is required.

The MCP response `content` shape, public byte-returning fetch API, queue keys,
task states, result retention and billing behavior remain unchanged.

| Producer | Worker | Behavior |
| --- | --- | --- |
| Old | Old | Existing direct receipt contract. |
| Old | New | Direct-only; omitted capability prevents provider use. |
| New | Old | Old decoder ignores additions; valid direct results still work. |
| New | New | Direct content or explicit provider acquisition. |

Old receipt-capable consumers reject provider results if such a result is
misrouted to them; unknown-field loss cannot turn provider content into verified
direct content. Permanent compatibility tests use actual frozen historical JSON
shapes instead of treating the latest shared Go type as an old decoder.

## Behavioral reproduction and regression evidence

Inspected worker base: `4c758f4fbeeadaac0bdcb77939b703730c69f4a6`.
Inspected GraphQL base: `a5b4b7c6f77388594e2ce183c9a4fc000d706a87`.
The worker's regression-only commit is
[`c13fadd`](https://github.com/Laisky/go-ramjet/commit/c13fadd5aeea29698695f1c64a2fd5953d8d21e2).
The companion caller's regression-only commit is
[`f55ca75`](https://github.com/Laisky/laisky-blog-graphql/commit/f55ca7524c04e9000185d187c790264f1d20290d).
Both commit trees match the local snapshots used for the original RED runs.

`TestCrawlerContentFirstChallenge/challenge_falls_back` initially failed:
expected terminal `success`, observed `failed` after an owned HTTP 403 fixture
with an active challenge element. The same run passed the ordinary-403,
quoted-challenge and old-caller controls. The fixed test passes all four cases.
The companion producer and consumer tests independently failed because the
capability was absent and provider content was rejected as unverified.

Independent review led to additional failing behavioral reproductions before
correction: HTML fragments mistaken for Markdown, blank direct HTML accepted,
provider conversion overwriting a direct diagnostic capture, provider outcomes
lost under typed direct errors, and quoted Markdown challenge examples rejected.
All now have permanent regressions alongside positive controls.

Relevant tests live in `crawler_fallback*_test.go`,
`crawler_provider_content_test.go`, `crawler_legacy_wire_test.go`, and
`web_fetch_proxy_http_test.go`. Existing direct, transport, conversion,
diagnostic, priority and legacy fallback suites remain in place. The previous
blanket no-fallback test is scoped to the direct acquisition helper; full worker
tests protect both legitimate fallback and terminal enforcement boundaries.

```sh
# In go-ramjet, after resolving the immutable companion module:
GOWORK=off go test -race -count=1 -timeout=3m \
  ./internal/tasks/gptchat/tasks ./library/log
GOWORK=off go vet ./internal/tasks/gptchat/tasks ./library/log

# Requires redis-server on PATH and the matching companion checkout:
GOWORK=off go test -race -c -o /tmp/issue94-ramjet-service.test \
  ./internal/tasks/gptchat/tasks
# In laisky-blog-graphql:
CRAWLER_SERVICE_TEST_WORKER_BINARY=/tmp/issue94-ramjet-service.test \
  GOWORK=off go test -race -count=1 -timeout=3m ./internal/mcp \
  -run '^TestMCPWebFetchServicePath$' -v
```

The cross-process fixture passed all 15 subcases with the race detector in both
services. It uses real MCP, billing and alert HTTP sockets, disposable Redis,
a separate compiled worker, the existing provider adapter, scoped provider HTTP
transport, real task publication and caller response preparation. It checks
stored provider provenance and rejects fabricated direct evidence. Chrome,
actual public peers and SQL call-log persistence remain explicit fixture seams.
Real-browser and live-provider qualification is separate; missing fixture
prerequisites or skipped tests are not successful production acceptance.

Standalone acceptance used `TZ=UTC`, `GOWORK=off`, read-only module resolution
and the published companion pin above. Initial qualification used Go 1.27.2;
the complete worker/logging suites and both-process service path were rerun on
the final Go 1.27.3 toolchain after the CI security finding described below:

| Check | Toolchain | Recorded result |
| --- | --- | --- |
| Complete worker and logging package race suites | Go 1.27.3 | PASS; 292 tests/subtests passed, zero failures. |
| Paired service path with race detection in both processes | Go 1.27.3 | PASS; all 15 subcases, zero skips (13.472 seconds for the GraphQL package). |
| Complete shared `crawleregress`, `search`, `db/redis` race suites | Go 1.27.2 | PASS with workspace disabled. |
| Targeted MCP tools `TestWebFetch` race suite | Go 1.27.2 | PASS with workspace disabled. |
| Relevant worker/shared/MCP-tools `go vet` | Go 1.27.2 | PASS. |
| Pinned golangci-lint 2.14.0 on both changed production scopes | Go 1.27.2 | PASS; zero issues. |
| Changed-file gofmt/goimports and diff checks | Both | PASS. |
| `go mod tidy -diff` | Go 1.27.3 | PASS; no module metadata changes. |
| `govulncheck` on auditlog and crawler task packages | Go 1.27.3 | PASS; zero reachable vulnerabilities and zero vulnerabilities in imported packages. |

The ordinary worker package run reported 13 skipped prerequisites: ten
real-browser cases, two opt-in live-worker cases and the service helper. The
service helper was subsequently executed by the successful explicit paired
test. The browser/live-provider prerequisites were not available in this source
acceptance environment; no result is inferred for them.

## CI qualification follow-ups

The initial PR head passed the hosted complete Go test run, then the browser
probe stopped before any browser cell with `browser_dependency_graph_unavailable`.
The setup artifact did not expose the underlying Go error, so it does not prove
which cache entry was absent. Behavioral regressions independently established
two required corrections: preparation omitted the historical shared crawler
module, and the original exact-graph matcher rejected its intended new pin.

The same source-reviewed manifest now names the exact old/new shared crawler
version, module checksum and `go.mod` checksum in addition to the existing SDK
transition. Preparation validates both profiles before downloading and verifies
the returned tuples without modifying source or copied manifests. Offline graph
and cache verification remain mandatory. Only these two reviewed module tuples
may differ; the other 65 selected module tuples must match. All six original
browser cells and the SDK-only schema version 1 remain supported. The 38 Python
contracts pass; real accepted/exact/current manifests each resolve and verify all
67 selected cached modules offline. The original matcher rejects those graphs
and the updated exact matcher accepts them. Full browser qualification remains
a hosted CI check on the final PR head, distinct from cache preparation.

Hosted `govulncheck` also reported reachable
[GO-2026-6809](https://pkg.go.dev/vuln/GO-2026-6809) in the pre-existing
`crypto/x509.Certificate.Verify` call in `auditlog.Log.ValidRootCA`.
The advisory affects Go versions before 1.27.3; the application call and its
source file are unchanged from the base. The module, container builder and
explicit verification workflow pins are updated together from Go 1.27.2 to
1.27.3. No application certificate workaround or scanner suppression is added.
The final targeted scan is clear at the reachable-symbol and imported-package
levels; it still reports one vulnerability in a required module that these
packages do not call. The repository-wide scan remains part of hosted CI.

## HOWTO: rollout and rollback

Review both PRs and qualify the paired runtime images before a normal release.
Release pushes can deploy automatically; a draft PR is not release approval.
Use the immutable companion pin in `go.mod`, with no local `replace` directive
or workspace override in the published candidate. Deploy the compatible worker
before enabling the capable caller. Direct retrieval continues in mixed fleets;
provider recovery requires both new versions.

Preserve pending tasks and queue names. Before rollback, stop new capable
submissions and drain accepted work, including running tasks; queue length zero
alone is insufficient. Restore the earlier caller, then the worker, using the
previously qualified immutable images. A rollback restores direct-only behavior;
it requires no stored-data rollback. Old consumers deliberately reject an
in-flight provider result, which is why the drain matters.

The October 20 22:15 challenge incident motivates the confirmed missing-fallback
correction. The independent 22:16 header-timeout event still lacks a confirmed
target/revision/cause. A deterministic timeout regression does not establish that
historical cause, and source acceptance does not prove production recovery.

## Provider references checked on 2026-10-21

- [Defuddle documentation](https://defuddle.md/docs) documents Markdown and HTML
  outputs, supporting local handling of both provider representations.
- [Jina Reader's own issue and maintainer response](https://github.com/jina-ai/reader/issues/1171)
  show a structured Markdown failure preamble and explicitly state that Reader
  does not solve CAPTCHAs. The regression uses a synthetic preamble, not a copied
  target or challenge token. Matching the structured preamble avoids rejecting
  ordinary articles that discuss HTTP 403.

These references inform parsing and failure handling; they are not evidence that
a live provider will recover a particular target.
