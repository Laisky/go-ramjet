# CV and GPTChat registered SSO migration

Task: 2026-11-12 ramjet SSO code migration (issuer: Laisky/laisky-blog-graphql#93).

## Contract and rollout gate

Register these clients at the issuer before deploying this consumer:

| Client ID | Exact callback |
| --- | --- |
| ramjet-cv | https://cv.laisky.com/ |
| ramjet-chat | https://chat.laisky.com/ |
| ramjet-chat2 | https://chat2.laisky.com/ |

These are public browser clients, without client secrets. The issuer must allow
the corresponding origins for its token-exchange CORS responses. Registration
requires exact HTTPS callbacks, no query or fragment. Other deployments require
their own explicitly registered origin; do not use wildcard callbacks. Hosts
cannot be shared by different registered clients under the current issuer policy.

The consumer sends all five code-flow fields inside redirect_to, stores a fresh
256-bit state and S256 PKCE verifier in tab-local sessionStorage, and exchanges
sso_code once via POST /sso/token. Callback data is scrubbed before telemetry,
metadata bootstrap, React mounting, or exchange. Do not retry ambiguous exchanges:
start a fresh sign-in. The original path, query and fragment are restored from
tab-local state, never from a callback-provided redirect. Tabs without the pending
context cannot complete a sign-in. Browser storage and WebCrypto are required to
begin sign-in; persistence failure after exchange has an in-memory fallback.

Deployment order: issuer code and client registry, consumer code, browser login
acceptance for both sites, then issuer legacy-flow retirement after all other
consumers are migrated. No production deployment or registry edits are performed
by this PR. Keep image tags latest; no digest pinning is introduced.

## Compatibility

Existing cv_sso_token and gptchat_ownership_sso_token sessions retain their
separate storage slots. Existing backend owner checks and session headers are
unchanged. Model/API keys are never used as ownership sessions. Saved datasets,
chat history, CV content and settings are untouched.

Legacy sso_token callbacks are intentionally rejected and scrubbed, including
callbacks already in flight when this consumer deploys. Users of such old tabs
must initiate sign-in again. Existing stored sessions are not replaced by these
callbacks. Local development requires an issuer-registered HTTPS callback.

## Evidence

Before changing the legacy admission functions, two behavioral regression tests
showed a URL bearer replacing an existing CV or GPTChat session. Both failed
with actual forged versus expected existing, then passed after removal.
This demonstrates legacy admission, not authorization bypass of backend checks.
The new code-flow test initially failed because the new module did not exist;
that scaffold failure is not claimed as a behavioral defect reproduction.

Tests cover PKCE binding, single exchange, URL scrubbing, app bootstrap order,
legacy rejection, distinct sessions, stored-owner editing, invalid/expired/future
pending state, wrong app/callback/route, external return paths, duplicate state,
network/HTTP/malformed exchange failures, replay suppression and disabled storage.

History retrieval and recording: the coordinator reported FileIO access denied.
That denial was not bypassed or retried in this work. FileIO search/readback is
therefore incomplete; this document and the PR description retain task evidence
until authorized access is restored. No credentials or request payloads are saved.

Acceptance checks: 57 focused tests passed, TypeScript build check passed, and
targeted ESLint passed. Production Vite build passed (existing chunk warnings).
The first full-suite run after fixture migration passed 667/669 tests; two
chat-domain renders hit their unchanged 5-second timeout under parallel load.
A bounded-worker full-suite rerun is recorded in the PR description. The separate
private source-map contract tests passed in pnpm test before Vitest ran.

## Resumed acceptance (2026-11-12)

- The prior frontend timeout did not reproduce in GitHub Actions: full frontend tests, lint, and build succeeded for `3a5458e` (run `43153639201`). This is evidence against changing application behavior or loosening test timeouts.
- The same run stopped backend validation in release-contract tests: three `KeyError: password` failures. Reproduced locally before the fix. The workflow already uses SSH key authentication and host fingerprint verification; the contract was stale. Updated the assertion without changing deployment credentials/settings, retained `latest`, and added negative controls for missing/changed keys/fingerprints and password reintroduction. All 29 contracts pass.
- Verified that the logger dependency redacts generic `code`/`token`, but not `sso_code`/`sso_token`. Behavioral requests reproduced these credentials reaching the inner HTTP handler before logger/recovery. The outer HTTP normalization boundary now redacts both fields and RequestURI before any middleware; browser URL processing is unaffected. Duplicate, mixed-case, percent-encoded, malformed-name/value cases are regression coverage; ordinary valid query bytes are unchanged. The library/web race suite passes (1.102s); registered cv/chat/chat2 root callbacks return the SPA without a redirect. Full `go test ./... -count=1` and `go vet ./...` pass; golangci-lint reports zero issues.
- FileIO evidence retrieval/storage remains unavailable due the prior authorization denial. No later search or verified FileIO closeout is claimed. Private local instruction-file access was also denied; no bypass was attempted.
- This remains a Draft client migration. Registry activation, merge and deployment are not claimed complete; full backend acceptance and independent review are required before readiness.
- The next security gate exposed reachable `GO-2026-6888` in Brotli v1.2.6 (crawler decompression). Updated only the official module patch to v1.2.7 and its checksums; `go mod tidy` introduced no unrelated changes. Full Go tests pass again and govulncheck now reports zero reachable vulnerabilities. The scanner failure→pass is verified; a crafted-stream allocation exploit was not independently reproduced, so this is not claimed as behavioral exploit validation. The existing govulncheck gate remains the regression guard against the vulnerable version.

## Per-host registration and route compatibility

The shared SPA supports `/`, `/cv`, and `/gptchat` on each host. Reproduced Chat sign-in from `cv.laisky.com/gptchat`: it incorrectly requested `ramjet-chat` for a hostname owned by `ramjet-cv`. Root callback bootstrap also used the host theme rather than the initiating app. Registration now follows hostname, while the PKCE-bound validated return route determines the distinct application session. Reciprocal CV-on-chat/chat2 tests preserve the other session and restore path/query/fragment. Full frontend suite: 676 tests across 82 files pass; TypeScript and repository-wide ESLint pass.

Staged plan (requires explicit configuration approval, not executed):
1. For each of the three per-host clients above, temporarily register five exact HTTPS callback paths: `/`, `/cv`, `/cv/`, `/gptchat`, `/gptchat/`, with per-client `legacy_token_redirect: allow`. Five entries per client is below the issuer limit of eight. Keep global legacy compatibility for unmigrated consumers.
2. Deploy the new consumer with `latest`, verify code sign-in and separate established sessions at both application routes on all three hosts, plus return query/hash and replay rejection.
3. Remove the four non-root temporary callbacks and the per-client legacy allowance after acceptance. Do not retire the global legacy mode until other consumers are migrated.

This covers conventional existing routes without wildcard permissions or duplicate hostname ownership. React also accepts unusual mixed-case route spellings; legacy callbacks from such old tabs may require refresh/restart because issuer matching is exact. Likewise, already-in-flight bearer callbacks intentionally restart sign-in after consumer deployment. No zero-interruption guarantee is claimed for those cases. Alternate client IDs cannot share a hostname under issuer validation, so they do not solve staging.

- Exact-head CI on `46c760f` passed full frontend, backend tests/browser smoke/vet/lint but failed govulncheck because CI selected module minimum Go1.27.3 (`GO-2026-6859`), while local acceptance used Go1.27.4. Synchronized the module minimum, Docker builder and auxiliary Go CI pin to1.27.4. Service image `latest` is unchanged. Local full Go acceptance and zero-reachable scan were already run with1.27.4; new exact-head CI must verify the selection.

Independent static review found no remaining P2 in the callback redaction/route binding. Added explicit root-landing mismatch, unknown-route, backslash and control-character negative controls; return URLs must remain same-origin. Full frontend recheck, TypeScript and ESLint pass. This is not live browser/issuer acceptance.


## Callback privacy follow-up (2026-11-13)

Credential-bearing callback responses now emit `Referrer-Policy: no-referrer`
before the document loads same-origin assets. JavaScript URL scrubbing remains
necessary but is not a substitute for that response header. The HTTP boundary
also removes `sso_code` and `sso_token` query values from incoming `Referer`
headers before application logging, including encoded names and duplicate header
values. Ordinary routes retain their existing referrer policy and unrelated
query/referrer bytes are preserved.

Synthetic request regressions reproduced the old permissive callback policy and
credentials reaching an inner logging handler, then passed after hardening.
These are application-boundary controls, not evidence that production ingress
logs are clean. No production leak was established. A reverse proxy receives
the initial callback request before the application can sanitize it. Repository
inspection found development proxy examples but no production ingress log format.

Before rollout, the operator must verify the actual ingress/CDN access-log and
tracing configuration excludes callback query strings and raw Referer values,
and preserves the callback response policy. Use a synthetic, non-redeemable
canary to request the callback document and same-origin assets; inspect the
browser network Referer headers and each ingress/application log sink. Never
use a live authorization code or bearer token as the canary. This live proxy and
browser acceptance is still pending; this change does not alter production
configuration or claim to sanitize logs already captured upstream.

### P2 failed-exchange route recovery acceptance (2026-11-13)

- Reproduced ten failing route assertions on the pre-fix implementation: network failure, HTTP rejection, malformed response JSON/body/token, timeout, and CV-host GPTChat cross-route callbacks. Existing sessions and invalid-context controls remained intact.
- Restore the fully validated return path in the exchange finalizer, after callback scrubbing and session handling but before SPA bootstrap. Restore only while the browser remains at the scrubbed callback; newer navigation wins. Invalid/expired/mismatched contexts still cannot select a destination.
- Permanent regressions cover failure return paths, separate CV/chat sessions, one-shot consumption, newer navigation during success/failure, and the existing actual SPA bootstrap ordering.
- Final acceptance: 685 frontend tests across 82 files passed with two workers; ESLint, TypeScript/Vite build, Go web race tests and Go vet passed. Initial unrestricted suite had two render timeouts; both pass focused and in the bounded full suite. An initial early-restoration implementation failed bootstrap ordering and was corrected before publishing.
- This is repository-level acceptance, not proof of deployed ingress log confidentiality or a production browser round trip; the live privacy gates above remain open.
