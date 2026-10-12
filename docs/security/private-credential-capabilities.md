# Private account ownership and model credentials

Private datasets and chatbot namespaces belong to a verified Laisky SSO account under a server-controlled Ramjet registry. Model API keys select model credentials and backends; they grant no private namespace ownership. Caller UID and free/paid headers are informational and cannot create a grant.

The earlier complete-key `ckv2-` hash and S3 HMAC-marker proposal is superseded. It did not establish account ownership or a trusted grant authority. The current source uses neither mechanism, aliases no historical namespace, and performs no production rollout or migration.

## Independent account verification

The gateway accepts the original session only in:

    X-Laisky-SSO-Token: Bearer <original Laisky SSO session>

The original model key remains separate in Authorization. Go verifies the session before forwarding a recognized private operation. Ramjet independently verifies that same original session and then checks its own registry; neither the forwarded UID nor a gateway assertion is authorization.

Both private verifiers use the fixed HTTPS WhoAmI authority. They require EdDSA, issuer `laisky-sso`, a decoded 64-byte signature, the documented `iss`, `sub`, `uid`, `exp`, `iat`, `jti`, `username`, and `display_name` claims, finite future expiry, bounded issue time, and matching nonzero canonical UUID subject and UID. Online WhoAmI must return the same active account. Duplicate JSON fields, ambiguous auth/envelope fields, unsupported critical headers, and malformed token encodings fail closed. Local claim parsing supplies rejection checks; the online authority verifies the original signed session.

The shared Go library retains the existing CV verifier/configuration/owner policy behind thin wrappers. Strict private verification is a separate wrapper and does not silently change CV's legacy parsing behavior.

Session and private-proxy requests refuse redirects. A private backend redirect becomes a gateway 502 without Location, preventing browser replay of the session. Public/model Ramjet and OneAPI forwarding remove the session header. A recognizable Laisky session, or the separately supplied original session, is rejected as a model key or when embedded in selected model-base URL userinfo, query, path, or fragment. Unrelated provider JWTs and intentional configured HTTP(S) backends remain supported.

## Disabled operator registry

Ramjet opens only an explicitly configured, existing local SQLite registry with `mode=rw`. Business requests do not create a database or schema. The registry validates its exact schema, configured bucket/root prefix, file ownership and protected parent/file permissions before use. Missing, disabled, inaccessible, or invalid authority returns a fixed unavailable response without revealing paths.

Every grant binds a verified account UUID, one random `pns3-` namespace, an exact permitted operation, and the server-configured bucket and root prefix. S3 objects, marker resemblance, provider-shaped keys, passwords, and caller-selected identifiers cannot create registry authority.

Only a validated upload may initialize a fresh empty random namespace, and only when the operator registry explicitly allows fresh initialization. Existing owners without an upload grant cannot escalate. The registry serializes its grant transaction, but the S3 empty-prefix check is not a distributed transaction with unrestricted legacy writers. A separately authorized rollout must stop those writers and drain their background jobs before enabling fresh initialization.

Historical SHA1, first-15-key, arbitrary-UID, and ckv2-looking records remain in place without automatic access through the new registry. Historical grant import is deliberately unsupported by this implementation. Any future migration needs independently verified account-to-record ownership and exact permitted operations; naming similarity or an encrypted-object password cannot establish namespace ownership. No registry configuration, creation outside disposable tests, host permission change, inventory, migration, or production action occurs in this source task.

## Operation boundaries

| Operation | Original SSO session | Separate model key |
| --- | --- | --- |
| List/delete datasets; list chatbots | Required | Optional |
| Upload; private chat/search; build/activate/publish | Required | Required |
| Read an explicit published share | Public named plaintext selector | Reader model use follows its existing key contract |
| Model-only chunks/query/image operations | Not forwarded | Required under the existing caller contract |

Metadata without Authorization does not invoke the generic Go key resolver or substitute a server model key. Model-using private operations preserve the caller's complete key and selected provider base. The private forwarded UID is the verified account ID for diagnostics only; Ramjet uses the registry namespace. Model-only UID is the informational value `public`.

Object names use full-component validation before access or scheduling. Private caches require the exact request-local grant and authenticated AES password proof on both warm and cold reads. Knowledge of a password alone cannot select another account's private cache.

Published links retain query `uid` and `chatbot_name`. Only explicit public GET `/ctx/share` may use a legacy UID header as a fallback selector; query UID takes precedence. The reader loads only the named `chatbot-share-v2` plaintext archive, never a private current pointer or encrypted private store. Public cache keys include owner and name. Reader model credentials and admission remain the reader's.

Legacy encrypted-original downloads retain their independent object-address/password capability and authenticated decryption. Public/content-addressed files, content-supplied chunk/summary caches, Go generic identity, sync encryption, memory session IDs, and anonymous free-tier quota semantics are unchanged. No memory-read leak or anonymous anti-Sybil guarantee is claimed.

Ramjet has no authoritative paid grant in this flow. The gateway sends the conservative free flag; private users retain free capacity and three process admission permits. The existing eight queued/running upload ceiling remains. Session identity, UID/free headers, fresh model keys, or max_chunks do not grant paid capacity.

## Frontend and qualification

The dataset UI uses a dedicated GPTChat ownership session slot, separate from CV storage and model configuration. Explicit sign-in starts the established redirect flow with a cryptographically random pending state. A callback can replace the session only when its state matches; session/state query parameters are scrubbed before requests. Account/session/model changes clear displayed items and abort stale requests. No actual sign-in or credential creation was performed. Runtime SSO callback allowlisting and delivery remain rollout prerequisites.

The focused Go qualification passed 519 case events across `library/ssosession`, CV, and GPTChat HTTP with exit 0 in 21.759 seconds. These are focused results, not a new full-application count. Synthetic fixtures use local authorities and mock transports; they contact no real SSO, storage, model, account, or quota service. The scoped frontend run passed 108 tests before its final same-origin/redirect policy and awaited-act fixture follow-ups; their formatting and scoped ESLint passed. Exact-source frontend qualification is coordinated separately under the shared dev test slot.

Historical reproduction and prior BYOK qualification receipts remain in [namespace characterization](../namespace-characterization.md), [caller contract](../ramjet-caller-byok.md), and [frontend contract](../ramjet-frontend-byok.md). The manual proxy selectors now assert stable account identity and independence from model keys; generic quota controls remain unchanged. Heavy regression suites remain local and automatic CI is not expanded by this documentation.
