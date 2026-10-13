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

User cloud sync uses the nonce/ciphertext/tag AES-GCM envelope. A fixed
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
