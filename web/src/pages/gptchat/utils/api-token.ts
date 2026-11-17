/**
 * API token helpers shared by the GPTChat config layer.
 *
 * The API token and the sync key are both secrets that live side by side in
 * the settings sidebar. Browser password managers have been observed copying
 * the sync key into the API key field, so this module also owns the invariant
 * that an API token must never be a verbatim copy of the sync key.
 */
import type { SessionConfig } from '../types'

/**
 * ApiTokenKind classifies a token by shape without exposing its secret material.
 */
export type ApiTokenKind =
  'missing' | 'legacy-default' | 'freetier' | 'byok' | 'custom'

/**
 * createFreeTierToken builds a random free-tier token for anonymous sessions.
 * It returns a new `FREETIER-` prefixed token string.
 */
export function createFreeTierToken(): string {
  const randomStr =
    Math.random().toString(36).substring(2, 18) +
    Math.random().toString(36).substring(2, 18)
  return `FREETIER-${randomStr}`
}

/**
 * describeApiTokenKind summarizes the token shape without exposing secret material.
 * It accepts any possibly-empty token and returns its ApiTokenKind.
 */
export function describeApiTokenKind(token?: string | null): ApiTokenKind {
  const trimmed = token?.trim() || ''
  if (!trimmed) return 'missing'
  if (trimmed === 'DEFAULT_PROXY_TOKEN') return 'legacy-default'
  if (trimmed.startsWith('FREETIER-')) return 'freetier'
  if (trimmed.startsWith('sk-') || trimmed.startsWith('laisky-')) {
    return 'byok'
  }
  return 'custom'
}

/**
 * SYNC_KEY_PREFIX prefixes every auto-generated sync key. No API token issuer
 * uses it, so a token carrying it can only be a misplaced sync key.
 */
export const SYNC_KEY_PREFIX = 'sync-'

/**
 * createSyncKey builds a random sync key for a browser that has none yet.
 * It returns a new `sync-` prefixed key string.
 */
export function createSyncKey(): string {
  return (
    SYNC_KEY_PREFIX +
    Math.random().toString(36).substring(2, 15) +
    Math.random().toString(36).substring(2, 15)
  )
}

/**
 * isApiTokenClobberedBySyncKey reports whether apiToken is really a sync key
 * that ended up in the API key slot (the password-manager autofill bug).
 *
 * A token qualifies when it carries the auto-generated sync key prefix
 * (`sync-`), which catches copies of an older sync key too, or when it is a
 * verbatim copy of the current syncKey. A verbatim copy only counts for the
 * `custom` kind: a recognizable API token (`sk-`, `laisky-`, `FREETIER-`) is
 * kept even when it equals the sync key, because a user may deliberately reuse
 * their API key as their sync key.
 * It returns true when the token must be treated as corrupted.
 */
export function isApiTokenClobberedBySyncKey(
  apiToken?: string | null,
  syncKey?: string | null,
): boolean {
  const token = apiToken?.trim() || ''
  if (!token) {
    return false
  }
  if (token.startsWith(SYNC_KEY_PREFIX)) {
    return true
  }

  const key = syncKey?.trim() || ''
  return token === key && describeApiTokenKind(token) === 'custom'
}

/**
 * hasUsableApiToken reports whether apiToken is a user-provided credential:
 * a `byok` or `custom` token that is not a clobbered sync key. Free-tier,
 * legacy-default, and missing tokens do not count.
 * It returns true when the token can stand in for the user's own API key.
 */
export function hasUsableApiToken(
  apiToken?: string | null,
  syncKey?: string | null,
): boolean {
  const kind = describeApiTokenKind(apiToken)
  if (kind !== 'byok' && kind !== 'custom') {
    return false
  }

  return !isApiTokenClobberedBySyncKey(apiToken, syncKey)
}

/**
 * findRecoverableApiToken picks the most recently updated usable API token
 * from sibling session configs, skipping tokens that were clobbered by the
 * sync key and tokens that carry no user-provided credential.
 * It returns the recovered token, or an empty string when none qualifies.
 */
export function findRecoverableApiToken(
  configs: ReadonlyArray<Partial<SessionConfig> | null | undefined>,
  syncKey?: string | null,
): string {
  let best = ''
  let bestUpdatedAt = -1
  for (const config of configs) {
    const token = config?.api_token?.trim() || ''
    if (!hasUsableApiToken(token, syncKey)) continue

    const updatedAt =
      typeof config?.updated_at === 'number' ? config.updated_at : 0
    if (updatedAt > bestUpdatedAt) {
      best = token
      bestUpdatedAt = updatedAt
    }
  }

  return best
}
