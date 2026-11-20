/**
 * Session config hydration for GPTChat: the read-normalize-repair pipeline
 * that turns a stored config into one that is safe to use.
 *
 * Pipeline order matters. Model migrations and the sync key come first; the
 * API endpoint is settled next because every credential decision is scoped
 * to it; the stored API token is repaired next, so a key it sets aside is
 * known; URL overrides are then applied as guarded writes, and a link that
 * would change a stored session's endpoint or key waits for the user.
 */
import { kvGet, kvSet, StorageKeys } from '@/utils/storage'
import { DefaultModel, ImageModelFluxDev, isImageModel } from '../models'
import { DefaultSessionConfig, type SessionConfig } from '../types'
import {
  createFreeTierToken,
  createSyncKey,
  describeApiTokenKind,
  isSameApiEndpoint,
} from '../utils/api-token'
import {
  holdLinkCredentialChange,
  isApiTokenClobberedBySyncKey,
  type LinkCredentialChange,
  repairStoredApiToken,
  validateCredentialUpdate,
} from '../utils/credential-policy'
import {
  applyUrlOverridesToConfig,
  getSessionConfigKey,
  normalizeConfigNumericFields,
} from '../utils/config-helpers'

/**
 * resolveGlobalSyncKey returns the browser-wide sync key, adopting the one
 * stored in config or fallbackConfig, or generating a new one, when the
 * browser has none yet. It persists an adopted or generated key, because
 * every session must share it.
 */
async function resolveGlobalSyncKey(
  config: SessionConfig,
  fallbackConfig?: SessionConfig,
): Promise<string> {
  const stored = await kvGet<string>(StorageKeys.SYNC_KEY)
  if (stored) {
    return stored
  }

  const syncKey = config.sync_key || fallbackConfig?.sync_key || createSyncKey()
  await kvSet(StorageKeys.SYNC_KEY, syncKey)
  return syncKey
}

/**
 * pickFallbackApiToken returns the token of fallbackConfig (normally the
 * previously active session) that a session without a token on apiBase may
 * inherit: only when both point at the same endpoint and the token is neither
 * missing, the legacy placeholder, nor a clobbered sync key.
 * It returns the inheritable token, or an empty string.
 */
function pickFallbackApiToken(
  fallbackConfig: SessionConfig | undefined,
  apiBase: string,
  syncKey: string,
): string {
  const token = fallbackConfig?.api_token?.trim() || ''
  if (
    !token ||
    token === 'DEFAULT_PROXY_TOKEN' ||
    isApiTokenClobberedBySyncKey(token, syncKey) ||
    !isSameApiEndpoint(fallbackConfig?.api_base, apiBase)
  ) {
    return ''
  }
  return token
}

/**
 * HydratedSessionConfigResult is the outcome of hydrateSessionConfig: the
 * usable config, the raw stored config (null when absent), and whether the
 * usable config differs from storage and should be written back.
 * pendingLinkChange is the endpoint and key a URL asked to change on a
 * stored session (I4); it is not applied and awaits confirmation.
 */
export interface HydratedSessionConfigResult {
  config: SessionConfig
  savedConfig: SessionConfig | null
  changed: boolean
  pendingLinkChange?: LinkCredentialChange
}

/**
 * PendingLinkChange is the endpoint and key a link asked to change, held for
 * the session it was opened in until the user confirms or dismisses it.
 */
export interface PendingLinkChange extends LinkCredentialChange {
  sessionId: number
}

/**
 * mergeSessionConfigs overlays the current config onto a loaded config while keeping nested switches intact.
 */
export function mergeSessionConfigs(
  baseConfig: SessionConfig,
  overrideConfig: SessionConfig,
): SessionConfig {
  return normalizeConfigNumericFields({
    ...baseConfig,
    ...overrideConfig,
    chat_switch: {
      ...baseConfig.chat_switch,
      ...overrideConfig.chat_switch,
    },
  })
}

/**
 * hydrateSessionConfig reads, normalizes, and repairs a session config before use.
 * sessionId selects the stored config; options.fallbackConfig (normally the
 * previously active session) supplies the API endpoint of a missing config
 * and a token for a tokenless session on the same endpoint; and
 * options.applyUrlOverrides applies URL query overrides as guarded writes;
 * an endpoint or key change on a stored session is returned as
 * pendingLinkChange instead of being applied.
 * It persists only a newly adopted or generated browser-wide sync key; the
 * session config itself is returned with changed set, and the caller decides
 * whether to write it back.
 */
