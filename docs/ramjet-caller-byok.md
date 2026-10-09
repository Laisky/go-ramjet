# Ramjet caller credential contract

Client-triggered Ramjet requests require the caller's own API key. The Go
Ramjet proxy and chunk-query caller share `resolveRamjetUser`, which rejects
missing credentials, `FREETIER-` session tokens, `DEFAULT_PROXY_TOKEN`, and
keys containing whitespace or control characters before outbound dispatch.
The existing Go key parser continues to accept its existing `sk-` and
`laisky-` formats and minimum length; this change does not broaden provider
credential formats.

The resolver verifies that cached BYOK account state still contains the
original caller key as both its account token and OpenAI token. It never
accepts a configured server key as a replacement. The redundant temporary
server-token assignment in chunk queries is removed.

The modern dataset client uses a shared header builder for upload, list,
delete, chatbot list, and chatbot selection. It rejects anonymous or malformed
credentials before hashing or fetch. Generic Go chat, OneAPI, account, and sync
endpoints retain their existing free-tier behavior.

## Forwarding and compatibility

The browser sends a Bearer key and optional `X-Laisky-Api-Base` to the
current-origin gateway. The Go proxy forwards the same key to configured
`RamjetURL`, using the existing raw Authorization format, and maps the selected
provider base to `X-Laisky-Openai-Api-Base`. Chunk queries use the same resolver
and header construction. No provider allowlist, gateway-only restriction,
or redirect policy is introduced. Explicit selected HTTP(S) URLs retain their
userinfo, query, and fragment fields. Malformed selected URLs fail before
dispatch instead of being replaced with the generic default. The Ramjet
resolver copies cached user state before selecting the provider, so generic
endpoint provider resolution remains unchanged.

The persisted Go user identifier remains the existing first 15 key characters.
Dataset identity, quota keys, and billing identifiers are unchanged. The original
caller key cached in request context also remains intact after the proxy's
existing header mutation. No new credential persistence is added.

Authentication diagnostics now use a SHA256 fingerprint rather than the BYOK
identifier. Short rejected keys are omitted from errors. Full free-tier tokens,
raw provider URLs, and URL-parser errors are omitted from these authentication
logs. Existing user-identifier logging in unrelated endpoints is a separate
privacy follow-up; these changes do not claim to sanitize all application logs.

The existing Go HTTP client follows redirects. Mock transport tests confirm
that Authorization is dropped for an unrelated hostname and retained for the
same hostname or its subdomains; custom identity headers remain copied.
This behavior is characterized without changing destination policy.

## Synthetic regression evidence

All credentials are synthetic, with mock HTTP transports and no production
requests. Before the fix:

- Missing, anonymous free-tier, and placeholder proxy requests reached the
  mocked Ramjet backend with the configured server key.
- Missing-key chunk queries reached the mocked backend.
- Three valid selected URLs containing query, userinfo, or fragment fields
  were silently replaced with the generic default provider. Five malformed
  selected URLs also reached the mocked backend.
- Authentication logs included a partial BYOK key and a complete free-tier
  token; short-key errors and invalid-URL warnings reflected supplied values.
- All 50 invalid-input frontend cases reached fetch. All 20 valid-key and
  endpoint-forwarding cases already passed.

Retained Go tests cover rejection before dispatch, cached server-key mismatch,
unchanged caller key/base/identity, free-tier behavior outside Ramjet,
authentication-log privacy, redirects, and URL-enrichment cancellation.
The final focused qualification passed 60 Go leaf cases across 25 top-level
tests (71 passing entries including parent groups) and 92 frontend tests, including
the existing sync, config, dataset, SHA1, and retirement compatibility controls.
The full HTTP package also passed 218 unit leaf cases across 107 top-level
tests, with eight existing live integration tests skipped.
See `ramjet-frontend-byok.md` for the frontend-only command.

From the repository root, use the existing offline/native SDK configuration
and a shared qualification slot to run:

```sh
go test -p 1 ./internal/tasks/gptchat/http \
  -run '^Test(BYOK|RamjetBYOK|GetUserByAuthHeader|URLEmbedding)' \
  -count=1 -timeout=45s
```

No deployment, host installation, CI expansion, or credential configuration
change is part of this work.

The local opt-in `TestRamjetBYOKActualResolverContract` captures real proxy
handler headers and invokes the actual standard-library-only Python resolver.
Ten cases passed: raw or Bearer inbound credentials with an internal HTTP
provider root, an existing `/v1` suffix, and query, userinfo, or fragment fields. The selected backend
and caller key remained unchanged, and `/v1` was added exactly once. The tested
companion resolver source SHA256 was
`f0c5d490a141d8a30d135f9f0b836cb303e0ed76b172c2203f2f9f28588e62f5`.
Set `RAMJET_CREDENTIAL_RESOLVER_PATH` to the companion public source file for
this local qualification; it stays opt-in when the other repository is absent.
