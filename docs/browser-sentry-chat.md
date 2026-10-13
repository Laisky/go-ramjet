# Chat browser error capture (task 20261012-08)

Only the unified React SPA on exact hostname `chat.laisky.com` is instrumented. CV, chat2, SSO, localhost and unrelated hosts are excluded. The legacy template loader does not instrument this SPA. No server telemetry, replay, tracing, network breadcrumbs, user feedback, logs or metrics is enabled.

## Explicit configuration gate

All four public build-time values must be provided; otherwise capture stays disabled:

| Variable                       | Required value                                                                                                                           |
| ------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------- |
| `VITE_CHAT_SENTRY_ENABLED`     | Exact string `true`. Omit or set false to disable.                                                                                       |
| `VITE_CHAT_SENTRY_DSN`         | Approved DSN for the existing `laisky/chat` JavaScript project. HTTPS Sentry ingestion URL; no password, query, fragment or custom port. |
| `VITE_CHAT_SENTRY_ENVIRONMENT` | Approved deployment label, 1-32 ASCII letters/digits/underscore/hyphen.                                                                  |
| `VITE_CHAT_SENTRY_RELEASE`     | `go-ramjet@<7-40 lowercase hexadecimal source SHA>`; prefer the full commit SHA.                                                         |

No project, key, token, credential or access grant was created, retrieved or configured. Do not copy the blog project's configuration. A DSN is public browser configuration; an auth/upload token must never enter source, bundles, `VITE_*` variables or Docker build arguments.

Vite substitutes these values during `pnpm -C web build`. The Docker frontend stage forwards the same four public `--build-arg` values into that command, with disabled/empty defaults. It does not forward them into the final runtime stage. Changing container environment after the build cannot configure compiled assets. The release workflow is unchanged; approving and supplying these values in the protected build is a separate gate. Environment and release labels must come from trusted release metadata.

## Privacy and error behavior

Initialization runs before application imports. The existing ErrorBoundary captures caught errors once and preserves its fallback/retry UI when telemetry is disabled or SDK initialization/capture fails. React 19 root hooks capture uncaught/recoverable render errors. Only GlobalHandlers and Dedupe integrations are enabled; they cover uncaught errors and unhandled rejections without SDK default integration drift.

Both `beforeSend` and the final transport reconstruct an allowlisted error schema. SDK-internal events can bypass `beforeSend`, so the transport independently projects even those events. It forwards one error item, validated event ID and generated timestamp, and drops every other envelope item, attachments and trace headers. Telemetry fetches omit credentials and referrers.

Allowed data: fixed site tag, configured release/environment, built-in exception type, fixed message, at most 3 exceptions and 30 frames each with positive numeric coordinates. Arbitrary messages/functions/source lines/variables, auth headers/cookies, API keys/tokens, bodies, prompts/completions/history, MCP arguments/results, user metadata, local storage, breadcrumbs, arbitrary tags/contexts/extra fields and debug metadata are discarded. Added diagnostics are static strings.

Frame URLs must match the exact initial build asset snapshot (`import.meta.url` and at most 40 module script/preload URLs), plus known Vite chunk stems and 8-character hashes. Queries/fragments and URL credentials are excluded. Unknown/fabricated filenames and unloaded lazy chunks lose their frames. This intentionally reduces diagnostic detail.

The ingestion service can still observe the network peer IP. Client filtering cannot conceal it; review Sentry project IP scrubbing, privacy and retention before activation. No real telemetry test event has been sent.

## Source maps, rollout and rollback gates

Normal builds keep source maps disabled. A private upload workflow is separately gated:

1. Qualify the exact release and its runtime before any rollout. Ramjet #287 and Go Ramjet #80 are merged source changes but remain runtime-unqualified; do not use this task to deploy them.
2. Build the approved exact SHA with matching `VITE_CHAT_SENTRY_RELEASE`. A private upload build may use `vite build --sourcemap hidden`; hidden maps are still output files and must never be published.
3. Upload assets/maps to approved `laisky/chat` from a protected build using an operator-provided scoped credential. Keep it outside frontend variables and logs. Use matching release and URL-prefix correlation for `https://chat.laisky.com/assets/`; debug metadata is intentionally dropped by the client projection.
4. Remove/exclude all `*.map` files and sensitive source files before publishing public artifacts or image layers. Verify final artifact contents and that public map URLs return denial or 404. Separately approved server map denial can provide another layer.
5. Only after explicit configuration and rollout approval, an operator may send a synthetic error without user data and verify project/release/environment, single-event capture and redacted envelopes.

Default-branch pushes can publish/deploy. This task creates only a feature branch and Draft PR; no merge, default push, release-control change or deployment is authorized.

Rollback: disable/omit `VITE_CHAT_SENTRY_ENABLED` and rebuild through a separately approved release; UI fallback remains available. Revert the feature commit for a full code/dependency rollback. No rollback rollout is executed by this task.

## Acceptance and SDK references

Tests cover valid events, bounded projection, sensitive data, unsafe/fabricated URLs, exact hosts, missing/invalid configuration, duplicate initialization/capture, init undefined/partial failure, capture failure, internal SDK events, blocked envelope types/attachments, transport failure, real SDK error/rejection envelopes through in-memory transport, and mounted routed boundary/root behavior. These tests do not transmit telemetry.

Acceptance: `pnpm -C web test`, `pnpm -C web lint`, changed-file Prettier check, and `pnpm -C web build`. Exact-head results are recorded in the Draft PR. No full Docker/backend build is needed for this browser-only change; deployment is gated.

Verified 2026-10-13 against official docs and installed SDK source/types. SDK `@sentry/react` is pinned at 11.6.0 (React 17/18/19 compatible), respecting existing dependency release-age policy.

- [Sentry React SPA setup and React 19 error hooks](https://docs.sentry.io/platforms/javascript/guides/react/)
- [Sentry data collection and options](https://docs.sentry.io/platforms/javascript/guides/react/configuration/options/)
- [Sentry Vite source map upload](https://docs.sentry.io/platforms/javascript/sourcemaps/uploading/vite/)
- [Vite source map modes](https://vite.dev/config/build-options.html#build-sourcemap)
