Historical receipt captured at commit `7c69d28`; the observations below describe the earlier source at `214c80d`. The current [private account ownership boundary](security/private-credential-capabilities.md) uses strict original SSO sessions and an independent operator registry. Retained manual proxy assertions now check verified account identity rather than first-15 or full-key scopes. Generic anonymous free-quota controls remain unchanged. The receipt below is preserved as historical evidence, not a description of current private authorization.

# Manual namespace characterization (task20261008-26)

These opt-in tests document a known ownership gap in the PR80 source at
214c80d1d54fe1b8bf8dfe4d95d8fd917ec4c7f7. They are reproduction evidence,
not a security fix or an ownership policy. Ordinary CI excludes them.

Run locally with:
`go test -tags namespace_characterization ./internal/tasks/gptchat/http -run '^TestNamespaceCharacterization' -count=1`

The actual Go authentication and GET/DELETE dataset proxy are exercised using
synthetic keys and strict in-memory HTTP/Redis transports. Different accepted
provider keys sharing their first 15 characters select the same forwarded UID;
a different-prefix key selects another UID. An incoming forged UID is overwritten.
For paid BYOK callers an incoming isFree header is forwarded unchanged, while
the actual Go UserConfig.IsFree remains false. This does not demonstrate a Go
token-quota bypass or provider validity, and no real dataset operation occurs.

The separate actual free-chat reservation function uses the authenticated Go
UserName for its Redis quota key. Two synthetic free-tier tokens sharing first15
select the same key; the different-prefix control selects another. Incoming UID
and isFree=false headers do not alter that selection. Dataset proxy cases make
zero token-quota Redis operations. These tests do not invoke a model or a full
chat handler.

No issuer/enrollment mapping, legacy identifier, application source, configuration,
CI job or dependency is changed. PR80 still requires an authoritative mapping
between principals, legacy data scopes and permitted operations.


Qualification on 12 October2026: all18 proxy and3 quota leaf cases passed,
zero failures/skips (22.113s after cached compilation). Tagged vet passed33.423s.
The first cold compilation reached its240-second bound before any case ran;
the bounded cached continuation did not repeat a previously executed case.
Required declared modules came from proxy.golang.org with sum.golang.org
verification into an isolated cache; test/vet execution then disabled downloads.
Module files, application implementation and automatic CI are unchanged.
The830-case application suite was not repeated.

Unexpected HTTP through either the default transport or existing request client
is rejected. Pre-fixture authentication/reservation calls are counted and must
remain zero; only the explicit in-memory proxy transport may dispatch.
Redis hooks prevent dialing and reject operations outside the asserted quota
sequence. This remains opt-in characterization, not an ownership repair.
