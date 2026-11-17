/**
 * Session config persistence helpers for GPTChat: session ordering and the
 * read-normalize-repair pipeline that turns a stored config into one that is
 * safe to use.
 */
import { kvGet, kvList, kvSet, StorageKeys } from '@/utils/storage'
import { DefaultModel, ImageModelFluxDev, isImageModel } from '../models'
import { DefaultSessionConfig, type SessionConfig } from '../types'
import {
  createFreeTierToken,
  createSyncKey,
  describeApiTokenKind,
  findRecoverableApiToken,
  hasUsableApiToken,
  isApiTokenClobberedBySyncKey,
} from '../utils/api-token'
import {
  applyUrlOverridesToConfig,
  getSessionConfigKey,
  normalizeConfigNumericFields,
} from '../utils/config-helpers'

/**
 * getSessionOrderFromKv reads the persisted session display order.
 * It accepts both the legacy bare-array shape and the timestamped object shape,
 * and returns the ordered session IDs (empty when nothing is stored).
 */
export async function getSessionOrderFromKv(): Promise<number[]> {
  const raw = await kvGet<number[] | { data: number[]; updated_at: number }>(
    StorageKeys.SESSION_ORDER,
  )
  return Array.isArray(raw) ? raw : raw?.data || []
}

/**
 * setSessionOrderToKv persists the session display order given by order,
 * stamping it with the current time so sync can pick the newest copy.
 */
export async function setSessionOrderToKv(order: number[]): Promise<void> {
  await kvSet(StorageKeys.SESSION_ORDER, {
    data: order,
    updated_at: Date.now(),
  })
}

/**
 * recoverApiTokenFromSiblingSessions scans every other session config for a
 * usable API token, so a session whose token was clobbered by the sync key can
 * be restored instead of silently downgraded to a free-tier token.
 * It returns the recovered token, or an empty string when none qualifies.
 */
async function recoverApiTokenFromSiblingSessions(
  sessionId: number,
  syncKey: string,
): Promise<string> {
  const ownKey = getSessionConfigKey(sessionId)
  const siblingKeys = (await kvList()).filter(
    (k) => k.startsWith(StorageKeys.SESSION_CONFIG_PREFIX) && k !== ownKey,
  )
  const siblings = await Promise.all(
    siblingKeys.map((k) => kvGet<SessionConfig>(k)),
  )
  return findRecoverableApiToken(siblings, syncKey)
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
 * sessionId selects the stored config; options.fallbackConfig supplies values
 * (API token, API base) for a missing or corrupted config, and
 * options.applyUrlOverrides applies URL query overrides last. It returns the
 * hydrated result without persisting it.
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

  let globalSyncKey = await kvGet<string>(StorageKeys.SYNC_KEY)
  if (!globalSyncKey) {
    if (finalConfig.sync_key) {
      globalSyncKey = finalConfig.sync_key
    } else if (fallbackConfig?.sync_key) {
      globalSyncKey = fallbackConfig.sync_key
    } else {
      globalSyncKey = createSyncKey()
    }
    await kvSet(StorageKeys.SYNC_KEY, globalSyncKey)
  }
  finalConfig.sync_key = globalSyncKey

  // Repair sessions whose API token was overwritten by the sync key (browser
  // password-manager autofill in the settings sidebar). Prefer a token from a
  // sibling session. A session still awaiting a reset retries on every load,
  // because another session may have gained a usable token since.
  const apiTokenClobbered = isApiTokenClobberedBySyncKey(
    finalConfig.api_token,
    globalSyncKey,
  )
  const awaitingApiTokenReset = finalConfig.api_token_reset_required === true
  if (
    apiTokenClobbered ||
    (awaitingApiTokenReset &&
      !hasUsableApiToken(finalConfig.api_token, globalSyncKey))
  ) {
    const recoveredApiToken = await recoverApiTokenFromSiblingSessions(
      sessionId,
      globalSyncKey,
    )
    if (recoveredApiToken) {
      finalConfig.api_token = recoveredApiToken
      configChanged = true
    } else if (apiTokenClobbered) {
      // Never send a sync key as a bearer token; the missing-token path below
      // installs a free-tier token so the session keeps working.
      finalConfig.api_token = ''
      configChanged = true
    }
    console.warn(
      `[useConfig] API token clobbered by sync key: session=${sessionId} recovered=${recoveredApiToken !== ''}`,
    )
  }

  const fallbackApiToken = isApiTokenClobberedBySyncKey(
    fallbackConfig?.api_token,
    globalSyncKey,
  )
    ? ''
    : fallbackConfig?.api_token?.trim() || ''
  if (
    !finalConfig.api_token ||
    finalConfig.api_token === 'DEFAULT_PROXY_TOKEN'
  ) {
    if (fallbackApiToken && fallbackApiToken !== 'DEFAULT_PROXY_TOKEN') {
      finalConfig.api_token = fallbackApiToken
      console.debug('[useConfig] preserved fallback API token for session', {
        sessionId,
        apiTokenKind: describeApiTokenKind(finalConfig.api_token),
      })
    } else {
      finalConfig.api_token = createFreeTierToken()
      console.debug('[useConfig] generated FREETIER token for session', {
        sessionId,
        apiTokenKind: describeApiTokenKind(finalConfig.api_token),
      })
    }
    configChanged = true
  }

  if (!finalConfig.api_base) {
    finalConfig.api_base =
      fallbackConfig?.api_base || DefaultSessionConfig.api_base
    configChanged = true
  }

  if (!finalConfig.session_name) {
    finalConfig.session_name = `Chat Session ${sessionId}`
    configChanged = true
  }

  if (options?.applyUrlOverrides) {
    const overrideResult = applyUrlOverridesToConfig(finalConfig)
    if (overrideResult.mutated) {
      finalConfig = overrideResult.config
      configChanged = true
    }
  }

  // Keep alerting until the session holds a user-provided token again.
  const apiTokenResetRequired =
    (apiTokenClobbered || awaitingApiTokenReset) &&
    !hasUsableApiToken(finalConfig.api_token, globalSyncKey)
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
