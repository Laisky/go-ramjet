# Ramjet frontend account and model contract

The dataset UI keeps its original Laisky SSO session in a dedicated GPTChat
ownership slot, separate from the model API key and CV's session storage.
Explicit sign-in starts the established SSO redirect contract with a
cryptographically random pending state. Callback acceptance requires that state;
duplicate, unsolicited, or mismatched callbacks cannot replace the session.
The callback scrubs session/state query parameters before any request.
JWT claims are not frontend authorization.

Every private dataset API function accepts an explicit optional session-token
argument and fails before fetch when it is missing. Requests send
`X-Laisky-SSO-Token: Bearer <original session>` only to the current-origin
Ramjet gateway. They never send this session to a selected provider base or a
public query. All five private API fetches use `mode: 'same-origin'` and
`redirect: 'error'` to reject cross-origin or redirected session delivery.
No CV token is adopted and no login credential is created
implicitly.

List/delete datasets and list chatbots require the ownership session without
requiring a model key. With an empty model key they omit Authorization, provider
base, and caller UID. Upload and chatbot activation retain a separately
validated caller model key in Bearer Authorization; valid keys remain unchanged.
Optional `X-Laisky-Api-Base` remains a model-routing option. Server verification
and the independent Ramjet registry decide ownership.

Account/session/model request-boundary changes synchronously clear displayed
items and abort stale responses. Generic chat, account, sync, and anonymous
session initialization retain their existing behavior. Runtime callback
allowlisting and SSO delivery are rollout prerequisites; this source task
performed no real sign-in, account action, or configuration change.

## Retained regression evidence

Current synthetic tests cover session-slot isolation, callback state/scrubbing,
SSO-only metadata, separate BYOK forwarding, missing-session errors, and stale
dataset-response cancellation:

- `web/src/pages/gptchat/utils/__tests__/ownership-session.test.ts`
- `web/src/pages/gptchat/utils/__tests__/api-ramjet-byok.test.ts`
- `web/src/pages/gptchat/components/__tests__/dataset-dependency.test.tsx`

The scoped frontend run passed 108 tests before the final redirect/same-origin
policy assertions and awaited-act fixture update. Formatting and scoped ESLint
passed after those follow-ups; behavior qualification of that exact source is
pending the parent's coordinated run under the shared dev slot. From `web`, with the repository-compatible Node runtime and existing
dependencies, use:

```sh
COREPACK_ENABLE_NETWORK=0 pnpm --config.verifyDepsBeforeRun=false exec vitest run \
  src/pages/gptchat/utils/__tests__/ownership-session.test.ts \
  src/pages/gptchat/utils/__tests__/api-ramjet-byok.test.ts \
  src/pages/gptchat/components/__tests__/dataset-dependency.test.tsx \
  --maxWorkers=1 --no-file-parallelism
```

The earlier BYOK baseline receipt is preserved as historical evidence. Before
that fix, all 50 initial invalid-input cases reached fetch and all 20 valid
key/base controls passed; the coordinated baseline took 2.56 seconds.
A separate baseline reproduced a C1 Unicode-control rejection failure in all
five methods. The earlier retained contract contained 75 cases: 55 invalid
inputs and 20 valid key/base controls. Those counts predate the SSO-only metadata
and original-session ownership flow and do not describe the current test count.
