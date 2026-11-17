# GPTChat credentials: API key and sync key

GPTChat keeps two secrets in the browser (IndexedDB). They have different roles, so they follow different rules.

| Secret | Scope | Who sees it | What it unlocks |
|---|---|---|---|
| API key (`api_token`) | per session | the API endpoint (`api_base`), with every request | the account billed for requests |
| Sync key (`sync_key`) | per browser | the Ramjet backend (`X-LAISKY-SYNC-KEY`) | the cloud backup: where it is stored and how it is encrypted. The backup holds every session config (API keys included) and the chat history. |

## Invariants

All decisions live in [`credential-policy.ts`](../../web/src/pages/gptchat/utils/credential-policy.ts). Components, `useConfig`, URL overrides and cloud import/export call it; none of them decides on its own.

1. **No write may make the API key and the sync key equal**, whatever their shape. Otherwise, whoever holds the API key (its provider included) could download and decrypt the backup. A pair stored before this rule was enforced is kept as is, so its backup stays reachable. The Data Sync section flags it and offers **Rotate sync key**.
2. **A stored API key that is really a sync key is never used.** That covers a generated sync-key shape (current or older) and a value without an API-key prefix that equals the sync key. Such a key is replaced by a free-tier token, and the session sets `api_token_reset_required`.
3. **A key never moves between sessions without evidence.** Sharing an endpoint is not evidence, because one endpoint serves many accounts. Two kinds of evidence count:
   - the user picks the key from the reset alert;
   - it is the same session's own earlier value. For example, a cloud download carrying a clobbered copy keeps the local key of that session.

## Where the invariants are enforced

| Path | Enforcement |
|---|---|
| Settings editors (`SecretField`) | Validates on **Save**, and explains a rejection instead of dropping it silently. |
| `useConfig.updateConfig` | `validateCredentialUpdate` on every write. Saving a key of the user's own also clears the reset flag. |
| Load and session switch (`hydrateSessionConfig`) | Repairs the session being loaded. |
| Startup (`repairStoredSessionCredentials`) | Repairs every inactive session, so none is exported or uploaded while clobbered. |
| Cloud import (`importAllData`) | Repairs incoming configs, preferring the same session's local key. |
| Cloud export (`exportAllData`) | Exports a free-tier token in place of a clobbered one, so devices not yet updated are protected. |
| URL overrides | `api_key` goes through the policy. `sync_key`, `mcp_servers`, `api_token_reset_required` and `updated_at` can never be set from a link, and are stripped from the address bar. |

## Secret fields are write-only

The settings sidebar never puts a stored secret into an `<input>`. It shows a hint such as `sk-…a1b2`, plus **Show**, **Copy** and **Replace** actions. **Replace** opens an empty editor that stores its value only on **Save** (or Enter). This is the reasoning:

- Password managers have nothing to autofill over. Some extensions (Bitwarden among them) call `focus()` before filling, so the editor's focus guard in [`secret-input-policy.ts`](../../web/src/components/ui/secret-input-policy.ts) is only a safeguard.
- Screen readers announce the real value of a CSS-masked text input. The masked hint reveals nothing until the user asks to show the secret.
- The editor is `type="text"`, masked with `-webkit-text-security`, which Chromium, WebKit and Firefox 114+ support. In other browsers, only the value being typed is visible.

## Config persistence is ordered

All config writes of `useConfig` go through one [`LatestValueWriter`](../../web/src/utils/latest-value-writer.ts). Writes run one at a time, in call order, and each write targets the session that was active when the update was made. Session switch, rename, delete and purge wait for pending writes first. IndexedDB retries a failed transaction after 300 ms, so without this queue an older config could land after a newer one.

## Sync key rotation

`rotateSyncKey` ([`sync-key-rotation.ts`](../../web/src/pages/gptchat/utils/sync-key-rotation.ts)) runs these steps in order:

1. Merge the backup under the old key into local data.
2. Upload everything under a new random key. The uploaded data records the new key as the sync key, so a device that downloads it keeps the new key and does not revert to the old one.
3. Save the new key locally.
4. Call `DELETE /gptchat/user/config` with the old key, which removes every stored version of the old backup.

If step 1 or 2 fails, nothing changes on this device. If step 4 fails, the user is told that the old backup still exists. Other devices must then be given the new key; if they upload with the old key, they recreate the old backup.