export async function hydrateSessionConfig(
  sessionId: number,
  options?: {
    fallbackConfig?: SessionConfig
    applyUrlOverrides?: boolean
  },
): Promise<HydratedSessionConfigResult> {
  const key = getSessionConfigKey(sessionId)
  const savedConfig = await kvGet<SessionConfig>(key)
  const fallbackConfig = options?.fallbackConfig

  console.debug(
    `[useConfig] hydrate session ${sessionId} config: saved=${!!savedConfig} savedApiTokenKind=${describeApiTokenKind(savedConfig?.api_token)} fallbackApiTokenKind=${describeApiTokenKind(fallbackConfig?.api_token)}`,
  )

  let finalConfig: SessionConfig = {
    ...DefaultSessionConfig,
  }

  if (savedConfig) {
    finalConfig = normalizeConfigNumericFields({
      ...finalConfig,
      ...savedConfig,
      chat_switch: {
        ...finalConfig.chat_switch,
        ...savedConfig.chat_switch,
      },
    })
  }

  let configChanged =
    !!savedConfig &&
    (savedConfig.selected_model !== finalConfig.selected_model ||
      savedConfig.selected_chat_model !== finalConfig.selected_chat_model ||
      savedConfig.selected_draw_model !== finalConfig.selected_draw_model)

  const hasSelectedChatModel =
    savedConfig &&
    Object.prototype.hasOwnProperty.call(savedConfig, 'selected_chat_model')
  if (!hasSelectedChatModel) {
    console.debug(
      `[useConfig] migrating legacy selected_model to selected_chat_model: ${finalConfig.selected_model}`,
    )
    finalConfig.selected_chat_model = isImageModel(finalConfig.selected_model)
      ? DefaultModel
      : finalConfig.selected_model || DefaultModel
    configChanged = true
  }

  const hasSelectedDrawModel =
    savedConfig &&
    Object.prototype.hasOwnProperty.call(savedConfig, 'selected_draw_model')
  if (!hasSelectedDrawModel) {
    console.debug(
      `[useConfig] migrating legacy selected_model to selected_draw_model: ${finalConfig.selected_model}`,
    )
    finalConfig.selected_draw_model = isImageModel(finalConfig.selected_model)
      ? finalConfig.selected_model
      : ImageModelFluxDev
    configChanged = true
  }

  if (!finalConfig.selected_model) {
    finalConfig.selected_model = finalConfig.selected_chat_model
    configChanged = true
  }

  // The browser-wide key is authoritative. A stale per-session copy (left
  // by a sync key change or an older import) is corrected in storage too,
  // so nothing exports or reads an older key.
  const globalSyncKey = await resolveGlobalSyncKey(finalConfig, fallbackConfig)
  if (savedConfig && savedConfig.sync_key !== globalSyncKey) {
    configChanged = true
  }
  finalConfig.sync_key = globalSyncKey

  // A missing config inherits the endpoint together with the token below, as
  // createSession does; a stored config keeps its own endpoint.
  if (!savedConfig || !finalConfig.api_base?.trim()) {
    finalConfig.api_base =
      fallbackConfig?.api_base?.trim() || DefaultSessionConfig.api_base
    configChanged = true
  }

  if (!finalConfig.session_name) {
    finalConfig.session_name = `Chat Session ${sessionId}`
    configChanged = true
  }

  // A token that is really the sync key (browser password-manager autofill
  // in the settings sidebar) is never sent anywhere. The session falls back
  // to the free tier and the UI asks the user to re-enter or pick a key;
  // another session's key is never adopted silently, because sharing an
  // endpoint does not mean sharing an account. This runs before URL
  // overrides, so a key the repair sets aside already counts as held when
  // a link asks to change the session's endpoint (I4).
  const repair = repairStoredApiToken(finalConfig, [globalSyncKey])
  if (repair.outcome !== 'healthy') {
    finalConfig = repair.config
    configChanged = true
    console.warn(
      `[useConfig] API token awaiting reset: session=${sessionId} outcome=${repair.outcome}`,
    )
  }

  let pendingLinkChange: LinkCredentialChange | undefined
  if (options?.applyUrlOverrides) {
    const overrideResult = applyUrlOverridesToConfig(finalConfig)
    if (overrideResult.mutated) {
      const overridden = overrideResult.config
      if (overridden.api_token !== finalConfig.api_token) {
        // A URL is just another write path for the API token, so it obeys
        // the same policy as updateConfig.
        const { accepted, rejections } = validateCredentialUpdate(finalConfig, {
          api_token: overridden.api_token,
        })
        if (rejections.length > 0) {
          console.warn(
            `[useConfig] ignored URL API token override copying sync key: session=${sessionId}`,
          )
        }
        overridden.api_token = accepted.api_token ?? finalConfig.api_token
        if (accepted.api_token_reset_required !== undefined) {
          overridden.api_token_reset_required =
            accepted.api_token_reset_required
        }
      }
      // The server sends a BYOK key, and the session's recent history, to
      // whatever endpoint the client names, so a link must not silently
      // change a stored session's endpoint or key (I4).
      const held = holdLinkCredentialChange(
        finalConfig,
        overridden,
        globalSyncKey,
        { firstVisit: !savedConfig },
      )
      if (held.pending) {
        pendingLinkChange = held.pending
        console.warn(
          `[useConfig] holding URL credential change for confirmation: session=${sessionId} endpoint=${held.pending.apiBase !== undefined} key=${held.pending.apiToken !== undefined}`,
        )
      }
      finalConfig = held.config
      configChanged = true
    }
  }

  if (
    !finalConfig.api_token?.trim() ||
    finalConfig.api_token === 'DEFAULT_PROXY_TOKEN'
  ) {
    const fallbackApiToken = pickFallbackApiToken(
      fallbackConfig,
      finalConfig.api_base,
      globalSyncKey,
    )
    finalConfig.api_token = fallbackApiToken || createFreeTierToken()
    console.debug(
      `[useConfig] filled missing API token: session=${sessionId} source=${fallbackApiToken ? 'fallback-session' : 'generated'} apiTokenKind=${describeApiTokenKind(finalConfig.api_token)}`,
    )
    configChanged = true
  }

  return {
    config: finalConfig,
    savedConfig,
    changed: configChanged,
    pendingLinkChange,
  }
}
