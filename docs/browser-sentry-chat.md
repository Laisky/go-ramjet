# Chat browser error capture (task 20261012-08)

Only the unified React SPA on exact HTTPS origin `https://chat.laisky.com` is instrumented. Explicit port 443 normalizes to that origin; HTTP and nondefault ports are refused. CV, chat2, SSO, localhost and unrelated hosts are excluded. The legacy template loader does not instrument this SPA. No server telemetry, replay, tracing, network breadcrumbs, user feedback, logs or metrics is enabled.

## Explicit configuration gate

All four public build-time values must be provided; otherwise capture stays disabled:

| Variable                       | Required value                                                                                                                           |
| ------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------- |
| `VITE_CHAT_SENTRY_ENABLED`     | Exact string `true`. Omit or set false to disable.                                                                                       |
| `VITE_CHAT_SENTRY_DSN`         | Approved DSN for the existing `laisky/chat` JavaScript project. HTTPS Sentry ingestion URL; no password, query, fragment or custom port. |
| `VITE_CHAT_SENTRY_ENVIRONMENT` | Approved deployment label, 1-32 ASCII letters/digits/underscore/hyphen.                                                                  |
| `VITE_CHAT_SENTRY_RELEASE`     | `go-ramjet@<7-40 lowercase hexadecimal source SHA>`; prefer the full commit SHA.                                                         |

No project, key, token, credential or access grant was created, retrieved or configured. Do not copy the blog project's configuration. A DSN is public browser configuration; an auth/upload token must never enter source, bundles, `VITE_*` variables or Docker build arguments.

Vite substitutes these values during `pnpm -C web build`. The Docker frontend stage forwards the same four public `--build-arg` values into that command, with disabled/empty defaults. It does not forward them into the final runtime stage. Changing container environment after the build cannot configure compiled assets.

The two release image jobs use only public GitHub repository variables `CHAT_SENTRY_ENABLED`, `CHAT_SENTRY_DSN` and `CHAT_SENTRY_ENVIRONMENT`. Their preparation helper emits exactly four arguments: absent/nonexact-true opt-in sets enabled=false and leaves DSN, environment and release blank. Explicit true requires a canonical HTTPS Sentry ingestion DSN (at most 256 characters) and an environment label matching the table; invalid/missing values fail preparation with a static message and no output values. Release is derived only as `go-ramjet@${{ github.sha }}`, never from a configurable override. Both publishing and cache builds receive identical arguments. Triggers, skips, dependencies, tags, push flags and deployment controls are unchanged. No Sentry auth/upload token is accepted or forwarded.

Variables must be supplied only after the existing `laisky/chat` project and public configuration are approved. Its existing JavaScript platform is reused unchanged with the React SDK. Setting variables alone does not trigger this push-only workflow or update running bundles.

## Privacy and error behavior

Initialization runs before application imports. The existing ErrorBoundary captures caught errors once and preserves its fallback/retry UI when telemetry is disabled or SDK initialization/capture fails. React 19 root hooks capture uncaught/recoverable render errors only after successful target-host initialization. Disabled, failed or unsupported-host initialization retains React's native root error dispatch. Only GlobalHandlers and Dedupe integrations are enabled; they cover uncaught errors and unhandled rejections without SDK default integration drift.

Both `beforeSend` and the final transport reconstruct an allowlisted error schema. SDK-internal events can bypass `beforeSend`, so the transport independently projects even those events. It forwards one error item, validated event ID and generated timestamp, and drops every other envelope item, attachments and trace headers. Telemetry fetches omit credentials and referrers.

Allowed data: fixed site tag, configured release/environment, built-in exception type, fixed message, at most 3 exceptions and 30 frames each with positive numeric coordinates. Arbitrary messages/functions/source lines/variables, auth headers/cookies, API keys/tokens, bodies, prompts/completions/history, MCP arguments/results, user metadata, local storage, breadcrumbs, arbitrary tags/contexts/extra fields and debug metadata are discarded. Added diagnostics are static strings.

Frame URLs must match the exact initial build asset snapshot (`import.meta.url` and at most 40 module script/preload URLs), plus known Vite chunk stems and 8-character hashes. Queries/fragments and URL credentials are excluded. Unknown/fabricated filenames and unloaded lazy chunks lose their frames. This intentionally reduces diagnostic detail.

The ingestion service can still observe the network peer IP and derive geolocation after client filtering. Before any real event, project-side approval must cover Prevent IP storage, default sensitive-data scrubbing, and an explicit approved Remove Anything rule for `$user.geo.**` (or approved equivalent). Prevent IP storage alone does not remove derived geolocation. Approval for that geolocation rule is a separate outstanding prerequisite; no setting is changed by this PR.

