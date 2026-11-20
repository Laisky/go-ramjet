/**
 * Credential policy for GPTChat: the one place that decides which API token
 * and sync key combinations may be stored, how a corrupted stored token is
 * repaired, and which credentials may be offered for recovery. UI components,
 * the config hook, URL overrides, and cloud import/export all call it instead
 * of deciding on their own.
 *
 * The two secrets have different roles. The API token is sent to the API
 * endpoint with every request. The sync key alone locates and decrypts the
 * cloud backup (chat history and every session config). The invariants:
 *
 *   I1. No write may make the API token and the sync key equal, whatever
 *       their shape: whoever sees the API token could otherwise read the
 *       backup. Pairs stored before this rule are kept and flagged for an
 *       explicit sync-key rotation, so existing backups stay reachable.
 *   I2. A stored API token that is, or may be, a sync key is never used.
 *       It is replaced by a free-tier token and the session awaits a reset.
 *       A value without a sync-key shape that merely equals the sync key may
 *       be the user's own key, reused on purpose, so it is set aside in
 *       api_token_before_reset for the user to restore, never discarded.
 *   I3. A credential never moves between sessions without evidence. Either
 *       the user picks it, or it is the same session's own earlier value.
 *       A shared endpoint is not evidence, because one endpoint serves many
 *       accounts.
 *   I4. A link never silently changes the API endpoint or the API key of
 *       a stored session, nor a key of the user's own (its API token, or
 *       the key a pending reset set aside). Such a change is held until the
 *       user confirms it, because the server forwards a BYOK key, and the
 *       session's recent history with every request, to whatever endpoint
 *       the client names, and the key's account sees every request. A link
 *       key that is not user-supplied (empty, free tier, a sync key) or
 *       implausibly short is dropped. A link naming the stored endpoint in
 *       another spelling keeps the stored text, because request routing
 *       compares that text. Only a first visit takes the link's values
 *       directly.
 *
 * Comparisons use constantTimeEqual as a best-effort measure. All secrets
 * involved already live in the same browser profile, so isolation, not
 * timing, is what protects them here.
 */
import { constantTimeEqual } from '@/utils/constant-time'
import type { SessionConfig } from '../types'
import {
  createFreeTierToken,
  describeApiTokenKind,
  isGeneratedSyncKey,
  isSameApiEndpoint,
  maskSecretHint,
} from './api-token'

/**
 * CredentialRejection names a field dropped from an update and why:
 * 'api-token-is-sync-key' when the API token would copy the sync key, and
 * 'sync-key-is-api-token' when the sync key would copy the API token.
 */
export interface CredentialRejection {
  field: 'api_token' | 'sync_key'
  reason: 'api-token-is-sync-key' | 'sync-key-is-api-token'
}

/**
 * ConfigUpdateResult is what a stored config update reports back: the
 * credential fields the policy dropped from it. An update that could not be
 * stored rejects instead of resolving.
 */
export interface ConfigUpdateResult {
  rejections: CredentialRejection[]
}

/**
 * ConfigUpdateHandler stores a partial update of the active session config.
 * It resolves once the update is stored, with the dropped credential fields,
 * and rejects when storing failed.
 */
export type ConfigUpdateHandler = (
  updates: Partial<SessionConfig>,
) => Promise<ConfigUpdateResult>

/**
 * CredentialUpdateContext is what validateCredentialUpdate needs to know
 * beyond the active session: otherApiTokens are the API tokens of every
 * other stored session, which a new sync key must not copy either.
 */
export interface CredentialUpdateContext {
  otherApiTokens?: ReadonlyArray<string | null | undefined>
}

/**
 * CredentialUpdateDecision is the outcome of validateCredentialUpdate:
 * accepted holds the updates that may be applied, and rejections lists the
 * dropped fields.
 */
export interface CredentialUpdateDecision {
  accepted: Partial<SessionConfig>
  rejections: CredentialRejection[]
}

/**
 * ApiTokenRepairOutcome tells how repairStoredApiToken left a config.
 * 'healthy' means unchanged. 'restored-previous' means the session's own
 * earlier token came back. 'reset-required' means the session fell back to
 * the free tier and awaits a new key.
 */
