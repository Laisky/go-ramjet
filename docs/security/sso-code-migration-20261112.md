# CV and GPTChat registered SSO migration

Task: 2026-11-12 ramjet SSO code migration (issuer: Laisky/laisky-blog-graphql#93).

## Contract and rollout gate

Register these clients at the issuer before deploying this consumer:

| Client ID | Exact callback |
| --- | --- |
| ramjet-cv | https://cv.laisky.com/ |
| ramjet-chat | https://chat.laisky.com/ |

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
