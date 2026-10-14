# Historical v6.4.0 compatibility baseline

This retained evidence describes original PR85 at 58b67b65a5d82225bfab93a2c5693f5a3f9e1a1c. It does not qualify the new diagnostic SDK pin; the current source PR records its separate exact-commit acceptance.

# go-utils v6.4 compatibility

This upgrade stays on the v6 module path. The v3/v5 indirect module pins,
Microsoft Speech SDK version, application configuration, and frontend files
are outside this change.

## Consumer contracts

The alert wrapper delegates to the verified-TLS email sender with no insecure
opt-out. The SMTP fixture uses ephemeral certificate trust only within the
test, advertises LOGIN after STARTTLS, and requires successful delivery.
A rejected-authentication control must return an error. A separate fixture
without STARTTLS must fail before the message envelope or body is transmitted.
This verifies the release's SMTP mechanism negotiation without changing
production transport policy or using operator credentials.

Memory tests invoke buildStorageEngine, the same MCP adapter created by chat
requests. They check successful empty and populated searches, NOT_FOUND versus
PAYLOAD_TOO_LARGE errors, append and byte-range arguments, missing-file metadata,
and invalid-path rejection before any backend call. These tests use an injected
ToolCaller; they do not contact a remote MCP service.

User cloud sync derives its key with HKDF-SHA256, compresses with gzip, and uses
the nonce/ciphertext/tag AES-GCM envelope. Frozen records written by the
original pin and v6.4.0 are read by both versions. New output is independently
decoded with standard-library HKDF, AES-GCM, and gzip, including empty data.
A fixed
AES-256-GCM known-answer fixture verifies existing ciphertext reads, while
a standard-library reader independently decodes newly written ciphertext.
Wrong-key, tamper, and truncated-envelope controls must fail. No stored data
is migrated. The application does not use go-utils PasswordHash; its monthly
Elasticsearch password generator uses the standard library.

## Rollout and rollback

Keep this change in a Draft PR until downstream acceptance and exact-head CI
are reviewed. Existing Ramjet/GoRamjet paired ownership and SSO release holds
remain in force; this dependency PR does not approve a release or deployment.
Integrate separately from the active frontend Sentry PR.

Rollback reverts this PR's module version and checksum changes; no database or
file migration is required. The prior module version may again fail delivery
to LOGIN-only SMTP servers, so rollback must account for that behavior.

## Recorded acceptance

The baseline is master 714bb86d9862f5af32d17a0abe632f24465177ef with
go-utils v6.3.2-0.20261008165128-506ec9758d5f. The verified v6.4.0 module
origin is tag commit 6a02887d3dbf60ecadf21a67b5d3b9449fd942e6.

With the same synthetic SMTP fixture, the baseline attempts unsupported PLAIN
against the LOGIN-only server and fails with SMTP 504. With v6.4.0, successful
delivery and the explicit SMTP 535 rejection control both pass. No-STARTTLS
refusal passes on both versions. The fixture accepts EOF only after complete
marker-bearing DATA because the sender closes rather than issuing QUIT.

All memoryx tests and the independent ciphertext compatibility test pass on
both pins with Go 1.27.2 and the race detector. Thirteen release selector and
deployment-contract tests pass using temporary command doubles. Full stable-source local unit/race/build/vet acceptance passed; exact-head
external checks are tracked in the Draft PR description.

One baseline attempt failed setup because an existing declared go-config
module was missing from the offline cache; the dependency was fetched at its
existing pin before the behavioral rerun. One upgraded attempt failed only
the fixture's incorrect QUIT expectation; this was corrected and rerun.
Neither is counted as evidence of an application regression.

Final local acceptance used stable source since 2026-10-13T13:03:02.627997Z,
with its digests checked before publication. Fresh full unit JSON completed
at 13:17:03 UTC: 608 top-level passes plus 504 subtest passes, 30 named test
skips, zero failures. It passed in 108.56 seconds. Full race passed in
420.01 seconds, build in 13.50 seconds, and vet in 22.70 seconds; all returned
exit code zero. Original-pin memory controls and final SMTP/memory race
controls also returned zero after correcting a mock map that retained prior
argument keys. The earlier broad unit invocation started before that fixture
correction and is excluded from acceptance.

The 30 skips cover guarded Mongo, Elasticsearch, external HTTP/Speech,
S3, Twitter, SSL, Docker integrations and the local cross-repository
TestRamjetBYOKActualResolverContract fixture requiring
RAMJET_CREDENTIAL_RESOLVER_PATH. Browser integration build-tag suites are
outside this local default suite and are exercised by existing PR CI.
The existing native Speech SDK was used for compilation/linking; no new
SDK was downloaded or installed. No real credentials or live services
were used by these checks.
