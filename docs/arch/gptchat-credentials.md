# GPTChat credentials: API key and sync key

GPTChat keeps two secrets in the browser (IndexedDB). They have different roles, so they follow different rules.

| Secret | Scope | Who sees it | What it unlocks |
|---|---|---|---|
| API key (`api_token`) | per session | the API endpoint (`api_base`), with every request | the account billed for requests |
| Sync key (`sync_key`) | per browser | the Ramjet backend (`X-LAISKY-SYNC-KEY`) | the cloud backup: where it is stored and how it is encrypted. The backup holds every session config (API keys included) and the chat history. |

The browser-wide sync key (`config_sync_key`) is authoritative. Each session config also carries a copy in `sync_key` for older clients. Loading a session and exporting a backup both rebind that copy to the browser-wide key, and a sync key change rewrites the copy in every stored session.

## Invariants

All decisions live in [`credential-policy.ts`](../../web/src/pages/gptchat/utils/credential-policy.ts). Components, `useConfig`, URL overrides and cloud import/export call it; none of them decides on its own.

1. **No write may make the API key and the sync key equal**, whatever their shape. Otherwise, whoever holds the API key (its provider included) could download and decrypt the backup. A pair stored before this rule was enforced is kept as is, so its backup stays reachable. The Data Sync section flags it and offers **Rotate sync key**.
2. **A stored API key that is, or may be, a sync key is never used.** That covers a generated sync-key shape (current or older) and a value without an API-key prefix that equals the sync key. Such a key is replaced by a free-tier token, and the session sets `api_token_reset_required`.
   - A generated sync-key shape is never an API key, so it is discarded.
   - A value without an API-key prefix that equals the sync key is ambiguous: a password manager may have written it there, or the user may have chosen one value for both. It is set aside in `api_token_before_reset`. Nothing sends it, and it is never discarded silently. The reset alert asks the user to decide:
     - **It is my API key** restores it. If the value is still the sync key, the sync key is rotated first, so the restore cannot recreate the pair.
     - **Re-enter API key** opens the API key editor.
     - **Keep free tier** keeps the free-tier token.

     Any choice ends the reset and clears `api_token_before_reset`.
3. **A key never moves between sessions without evidence.** Sharing an endpoint is not evidence, because one endpoint serves many accounts. Two kinds of evidence count:
   - the user picks the key from the reset alert;
   - it is the same session's own earlier value. For example, a cloud download carrying a clobbered copy keeps the local key of that session.

## Where the invariants are enforced

| Path | Enforcement |
|---|---|
| `useConfig.updateConfig` (the only write path for credentials) | Runs `validateCredentialUpdate` on every write. A new sync key is also checked against the API key of every stored session. The update resolves with the refused fields (`ConfigUpdateResult`), so the UI can explain them. Saving a key of the user's own also clears the reset flag. |
| Settings editors (`SecretField`) | Store a value only on **Save**. They explain a refusal reported by `updateConfig`, and keep the editor and draft open when the write fails. |
| Load and session switch (`hydrateSessionConfig`) | Repairs the session being loaded, and writes back a stale `sync_key` copy. |
| Startup (`repairStoredSessionCredentials`) | Repairs every inactive session, so none is exported or uploaded while clobbered. |
| Cloud import (`importAllData`) | Repairs incoming configs, preferring the same session's local key. Never adopts the backup's sync key: the device keeps the key it downloaded with, and incoming configs are bound to it. |
| Cloud export (`exportAllData`) | Exports a free-tier token in place of a clobbered one, so devices not yet updated are protected. Binds every exported config to the browser-wide sync key. Device-local keys (`config_pending_backup_deletions`) are never exported. |
| URL overrides | Only an explicit allowlist can be set from a link: the scalar fields of `DefaultSessionConfig` and of its `chat_switch`. Values are coerced to the default's type. `__proto__`, `prototype` and `constructor` are rejected anywhere in a path. `api_key` goes through the policy. `sync_key`, `mcp_servers`, `api_token_reset_required` and `updated_at` can never be set from a link, and are stripped from the address bar. |

## Secret fields are write-only

The settings sidebar never puts a stored secret into an `<input>`. It shows a hint such as `sk-…a1b2`, plus **Show**, **Copy** and **Replace** actions. **Replace** opens an empty editor that stores its value only on **Save** (or Enter). This is the reasoning:

- Password managers have nothing to autofill over. Some extensions (Bitwarden among them) call `focus()` before filling, so the editor's focus guard in [`secret-input-policy.ts`](../../web/src/components/ui/secret-input-policy.ts) is only a safeguard.
- Screen readers announce the real value of a CSS-masked text input. The masked hint reveals nothing until the user asks to show the secret.
- The editor is `type="text"`, masked with `-webkit-text-security`, which Chromium, WebKit and Firefox 114+ support. In other browsers, only the value being typed is visible.

MCP server API keys follow the same rule. Editing a server shows only a hint of its stored key and an empty key field. An empty field keeps the stored key, a typed key replaces it, and **Remove key** deletes it.

## Config persistence is ordered

All config writes of `useConfig` go through one [`LatestValueWriter`](../../web/src/utils/latest-value-writer.ts). Writes run one at a time, in call order, and each write targets the session that was active when the update was made. IndexedDB retries a failed transaction after 300 ms, so without this queue an older config could land after a newer one.

Some operations wait for pending writes before they read storage: session switch, rename, delete, duplicate, fork and purge, cloud export, and cloud import. So a backup always holds the newest config. After an import, the active session is reloaded from storage, so a later update builds on the imported config.

`updateConfig` resolves only once the config is stored. If the write fails, it rejects, and the in-memory config reverts unless a newer update has been applied since. Callers that wait for the result, such as the secret editors, show the failure. Controls that apply on change (`applyConfigInBackground`, `ConfigSidebar`'s `applyChange`) log it.

## Sync key rotation

`rotateSyncKey` ([`sync-key-rotation.ts`](../../web/src/pages/gptchat/utils/sync-key-rotation.ts)) runs these steps in order:

1. Merge the backup under the old key into local data.
2. Upload everything under a new random key. Every config in the upload carries the new key as its sync key.
3. Save the new key locally through `updateConfig`, which copies it into every session config. If this fails, the backup uploaded in step 2 is deleted again, because no device has its key.
4. Call `DELETE /gptchat/user/config` with the old key, which removes every stored version of the old backup. The old key is first recorded in `config_pending_backup_deletions`, a device-local list, and is removed from it only once the delete succeeds.

If step 1, 2 or 3 fails, the device keeps the old key. If step 4 fails, or the tab closes during it, the old key stays in the list. Data Sync then shows **Delete old backup** until a retry succeeds.

Other devices must be given the new key. If one of them uploads with the old key, it recreates the old backup.

[`useSyncKeyRotation`](../../web/src/pages/gptchat/hooks/use-sync-key-rotation.ts) wires these steps for the Data Sync section and for the reset alert.