Collector Allowed Domains must use the approved exact HTTPS origin `https://chat.laisky.com`, with no wildcard or subdomain expansion. The browser's exact-origin gate remains definitive for protocol/default port; collector settings alone must not be claimed to enforce it. Review retention before activation. No real telemetry test event has been sent.

## Independent release blocker

This Dockerfile rebuilds the complete Go backend and copies the SPA from the same checkout. The current Draft head includes held Ramjet/Go Ramjet ownership and SSO source, and the normal release workflow publishes `latest` then recreates the full service. Wiring public Sentry arguments does not isolate those changes. `[publish only]` still publishes `latest`; it is not an independent Sentry release or a safe way around the holds.

No running go-ramjet process/container was available on saved dev for a verified live source/image baseline. A dev checkout or passing CI does not identify or qualify the production artifact. Sentry can be released independently only after separately approving a known running source/image baseline and an isolated Sentry-only frontend backport/artifact that excludes held frontend and backend changes. A derivative of a pinned approved image must preserve its backend binary, dependencies and entrypoint. Designing or executing that alternate release path is a separate CD/rollout review. All ownership, billing, SSO and dependency holds remain in force.

## Source maps, rollout and rollback gates

Normal builds keep source maps disabled. A private upload workflow is separately gated:

1. Qualify the exact release and its runtime before any rollout. Ramjet #287 and Go Ramjet #80 are merged source changes but remain runtime-unqualified; do not use this task to deploy them.
2. Build the approved exact SHA with matching `VITE_CHAT_SENTRY_RELEASE`. A private upload build may use `vite build --sourcemap hidden`; hidden maps are still output files and must never be published.
3. Upload assets/maps to approved `laisky/chat` from a protected build using an operator-provided scoped credential. Keep it outside frontend variables and logs. Use matching release and URL-prefix correlation for `https://chat.laisky.com/assets/`; debug metadata is intentionally dropped by the client projection.
4. Remove/exclude all `*.map` files and sensitive source files before publishing public artifacts or image layers. Verify final artifact contents and that public map URLs return denial or 404. Separately approved server map denial can provide another layer.
5. Only after explicit configuration and rollout approval, an operator may send a synthetic error without user data and verify project/release/environment, single-event capture and redacted envelopes.

Default-branch pushes can publish/deploy. This task creates only a feature branch and Draft PR; no merge, default push, release-control change or deployment is authorized.

Before an approved rollout, record the previous approved immutable image digest and frontend artifact for rollback. Disabling/omitting `CHAT_SENTRY_ENABLED` produces a disabled rebuild, but changing the variable alone cannot stop an already-built or already-loaded browser client. Apply a separately approved disabled rebuild or restore the recorded approved artifact, then verify with a fresh page/browser context. Existing open chat pages may need reload. Reverting this feature must not roll out held source incidentally; preserve the approved backend baseline and environment. No rollback is executed by this task.

## Acceptance and SDK references

Tests cover valid events, bounded projection, sensitive data, unsafe/fabricated URLs, exact hosts, missing/invalid configuration, duplicate initialization/capture, init undefined/partial failure, capture failure, internal SDK events, blocked envelope types/attachments, transport failure, real SDK error/rejection envelopes through in-memory transport, and mounted routed boundary/root behavior. These tests do not transmit telemetry.

Acceptance: `python3 ci/test_release_deployment.py` (public-argument behavior plus unchanged release/feature-push contracts), `pnpm -C web test`, `pnpm -C web lint`, changed-file Prettier check, and `pnpm -C web build`. Exact-head results are recorded in the Draft PR. No full Docker/backend build is needed for this browser-only change; deployment is gated.

Verified 2026-10-13 against official docs and installed SDK source/types. SDK `@sentry/react` is pinned at 11.6.0 (React 17/18/19 compatible), respecting existing dependency release-age policy.

- [Sentry server-side scrubbing and derived geolocation](https://docs.sentry.io/security-legal-pii/scrubbing/server-side-scrubbing/)
- [Sentry React SPA setup and React 19 error hooks](https://docs.sentry.io/platforms/javascript/guides/react/)
- [Sentry data collection and options](https://docs.sentry.io/platforms/javascript/guides/react/configuration/options/)
- [Sentry Vite source map upload](https://docs.sentry.io/platforms/javascript/sourcemaps/uploading/vite/)
- [Vite source map modes](https://vite.dev/config/build-options.html#build-sourcemap)