export type ApiTokenRepairOutcome =
  'healthy' | 'restored-previous' | 'reset-required'

/**
 * ApiTokenCandidate is a token another session holds on the target
 * endpoint. hint is safe to display, and sessions lists who holds it.
 */
export interface ApiTokenCandidate {
  token: string
  hint: string
  sessions: Array<{ id: number; name: string }>
}

/**
 * sameSecret reports whether a and b are the same non-empty secret after
 * trimming, compared in constant time.
 */
function sameSecret(a?: string | null, b?: string | null): boolean {
  const x = a?.trim() || ''
  const y = b?.trim() || ''
  return !!x && !!y && constantTimeEqual(x, y)
}

/**
 * isApiTokenClobberedBySyncKey reports whether apiToken is really a sync key
 * in the API key slot (I2). That is the case when it has the exact shape of
 * a generated sync key, which also catches older keys. It is also the case
 * when a token with no API-key prefix equals syncKey.
 */
export function isApiTokenClobberedBySyncKey(
  apiToken?: string | null,
  syncKey?: string | null,
): boolean {
  if (isGeneratedSyncKey(apiToken)) return true
  return (
    describeApiTokenKind(apiToken) === 'custom' && sameSecret(apiToken, syncKey)
  )
}

/**
 * isSyncKeyReusedAsApiToken reports whether a usable, API-shaped apiToken
 * also serves as syncKey: a pair stored before I1, which needs a sync-key
 * rotation rather than a token repair.
 */
export function isSyncKeyReusedAsApiToken(
  apiToken?: string | null,
  syncKey?: string | null,
): boolean {
  return (
    sameSecret(apiToken, syncKey) &&
    !isApiTokenClobberedBySyncKey(apiToken, syncKey)
  )
}

/**
 * isUserSuppliedApiToken reports whether apiToken has the shape of a key the
 * user entered: a `byok` or `custom` token that is not a clobbered sync key.
 * It checks shape only and says nothing about validity.
 */
export function isUserSuppliedApiToken(
  apiToken?: string | null,
  syncKey?: string | null,
): boolean {
  const kind = describeApiTokenKind(apiToken)
  if (kind !== 'byok' && kind !== 'custom') return false
  return !isApiTokenClobberedBySyncKey(apiToken, syncKey)
}

/**
 * validateCredentialUpdate decides which credential fields of updates may be
 * applied on top of current, enforcing I1 and I2 for every write path. A
 * field is dropped only when it changes and would create a forbidden pair,
 * so re-sending an unchanged value never fails. A new sync key must differ
 * from the API token of this session and, through context.otherApiTokens,
 * of every other stored session. Saving a key of the user's own completes
 * a pending API key reset, and a reset that ends clears
 * api_token_before_reset.
 * It returns the accepted updates and the rejections.
 */
export function validateCredentialUpdate(
  current: Pick<
    SessionConfig,
    'api_token' | 'sync_key' | 'api_token_reset_required'
  >,
  updates: Partial<SessionConfig>,
  context: CredentialUpdateContext = {},
): CredentialUpdateDecision {
  const accepted: Partial<SessionConfig> = { ...updates }
  const rejections: CredentialRejection[] = []

  if (
    accepted.sync_key !== undefined &&
    !sameSecret(accepted.sync_key, current.sync_key) &&
    [
      accepted.api_token ?? current.api_token,
      ...(context.otherApiTokens ?? []),
    ].some((token) => sameSecret(accepted.sync_key, token))
  ) {
    delete accepted.sync_key
    rejections.push({ field: 'sync_key', reason: 'sync-key-is-api-token' })
  }

  const nextSyncKey = accepted.sync_key ?? current.sync_key
  if (accepted.api_token !== undefined) {
    const token = accepted.api_token
    const copiesSyncKey =
      !sameSecret(token, current.api_token) && sameSecret(token, nextSyncKey)
    if (copiesSyncKey || isApiTokenClobberedBySyncKey(token, nextSyncKey)) {
      delete accepted.api_token
      rejections.push({ field: 'api_token', reason: 'api-token-is-sync-key' })
    } else if (
      current.api_token_reset_required === true &&
      accepted.api_token_reset_required === undefined &&
      isUserSuppliedApiToken(token, nextSyncKey)
    ) {
      accepted.api_token_reset_required = false
    }
  }

  if (accepted.api_token_reset_required === false) {
    accepted.api_token_before_reset = undefined
  }

  return { accepted, rejections }
}

