/**
 * Storage-facing credential maintenance across every stored GPTChat session:
 * repairing clobbered tokens in inactive sessions, offering a sibling's key
 * for the user to choose, and detecting a sync key that doubles as an API
 * token. The decisions themselves come from credential-policy.ts.
 */
import { constantTimeEqual } from '@/utils/constant-time'
import { kvGet, kvList, kvSet, StorageKeys } from '@/utils/storage'
import type { SessionConfig } from '../types'
import { maskSecretHint } from '../utils/api-token'
import {
  findApiTokenCandidates,
  isSyncKeyReusedAsApiToken,
  repairStoredApiToken,
} from '../utils/credential-policy'

/**
 * StoredSessionConfig is one persisted session config and its session id.
 */
interface StoredSessionConfig {
  sessionId: number
  key: string
  config: SessionConfig | null
}

/**
 * ApiTokenChoice is a key another session holds that the user may adopt. It
 * carries a display hint and the holders, never the key itself.
 */
export interface ApiTokenChoice {
  sourceSessionId: number
  hint: string
  sessionNames: string[]
}

/**
 * readStoredSessionConfigs loads every persisted session config. It returns
 * them ordered by session id.
 */
async function readStoredSessionConfigs(): Promise<StoredSessionConfig[]> {
  const keys = (await kvList()).filter((k) =>
    k.startsWith(StorageKeys.SESSION_CONFIG_PREFIX),
  )
  const entries = await Promise.all(
    keys.map(async (key) => ({
      sessionId: Number.parseInt(
        key.slice(StorageKeys.SESSION_CONFIG_PREFIX.length),
        10,
      ),
      key,
      config: await kvGet<SessionConfig>(key),
    })),
  )
  return entries
    .filter((entry) => !Number.isNaN(entry.sessionId))
    .sort((a, b) => a.sessionId - b.sessionId)
}

/**
 * repairStoredSessionCredentials enforces the API token invariant on every
 * stored session, not just the active one. Inactive sessions are then safe
 * to export, upload, or switch to. syncKey is the browser-wide sync key.
 * skipSessionId names the session the caller already repaired and still
 * holds in memory, so its newer in-memory state is not overwritten.
 * It returns the ids of the sessions it repaired.
 */
export async function repairStoredSessionCredentials(
  syncKey: string,
  skipSessionId?: number,
): Promise<number[]> {
  const repaired: number[] = []
  for (const entry of await readStoredSessionConfigs()) {
    if (!entry.config || entry.sessionId === skipSessionId) continue
    const result = repairStoredApiToken(entry.config, [syncKey])
    if (result.outcome === 'healthy') continue
    await kvSet(entry.key, result.config)
    repaired.push(entry.sessionId)
  }
  return repaired
}

/**
 * listApiTokenChoices lists the keys other sessions hold on target.apiBase
 * that the user may adopt for sessionId. Keys stay in storage; only hints
 * leave this module. It returns the choices.
 */
export async function listApiTokenChoices(
  sessionId: number,
  target: { apiBase: string; syncKey?: string },
): Promise<ApiTokenChoice[]> {
  const siblings = (await readStoredSessionConfigs()).filter(
    (entry) => entry.sessionId !== sessionId,
  )
  return findApiTokenCandidates(siblings, target).map((candidate) => ({
    sourceSessionId: candidate.sessions[0].id,
    hint: candidate.hint,
    sessionNames: candidate.sessions.map((s) => s.name),
  }))
}

/**
 * readAdoptableApiToken re-reads the key of sourceSessionId and returns it
 * only if it is still a valid choice for target. Storage may have changed
 * since the choice was offered. It returns an empty string otherwise.
 */
export async function readAdoptableApiToken(
  sourceSessionId: number,
  target: { apiBase: string; syncKey?: string },
): Promise<string> {
  const key = `${StorageKeys.SESSION_CONFIG_PREFIX}${sourceSessionId}`
  const config = await kvGet<SessionConfig>(key)
  const [candidate] = findApiTokenCandidates(
    [{ sessionId: sourceSessionId, config }],
    target,
    1,
  )
  return candidate?.token ?? ''
}

/**
 * isApiTokenOfAnySession reports whether any stored session uses secret as
 * its API token, so a new sync key can be refused before it is saved.
 */
export async function isApiTokenOfAnySession(secret: string): Promise<boolean> {
  const value = secret.trim()
  if (!value) return false
  return (await readStoredSessionConfigs()).some((entry) => {
    const token = entry.config?.api_token?.trim() || ''
    return !!token && constantTimeEqual(token, value)
  })
}

/**
 * findSessionsReusingSyncKey lists the sessions whose API token is also the
 * sync key, which lets anyone holding that API token read the cloud backup.
 * It returns the session ids and the masked hint of the shared value.
 */
export async function findSessionsReusingSyncKey(
  syncKey: string,
): Promise<{ sessionIds: number[]; hint: string }> {
  const sessionIds = (await readStoredSessionConfigs())
    .filter((entry) =>
      isSyncKeyReusedAsApiToken(entry.config?.api_token, syncKey),
    )
    .map((entry) => entry.sessionId)
  return { sessionIds, hint: sessionIds.length ? maskSecretHint(syncKey) : '' }
}
