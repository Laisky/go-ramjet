/**
 * Storage-facing credential maintenance across every stored GPTChat session:
 * repairing clobbered tokens in inactive sessions, offering a sibling's key
 * for the user to choose, and detecting a sync key that doubles as an API
 * token. The decisions themselves come from credential-policy.ts.
 */
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
 * StoredValueWriter stores one value under one storage key. Callers pass
 * their ordered writer, so these writes queue behind their pending ones.
 */
export type StoredValueWriter = (key: string, value: unknown) => Promise<void>

/**
 * writeDirectly is the StoredValueWriter for callers without a write queue.
 */
const writeDirectly: StoredValueWriter = (key, value) => kvSet(key, value)

/**
 * repairStoredSessionCredentials enforces the API token invariant on every
 * stored session, not just the active one. Inactive sessions are then safe
 * to export, upload, or switch to. syncKey is the browser-wide sync key.
 * skipSessionId names the session the caller already repaired and still
 * holds in memory, so its newer in-memory state is not overwritten. write
 * stores a repaired config.
 * It returns the ids of the sessions it repaired.
 */
export async function repairStoredSessionCredentials(
  syncKey: string,
  skipSessionId?: number,
  write: StoredValueWriter = writeDirectly,
): Promise<number[]> {
  const repaired: number[] = []
  for (const entry of await readStoredSessionConfigs()) {
    if (!entry.config || entry.sessionId === skipSessionId) continue
    const result = repairStoredApiToken(entry.config, [syncKey])
    if (result.outcome === 'healthy') continue
    await write(entry.key, result.config)
    repaired.push(entry.sessionId)
  }
  return repaired
}

/**
 * rebindStoredSyncKey makes every stored session config carry syncKey, the
 * browser-wide sync key, as its own copy of it, so no stored or exported
 * config points at an older key. updated_at is left alone: this is not a
 * user edit and must not win a later merge. skipSessionId names a session
 * the caller already wrote. write stores a rebound config.
 * It returns the ids of the sessions it rebound.
 */
export async function rebindStoredSyncKey(
  syncKey: string,
  skipSessionId?: number,
  write: StoredValueWriter = writeDirectly,
): Promise<number[]> {
  const rebound: number[] = []
  if (!syncKey) return rebound
  for (const entry of await readStoredSessionConfigs()) {
    if (!entry.config || entry.sessionId === skipSessionId) continue
    if (entry.config.sync_key === syncKey) continue
    await write(entry.key, { ...entry.config, sync_key: syncKey })
    rebound.push(entry.sessionId)
  }
  return rebound
}

/**
 * listStoredApiTokens returns the API tokens of every stored session except
 * skipSessionId, so a new sync key can be checked against all of them.
 */
export async function listStoredApiTokens(
  skipSessionId?: number,
): Promise<string[]> {
  return (await readStoredSessionConfigs())
    .filter((entry) => entry.sessionId !== skipSessionId)
    .map((entry) => entry.config?.api_token?.trim() || '')
    .filter(Boolean)
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
