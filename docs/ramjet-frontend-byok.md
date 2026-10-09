# Ramjet frontend BYOK contract

The modern dataset client requires a user-supplied API key for uploading, listing,
and deleting datasets, listing chatbots, and selecting the active chatbot. Missing
keys, anonymous `FREETIER-` tokens, the legacy `DEFAULT_PROXY_TOKEN` placeholder,
and keys containing whitespace or control characters fail before hashing or
sending a request. Validation errors contain a fixed message and log no key.

Valid keys remain byte-for-byte unchanged in the Bearer authorization header.
The existing derived user-id header and all dataset-specific fields are retained.
The optional `X-Laisky-Api-Base` header is forwarded unchanged; this change adds no
endpoint allowlist or restriction on configured internal backends. Requests still
target the current-origin gateway.

Generic chat, account, and sync clients retain their existing behavior. In
particular, their anonymous-session initialization is outside this dataset-client
contract.

## Regression evidence

The retained synthetic tests are in
`web/src/pages/gptchat/utils/__tests__/api-ramjet-byok.test.ts`. They mock fetch and
key hashing, contact no backend, and use no real credentials. Before the fix, all
50 initial invalid-input cases reached fetch and failed rejection expectations;
all 20 valid-key and endpoint-forwarding cases passed. The coordinated baseline run
took 2.56 seconds. A separate baseline reproduced rejection failures in all five
methods for a synthetic C1 Unicode control character before the predicate was
aligned with the backend. The retained contract now contains 75 cases: 55 invalid
inputs and 20 valid-key and endpoint-forwarding cases.

From `web`, run the focused contract with the repository's compatible Node runtime:

```sh
pnpm exec vitest run src/pages/gptchat/utils/__tests__/api-ramjet-byok.test.ts --maxWorkers=1
```

If pnpm is unavailable but matching dependencies already exist, use:

```sh
node node_modules/vitest/vitest.mjs run src/pages/gptchat/utils/__tests__/api-ramjet-byok.test.ts --maxWorkers=1
```

Use the shared qualification slot for execution on dev.