/**
 * LinkCredentialChange is what a link asked to change on a stored session
 * (I4): a different API endpoint, a user-supplied key, or both. It waits for
 * confirmation.
 */
export interface LinkCredentialChange {
  apiBase?: string
  apiToken?: string
}

/**
 * LinkCredentialHoldOptions tells holdLinkCredentialChange about the
 * session: firstVisit is true when nothing was stored for it before the
 * link was opened.
 */
export interface LinkCredentialHoldOptions {
  firstVisit?: boolean
}

/**
 * MIN_LINK_API_KEY_LENGTH is the shortest link-supplied key offered as a
 * replacement for a key the user holds. It is the server's own minimum for
 * `sk-`/`laisky-` keys (getUserByToken rejects shorter ones), and real
 * provider keys are all longer, so a shorter link key could only replace a
 * working key with one that cannot authenticate. Keys typed in Settings are
 * not affected.
 */
const MIN_LINK_API_KEY_LENGTH = 15

/**
 * heldUserApiToken returns the key of the user's own that config holds: its
 * API token when that is user-supplied, otherwise, while a reset is
 * pending, the key the reset set aside (which the recovery alert can
 * restore). It returns an empty string when config holds no such key.
 */
export function heldUserApiToken(
  config: Pick<
    SessionConfig,
    'api_token' | 'api_token_reset_required' | 'api_token_before_reset'
  >,
  syncKey?: string | null,
): string {
  if (isUserSuppliedApiToken(config.api_token, syncKey)) {
    return config.api_token.trim()
  }
  if (config.api_token_reset_required === true) {
    return config.api_token_before_reset?.trim() || ''
  }
  return ''
}

/**
 * holdLinkCredentialChange enforces I4 on linked, the result of applying a
 * link's parameters to current. On a first visit (options.firstVisit) to a
 * session without a key of the user's own it returns linked as it is.
 * Otherwise it returns linked with current's endpoint and credential fields
 * kept, and pending set to the endpoint change and the user-supplied key the
 * link asked for; a link key that is not user-supplied, or shorter than
 * MIN_LINK_API_KEY_LENGTH, is dropped. Either way, a link API base that
 * names current's endpoint in another spelling is replaced by current's
 * text. syncKey is the browser-wide sync key.
 */
export function holdLinkCredentialChange(
  current: SessionConfig,
  linked: SessionConfig,
  syncKey?: string | null,
  options: LinkCredentialHoldOptions = {},
): { config: SessionConfig; pending?: LinkCredentialChange } {
  const movesEndpoint = !isSameApiEndpoint(current.api_base, linked.api_base)
  if (options.firstVisit === true && !heldUserApiToken(current, syncKey)) {
    // Chat, media and realtime routing compare the API base text (the
    // server default is exactly DefaultSessionConfig.api_base), so the same
    // endpoint in another spelling must not replace the stored text.
    return {
      config: movesEndpoint
        ? linked
        : { ...linked, api_base: current.api_base },
    }
  }

  const pending: LinkCredentialChange = {}
  if (movesEndpoint) {
    pending.apiBase = linked.api_base
  }
  const linkToken = linked.api_token?.trim() || ''
  if (
    linkToken.length >= MIN_LINK_API_KEY_LENGTH &&
    !sameSecret(linkToken, current.api_token) &&
    isUserSuppliedApiToken(linkToken, syncKey)
  ) {
    pending.apiToken = linkToken
  }

  return {
    config: {
      ...linked,
      api_base: current.api_base,
      api_token: current.api_token,
      api_token_reset_required: current.api_token_reset_required,
      api_token_before_reset: current.api_token_before_reset,
    },
    pending:
      pending.apiBase !== undefined || pending.apiToken !== undefined
        ? pending
        : undefined,
  }
}

