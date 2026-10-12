# Ramjet caller credential contract

Private Ramjet requests use a verified original Laisky SSO session and a separate
model credential. The gateway sends `X-Laisky-SSO-Token: Bearer <session>` only
to recognized private Ramjet routes after strict online WhoAmI verification.
Ramjet independently verifies that session and checks an exact operator-registry
grant. Forwarded UID is informational and never a private ownership authority.
See [the ownership boundary](security/private-credential-capabilities.md) for
the disabled registry, historical-data boundary, and strict session policy.

Dataset list/delete and chatbot list can authenticate with SSO alone. Upload,
private chat/search, build, activation, and publication additionally require the
caller's model key. Metadata without Authorization does not call the generic key
resolver or substitute a configured server credential.

Model-using routes share `resolveRamjetUser`. It rejects missing credentials,
`FREETIER-` session tokens, `DEFAULT_PROXY_TOKEN`, non-ASCII/control/whitespace
keys, recognizable Laisky SSO JWTs, and a key equal to the separate original SSO
session before model dispatch. Existing `sk-` and `laisky-` formats and minimum
length remain. Cached BYOK state must contain the original caller key as account
and OpenAI token; image routes additionally require that same image credential.
The resolver copies cached state and does not overwrite the generic identity.

The active caller inventory is the SPA dataset methods, the wildcard
`RamjetProxyHandler`, and `queryChunks` for URL enrichment, including Responses
chat. The retained `templates/js/chat.js` is inactive: current `router.go`
retires its HTML/static routes. Generic Go chat, account, sync, memory and free-tier
quota behavior remain unchanged.

## Forwarding and compatibility

The browser targets the current-origin gateway, using a Bearer model key and an
optional `X-Laisky-Api-Base`. Go forwards that exact model key in the existing
raw Authorization format and maps the selected base to
`X-Laisky-Openai-Api-Base`. Chunk queries retain the same key/base contract.
Public/model-only requests strip SSO and use informational UID `public`;
private UID is the verified stable account UUID. Private namespace selection is
performed independently by Ramjet's registry.

Intentional configured HTTP(S) backends, including internal destinations,
userinfo, query, and fragment syntax, remain supported. Malformed selected URLs
fail before dispatch. The narrow additional guard rejects recognizable Laisky
SSO session values in those URL components; it is not a broad provider allowlist
or a prompt/content filter.

Private proxy requests use a request-specific client with redirects disabled;
private backend 3xx responses become 502 without Location. WhoAmI also refuses
redirects. Existing model/public HTTP redirect behavior remains characterized.
Header cloning preserves caller state, and SSO is removed from OneAPI forwarding.

BYOK authentication diagnostics use categorical messages without keys, key
prefixes, credential fingerprints, raw provider URLs, or URL-parser errors.
Existing unrelated endpoint logging is outside this change.

## Current and historical regression evidence

The current focused Go qualification passed 519 case events across the shared
SSO verifier, CV, and GPTChat HTTP packages, exit 0 in 21.759 seconds. Tests cover
strict original-session/active-account binding, malformed and ambiguous claims,
stable account identity across model-key rotation, shared model keys across
different accounts, SSO-only metadata, header separation, redirects, and
targeted URL session rejection. These are focused results, not a full-suite count.

The following receipts describe the earlier BYOK stage, before the account
ownership repair. They remain historical evidence; its first-15 identity and
redirect observations are not the current private ownership contract.

All credentials were synthetic, using mock transports. Before that BYOK fix:

- Missing, anonymous free-tier, and placeholder proxy requests reached the
  mocked backend with the configured server key.
- Missing-key chunk queries reached the mocked backend.
- Empty/substituted cached image credentials were forwarded.
- Three valid selected URLs were replaced with the generic default provider;
  five malformed selected URLs also reached the mocked backend.
- Logs exposed a partial BYOK key and complete free-tier token.
- All 50 invalid-input frontend cases reached fetch; 20 valid controls passed.

That earlier focused qualification passed 63 Go leaf cases across 26 top-level
tests (75 passing entries including parent groups) and 92 frontend tests.
Its full HTTP package passed 221 unit leaf cases across 108 top-level tests,
with eight existing live integration skips. These counts are preserved rather
than combined with the current focused result.

The earlier opt-in `TestRamjetBYOKActualResolverContract` captured proxy
headers and invoked the actual standard-library-only Python resolver. Ten cases
passed for raw/Bearer keys and internal HTTP, existing /v1, query, userinfo and
fragment bases. The companion commit was
`903e8c5c13adeebbddfc7c39af2469a948cea520`, source SHA256
`03e167c5362d18aff215e420e33eb83ef5b27289aab254730c9cfdac5ebc3b86`.
The retained test uses `RAMJET_CREDENTIAL_RESOLVER_PATH` and remains opt-in when
the companion repository is absent. It proves key/backend forwarding, not
private account authorization.

Use the repository's existing offline/native SDK configuration and shared dev
qualification slot. No production calls, deployment, host installation,
credential configuration, or CI expansion is part of this work.


## Image capability selection

The active wildcard proxy accepts `POST /gptchat/ramjet/gptchat/image/dalle`
and forwards the JSON body unchanged to Ramjet. Supply `prompt` and, when
needed, `model` and `image_profile`, with the same caller-owned key and
selected provider headers described above. The gateway does not choose a model,
rewrite capabilities, or perform alternate-model/provider retries.

For the exact official `https://api.openai.com/v1` backend, Ramjet defaults to
the pinned `gpt-image-2-2026-04-21` model and the `gpt-image` profile. The
official backend rejects retired `dall-e-2`/`dall-e-3` models and a conflicting
`legacy` profile. A custom backend requires an explicit model; known GPT image
and DALL-E model names infer their corresponding profile. Unknown custom models
must declare either `image_profile: "gpt-image"` or
`image_profile: "legacy"`. The GPT image profile uses PNG, low quality, one
1024x1024 image, and no `response_format`; the legacy profile requests
`b64_json` and omits `quality` and `output_format`.

For example, a configured compatible backend may receive:

```json
{"prompt":"A synthetic landscape","model":"custom-image-model","image_profile":"gpt-image"}
```

The public success response remains `{"task_id":"...","image_url":["..."]}`.
The gateway also forwards Ramjet's rejection status and generic response
unchanged. The retained `TestRamjetBYOKImageCapabilityPassthrough` regression
uses synthetic transports to check exact request bytes, key, backend, informational
public UID, success-array response, and rejection-response forwarding. It proves
gateway passthrough, rather than claiming to validate the Python capability
resolver or perform image generation.

There is no active Ramjet image UI in this checkout. The modern SPA and retained
inactive legacy source use the separate Go `/images/generations` and edit APIs
for their image controls. Those independent APIs and their model configuration
are unchanged by this Ramjet caller contract.
