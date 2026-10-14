# Chat browser error capture (task 20261012-08)

Only the unified React SPA on exact HTTPS origin `https://chat.laisky.com` is instrumented. Explicit port 443 normalizes to that origin; HTTP and nondefault ports are refused. CV, chat2, SSO, localhost and unrelated hosts are excluded. The legacy template loader does not instrument this SPA. No server telemetry, replay, tracing or log shipping is enabled. Standard SDK error integrations and useful sanitized console, navigation, fetch and UI breadcrumbs remain enabled; BrowserSession is excluded because release-health sessions are outside this errors-only scope.

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

Initialization runs before application imports. The existing ErrorBoundary captures caught errors once, including React's component stack, and preserves its fallback/retry UI when capture is disabled or the SDK fails. React 19 root hooks capture uncaught/recoverable render errors only after successful target-origin initialization. Disabled, failed or unsupported-origin initialization retains React's native error dispatch. Standard SDK transport and default error integrations cover global errors, unhandled rejections, linked causes and deduplication; BrowserSession is the only excluded default integration.

Ordinary exception messages, function names, complete and lazy-loaded stack frames, mechanisms, React component stacks, diagnostic contexts and Debug ID metadata survive. BeforeSend uses targeted redaction rather than reconstructing an event or envelope schema. SDK dataCollection disables user metadata, cookies, headers, request/response bodies, query parameters, frame variables, GraphQL documents/variables and generative-AI inputs/outputs. Application hooks also remove explicit scope users, auth/credential fields, callback arguments and attachments. URL credentials, queries and fragments are stripped, including embedded URLs.

Raw API/MCP/provider response errors and JSON decoding errors at private response origins are marked where they originate. Their UI messages remain unchanged; serialized error/cause/React copies receive an operation/status summary. Normal programmer and transport errors retain their useful messages. Expected authentication, validation, quota and ordinary HTTP error responses, plus cancellation, remain UI errors. Unexpected transport, response decoding or stream protocol failures are captured once with a static operation tag.

Console breadcrumbs keep standard levels and ordinary safe messages. Known private config/storage/SSE/TTS log families receive useful fixed summaries, raw argument objects are removed, and recognizable credentials are redacted. Navigation/fetch breadcrumbs keep paths, methods and statuses with URL queries/fragments/userinfo removed. UI breadcrumbs retain actions and element types/classes while private chat IDs, labels, titles and attachment names are removed. Malformed breadcrumb getters drop only that breadcrumb and do not interrupt logging/UI flow. Recursive explicit metadata scrubbing has a bounded node/depth budget without imposing the former small stack-frame limit.

This follows the current product policy: useful diagnostics with targeted protection for credentials and private chat data. It does not claim a universal arbitrary-data guarantee for every custom integration or SDK-internal failure. Future private-data-bearing error/log origins need the same targeted marking and tests. No real telemetry has been sent.

## Project settings and activation choices

Reuse the existing `laisky/chat` JavaScript project with the React SDK and approved public DSN; do not copy the blog configuration. Chat collector settings have not been changed by this work. Public configuration and an approved release/rollback path remain required before activation.

Sentry's default server-side sensitive-data scrubbing is useful alongside client filtering. Prevent IP storage, additional geolocation removal, collector Allowed Domains, retention and source-fetching are explicit project privacy/security choices for the owner, not prerequisites imposed by this code. Do not undo settings or retry previously denied writes through this task. If an origin allowlist is chosen, use `https://chat.laisky.com` without wildcard expansion; browser exact-origin gating remains authoritative. Prevent IP storage can still leave derived geolocation, so any stronger no-geolocation policy must be validated against actual collector behavior. Source fetching is optional; private uploaded Debug ID artifacts provide an alternative.

The project service/webhook Verify SSL setting governs outbound integrations, not browser ingestion TLS. This browser uses the configured HTTPS Sentry DSN. No credential or settings operation is part of this PR.

## Independent release blocker

This Dockerfile rebuilds the complete Go backend and copies the SPA from the same checkout. The current Draft head includes held Ramjet/Go Ramjet ownership and SSO source, and the normal release workflow publishes `latest` then recreates the full service. Wiring public Sentry arguments does not isolate those changes. `[publish only]` still publishes `latest`; it is not an independent Sentry release or a safe way around the holds.

Public runtime version metadata identified source revision 46ff3a2360e515a217fab1f0e582afbf3bd6e6ac, and live assets matched that baseline. Public image tag/digest correlation did not prove the running image digest; anonymous registry artifact access required a bearer token and was stopped. The prior local-only candidate and sealed qualification artifacts use the former strict privacy implementation and do not qualify this revised source. A dev checkout or passing CI does not identify or qualify the production artifact. Sentry can be released independently only after separately approving a known running source/image baseline and an isolated Sentry-only frontend backport/artifact that excludes held frontend and backend changes. A derivative of a pinned approved image must preserve its backend binary, dependencies and entrypoint. Designing or executing that alternate release path is a separate CD/rollout review. All ownership, billing, SSO and dependency holds remain in force.

## Private source maps, rollout and rollback gates

Normal builds keep source maps disabled. Source maps are optional for basic capture and do not gate activation on upload credentials. The supported private build mode is explicit:

```sh
CHAT_SENTRY_PRIVATE_SOURCE_MAPS=true pnpm -C web build
```