/**
 * repairStoredApiToken enforces I2 on a stored config. A clobbered token is
 * replaced, in order of preference, by options.previous's token and then by
 * a free-tier token. options.previous is the same session's own earlier
 * config, for example the local copy a cloud download is about to
 * overwrite, and it is only used when it holds a key of the user's own on
 * the same endpoint (I3). syncKeys lists every sync key the token must not
 * copy. A replaced token that has no sync-key shape may be the user's own
 * key reused as the sync key, so it is kept in api_token_before_reset for
 * the user to restore. It returns the possibly repaired config and the
 * outcome; a healthy config is returned unchanged.
 */
export function repairStoredApiToken(
  config: SessionConfig,
  syncKeys: ReadonlyArray<string | null | undefined>,
  options?: { previous?: Partial<SessionConfig> | null },
): { config: SessionConfig; outcome: ApiTokenRepairOutcome } {
  const clobbered = (token?: string | null) =>
    isGeneratedSyncKey(token) ||
    syncKeys.some((key) => isApiTokenClobberedBySyncKey(token, key))

  if (!clobbered(config.api_token)) {
    return { config, outcome: 'healthy' }
  }

  const previous = options?.previous
  const previousToken = previous?.api_token?.trim() || ''
  if (
    previous &&
    !clobbered(previousToken) &&
    syncKeys.every((key) => isUserSuppliedApiToken(previousToken, key)) &&
    isSameApiEndpoint(previous.api_base, config.api_base)
  ) {
    return {
      config: {
        ...config,
        api_token: previousToken,
        api_token_reset_required: previous.api_token_reset_required === true,
        api_token_before_reset:
          previous.api_token_reset_required === true
            ? previous.api_token_before_reset
            : undefined,
      },
      outcome: 'restored-previous',
    }
  }

  const replaced = config.api_token?.trim() || ''
  return {
    config: {
      ...config,
      api_token: createFreeTierToken(),
      api_token_reset_required: true,
      api_token_before_reset: isGeneratedSyncKey(replaced)
        ? config.api_token_before_reset
        : replaced,
    },
    outcome: 'reset-required',
  }
}

/**
 * restoringApiTokenNeedsRotation reports whether restoring the key a reset
 * set aside (config.api_token_before_reset) would make the API token and
 * the sync key equal again (I1). The sync key must then be rotated first;
 * if it has changed since the reset, the key can be restored as is.
 */
export function restoringApiTokenNeedsRotation(
  config: Pick<SessionConfig, 'api_token_before_reset' | 'sync_key'>,
): boolean {
  return sameSecret(config.api_token_before_reset, config.sync_key)
}

/**
 * findApiTokenCandidates lists the distinct keys of the user's own that
 * other sessions hold on target.apiBase, for the user to choose from (I3).
 * Nothing is chosen automatically, because one endpoint serves many
 * accounts. entries are the other sessions. limit caps how many distinct
 * candidates are returned. It returns the candidates in session order.
 */
export function findApiTokenCandidates(
  entries: ReadonlyArray<{
    sessionId: number
    config: Partial<SessionConfig> | null | undefined
  }>,
  target: { apiBase?: string | null; syncKey?: string | null },
  limit = 5,
): ApiTokenCandidate[] {
  const candidates: ApiTokenCandidate[] = []
  for (const { sessionId, config } of entries) {
    const token = config?.api_token?.trim() || ''
    if (!isUserSuppliedApiToken(token, target.syncKey)) continue
    if (!isSameApiEndpoint(config?.api_base, target.apiBase)) continue

    const session = {
      id: sessionId,
      name: config?.session_name || `Chat Session ${sessionId}`,
    }
    const known = candidates.find((c) => constantTimeEqual(c.token, token))
    if (known) {
      known.sessions.push(session)
    } else if (candidates.length < limit) {
      candidates.push({
        token,
        hint: maskSecretHint(token),
        sessions: [session],
      })
    }
  }
  return candidates
}
