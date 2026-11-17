/**
 * Session config hydration for GPTChat: the read-normalize-repair pipeline
 * that turns a stored config into one that is safe to use.
 *
 * Pipeline order matters. Model migrations and the sync key come first; the
 * API endpoint is settled next because every credential decision is scoped
 * to it; URL overrides are applied as guarded writes; only then is the API
 * token invariant enforced, so no write path can bypass it.
 */
import { kvGet, kvList, kvSet, StorageKeys } from '@/utils/storage'
import { DefaultModel, ImageModelFluxDev, isImageModel } from '../models'
import { DefaultSessionConfig, type SessionConfig } from '../types'
import {
  type ApiTokenRecovery,
  createFreeTierToken,
  createSyncKey,
  describeApiTokenKind,
  findRecoverableApiToken,
  isApiTokenClobberedBySyncKey,
  isSameApiEndpoint,
  isUserSuppliedApiToken,
} from '../utils/api-token'
import {
  applyUrlOverridesToConfig,
  getSessionConfigKey,
  normalizeConfigNumericFields,
} from '../utils/config-helpers'

/**
 * recoverApiTokenFromSiblingSessions looks for the credential that
 * sessionId lost to the sync key among every other stored session config,
 * scoped to target.apiBase and target.syncKey (see findRecoverableApiToken).
 * It returns the recovery outcome.
 */
async function recoverApiTokenFromSiblingSessions(
  sessionId: number,
  target: { apiBase: string; syncKey: string },
): Promise<ApiTokenRecovery> {
  const ownKey = getSessionConfigKey(sessionId)
  const siblingKeys = (await kvList()).filter(
    (k) => k.startsWith(StorageKeys.SESSION_CONFIG_PREFIX) && k !== ownKey,
  )
  const siblings = await Promise.all(
    siblingKeys.map((k) => kvGet<SessionConfig>(k)),
  )
  return findRecoverableApiToken(siblings, target)
}

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
 */
export interface HydratedSessionConfigResult {
  config: SessionConfig
  savedConfig: SessionConfig | null
  changed: boolean
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
 * options.applyUrlOverrides applies URL query overrides as guarded writes.
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

  console.debug(`[useConfig] hydrate session ${sessionId} config`, {
    hasSavedConfig: !!savedConfig,
    savedApiTokenKind: describeApiTokenKind(savedConfig?.api_token),
    fallbackApiTokenKind: describeApiTokenKind(fallbackConfig?.api_token),
    legacySelectedModel: (savedConfig as Record<string, unknown> | null)
      ?.selected_model,
  })

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

  const globalSyncKey = await resolveGlobalSyncKey(finalConfig, fallbackConfig)
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

  let urlSuppliedApiToken = false
  if (options?.applyUrlOverrides) {
    const overrideResult = applyUrlOverridesToConfig(finalConfig)
    if (overrideResult.mutated) {
      const overridden = overrideResult.config
      if (overridden.api_token !== finalConfig.api_token) {
        if (isApiTokenClobberedBySyncKey(overridden.api_token, globalSyncKey)) {
          // A URL is just another write path: it may not install a sync key
          // as the API token, exactly like updateConfig.
          console.warn(
            `[useConfig] ignored URL API token override copying sync key: session=${sessionId}`,
          )
          overridden.api_token = finalConfig.api_token
        } else {
          urlSuppliedApiToken = isUserSuppliedApiToken(
            overridden.api_token,
            globalSyncKey,
          )
        }
      }
      finalConfig = overridden
      configChanged = true
    }
  }

  // Repair sessions whose API token was overwritten by the sync key (browser
  // password-manager autofill in the settings sidebar). A session awaiting a
  // reset retries on every load while it still has no key of the user's own,
  // because another session may have gained one since; a key the user has
  // started entering is never replaced.
  const awaitingApiTokenReset = finalConfig.api_token_reset_required === true
  const apiTokenClobbered = isApiTokenClobberedBySyncKey(
    finalConfig.api_token,
    globalSyncKey,
  )
  let apiTokenResetRequired = awaitingApiTokenReset
  if (
    apiTokenClobbered ||
    (awaitingApiTokenReset &&
      !isUserSuppliedApiToken(finalConfig.api_token, globalSyncKey))
  ) {
    const recovery = await recoverApiTokenFromSiblingSessions(sessionId, {
      apiBase: finalConfig.api_base,
      syncKey: globalSyncKey,
    })
    if (recovery.outcome === 'recovered') {
      finalConfig.api_token = recovery.token
      apiTokenResetRequired = false
      configChanged = true
    } else {
      if (apiTokenClobbered) {
        // Never send a sync key as a bearer token, and never borrow another
        // session's credential here: a free-tier token keeps the session
        // working while the user re-enters the key.
        finalConfig.api_token = createFreeTierToken()
        configChanged = true
      }
      apiTokenResetRequired = true
    }
    console.warn(
      `[useConfig] API token awaiting reset: session=${sessionId} clobbered=${apiTokenClobbered} recovery=${recovery.outcome} candidates=${recovery.candidates}`,
    )
  }
  if (urlSuppliedApiToken) {
    apiTokenResetRequired = false
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
    console.debug('[useConfig] filled missing API token for session', {
      sessionId,
      source: fallbackApiToken ? 'fallback-session' : 'generated',
      apiTokenKind: describeApiTokenKind(finalConfig.api_token),
    })
    configChanged = true
  }

  if (apiTokenResetRequired !== awaitingApiTokenReset) {
    finalConfig.api_token_reset_required = apiTokenResetRequired
    configChanged = true
  }

  return {
    config: finalConfig,
    savedConfig,
    changed: configChanged,
  }
}