The pinned official `@sentry/vite-plugin` 5.4.1 runs after React and injects matching Debug IDs before asset hashing. Its supported `sourcemaps.disable: 'disable-upload'` option keeps Debug ID injection while disabling upload. Plugin telemetry and release injection/creation/finalization are disabled. No token is read or supplied by our integration, and no upload or external release call is made. The runtime release still comes from the four deployment values.

Hidden maps are initially build outputs. A post-build helper verifies each emitted mapped JS chunk's matching Debug ID, copies JS/maps and a checksum manifest into an OS temporary directory with private permissions outside both `dist` and the checkout/Docker build context, then deletes all served maps, including CSS maps. A failed quarantine aborts the build and removes collected maps. The helper prints only the private artifact path. Keep that artifact private and delete it when no longer needed; never copy it into public assets or image layers. Existing Docker/CI builds do not select this opt-in flag. Vite can emit unmapped facade/runtime chunks; those remain in the public output and their full event stacks are retained, but the private manifest reports them as unmapped coverage. Every emitted JS map is validated against the official footer of its mapped JS and quarantined. Some final Vite maps can omit the plugin's Debug ID metadata. Only when both metadata fields are absent, the helper prepares the private map copy with `debug_id` and `debugId` from exactly one validated official JS footer, corroborating the runtime marker when present. Every existing ID must match; inconsistent IDs fail. Mapping/source fields and public JS bytes/names remain unchanged. The manifest records original and prepared map checksums and the repair count. This follows the [official bundler metadata stamping behavior](https://github.com/getsentry/sentry-javascript/blob/develop/packages/bundler-plugins/src/core/debug-id-upload.ts).

The private JS and maps must come from the same build, and the delivered public JS must be exactly the JS recorded in that private manifest. Uploading artifacts from a separately rebuilt private mode cannot symbolize the normal map-off Docker/CI assets: their bytes and Debug IDs differ. This PR does not change the Docker/release pipeline to deliver private-mode JS.

1. Qualify the exact source and runtime before rollout. Ramjet #287 and Go Ramjet #80 remain runtime-unqualified; preserve their separate holds and the approved backend baseline.
2. Build the approved exact SHA with matching public environment/release. For the optional private mode, preserve the printed artifact directory and its checksum manifest.
3. After separately authorizing a scoped upload credential, an operator can upload that private artifact bundle to `laisky/chat` using the official Debug ID workflow. Keep credentials outside frontend variables, source and logs. No upload credentials are required to build or test this mode.
4. Verify public `dist`/image layers contain no maps or private sources, and actual public map URLs deny access or return 404. Never interpret hidden maps alone as private.
5. Only after configuration and rollout approval, send one synthetic error without user data and verify project/release/environment, useful diagnostics, single capture and sanitized data. If private artifacts were uploaded, also verify symbolication by Debug ID.

Default-branch pushes can publish/deploy. This task creates only a feature branch and Draft PR; no merge, default push, release-control change or deployment is authorized.

Before an approved rollout, record the previous approved immutable image digest and frontend artifact for rollback. Disabling/omitting `CHAT_SENTRY_ENABLED` produces a disabled rebuild, but changing the variable alone cannot stop an already-built or already-loaded browser client. Apply a separately approved disabled rebuild or restore the recorded approved artifact, then verify with a fresh page/browser context. Existing open chat pages may need reload. Reverting this feature must not roll out held source incidentally; preserve the approved backend baseline and environment. No rollback is executed by this task.

## Acceptance and SDK references

Tests reproduce the former loss of ordinary messages, lazy frames, React component stacks and Debug IDs, then cover their preservation through the actual installed SDK with offline in-memory transport. Behavioral fixtures cover marked private API responses and linked causes, credential aliases, headers/bodies, callback arguments, attachments, sensitive console/DOM/query data, malformed breadcrumbs, expected HTTP/cancellation exclusion, and actual unexpected request failures. Configuration, origin, deduplication and SDK-failure behavior remain covered. Private map contracts cover default-off, no-upload options, matching IDs, quarantine cleanup and unsafe temporary output containment. No test transmits telemetry.

Acceptance: `python3 ci/test_release_deployment.py` (public-argument behavior plus unchanged release/feature-push contracts), `pnpm -C web test`, `pnpm -C web lint`, changed-file Prettier check, and `pnpm -C web build`, including the optional private-map build. The normal frontend test command also runs the private-map contracts. Exact-head results are recorded in the Draft PR. No full Docker/backend build is needed for this browser-only change; deployment is gated.

Verified 2026-10-13 against official docs and installed SDK source/types. SDK `@sentry/react` is pinned at 11.6.0 (React 17/18/19 compatible), respecting existing dependency release-age policy.

- [Sentry server-side scrubbing and derived geolocation](https://docs.sentry.io/security-legal-pii/scrubbing/server-side-scrubbing/)
- [Sentry React SPA setup and React 19 error hooks](https://docs.sentry.io/platforms/javascript/guides/react/)
- [Sentry data collection and options](https://docs.sentry.io/platforms/javascript/guides/react/configuration/options/)
- [Sentry Vite source map upload](https://docs.sentry.io/platforms/javascript/guides/react/sourcemaps/uploading/vite/)
- [Vite source map modes](https://vite.dev/config/build-options.html#build-sourcemap)

- [Sentry breadcrumbs and filtering](https://docs.sentry.io/platforms/javascript/guides/react/enriching-events/breadcrumbs/)
- [Sentry Debug ID troubleshooting](https://docs.sentry.io/platforms/javascript/sourcemaps/troubleshooting_js/debug-ids/)
