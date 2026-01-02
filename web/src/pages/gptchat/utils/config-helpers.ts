/**
 * Helper functions for chat configuration.
 */
import { kvGet, StorageKeys } from '@/utils/storage'
import { AllModels } from '../models'
import { DefaultSessionConfig, type SessionConfig } from '../types'

export const DEFAULT_SESSION_ID = 1

const UrlConfigBooleanFields = new Set([
  'all_in_one',
  'disable_https_crawler',
  'enable_talk',
  'enable_mcp',
])
const UrlConfigIntegerFields = new Set([
  'max_tokens',
  'n_contexts',
  'draw_n_images',
])
const UrlConfigFloatFields = new Set([
  'temperature',
  'presence_penalty',
  'frequency_penalty',
])
const UrlParamAliasMap = new Map<string, string>([
  ['api_key', 'api_token'],
  ['apikey', 'api_token'],
  ['token', 'api_token'],
  ['api_token', 'api_token'],
  ['api_token_type', 'token_type'],
  ['token_type', 'token_type'],
  ['tokentype', 'token_type'],
  ['api_base', 'api_base'],
  ['base', 'api_base'],
  ['apibase', 'api_base'],
  ['model', 'selected_model'],
  ['chat_model', 'selected_model'],
  ['chatmodel', 'selected_model'],
  ['selected_model', 'selected_model'],
  ['selectedmodel', 'selected_model'],
  ['system_prompt', 'system_prompt'],
  ['prompt', 'system_prompt'],
  ['systemprompt', 'system_prompt'],
  ['max_token', 'max_tokens'],
  ['max_tokens', 'max_tokens'],
  ['maxtoken', 'max_tokens'],
  ['maxtokens', 'max_tokens'],
  ['temperature', 'temperature'],
  ['presence_penalty', 'presence_penalty'],
  ['presencepenalty', 'presence_penalty'],
  ['frequency_penalty', 'frequency_penalty'],
  ['frequencypenalty', 'frequency_penalty'],
  ['context', 'n_contexts'],
  ['contexts', 'n_contexts'],
  ['n_contexts', 'n_contexts'],
  ['context_len', 'n_contexts'],
  ['contextlength', 'n_contexts'],
  ['contextlen', 'n_contexts'],
  ['draw_n_images', 'chat_switch.draw_n_images'],
  ['draw_images', 'chat_switch.draw_n_images'],
  ['drawimages', 'chat_switch.draw_n_images'],
  ['draw', 'chat_switch.draw_n_images'],
  ['enable_mcp', 'chat_switch.enable_mcp'],
  ['enablemcp', 'chat_switch.enable_mcp'],
  ['chat_switch.enable_mcp', 'chat_switch.enable_mcp'],
  ['chat_switch.enablemcp', 'chat_switch.enable_mcp'],
  ['disable_https_crawler', 'chat_switch.disable_https_crawler'],
  ['chat_switch.disable_https_crawler', 'chat_switch.disable_https_crawler'],
  ['disablehttpscrawler', 'chat_switch.disable_https_crawler'],
  ['chat_switch.disablehttpscrawler', 'chat_switch.disable_https_crawler'],
  ['https_crawler', 'chat_switch.disable_https_crawler'],
  ['all_in_one', 'chat_switch.all_in_one'],
  ['allinone', 'chat_switch.all_in_one'],
  ['chat_switch.all_in_one', 'chat_switch.all_in_one'],
  ['enable_talk', 'chat_switch.enable_talk'],
  ['enabletalk', 'chat_switch.enable_talk'],
  ['chat_switch.enable_talk', 'chat_switch.enable_talk'],
  ['chat_switch.enabletalk', 'chat_switch.enable_talk'],
  ['draw_model', 'selected_draw_model'],
  ['imagemodel', 'selected_draw_model'],
  ['selected_draw_model', 'selected_draw_model'],
  ['selected_chat_model', 'selected_chat_model'],
])

function normalizeUrlParamKey(key: string): string {
  return key
    .trim()
    .toLowerCase()
    .replace(/[\s-]+/g, '_')
}

function parseBooleanParamValue(value: unknown): boolean | null {
  if (typeof value === 'boolean') return value
  const normalized = String(value ?? '')
    .trim()
    .toLowerCase()
  if (['1', 'true', 'yes', 'y', 'on'].includes(normalized)) return true
  if (['0', 'false', 'no', 'n', 'off'].includes(normalized)) return false
  return null
}

function parseIntegerParamValue(value: unknown): number | null {
  if (typeof value === 'number' && Number.isInteger(value)) return value
  const str = String(value ?? '').trim()
  if (!/^-?\d+$/.test(str)) return null
  const parsed = parseInt(str, 10)
  return Number.isNaN(parsed) ? null : parsed
}

function parseFloatParamValue(value: unknown): number | null {
  if (typeof value === 'number' && !Number.isNaN(value)) return value
  const str = String(value ?? '').trim()
  if (!/^-?\d+(\.\d+)?$/.test(str)) return null
  const parsed = parseFloat(str)
  return Number.isNaN(parsed) ? null : parsed
}

function getNestedConfigValue(
  config: Record<string, unknown>,
  pathSegments: string[],
) {
  return pathSegments.reduce<unknown>((acc, segment) => {
    if (typeof acc !== 'object' || acc === null) {
      return undefined
    }
    return (acc as Record<string, unknown>)[segment]
  }, config)
}

function setNestedConfigValue(
  config: Record<string, unknown>,
  pathSegments: string[],
  value: unknown,
) {
  let cursor: Record<string, unknown> = config
  for (let i = 0; i < pathSegments.length - 1; i++) {
    const segment = pathSegments[i]
    const next = cursor[segment]
    if (typeof next !== 'object' || next === null) {
      cursor[segment] = {}
    }
    cursor = cursor[segment] as Record<string, unknown>
  }
  cursor[pathSegments[pathSegments.length - 1]] = value
}

function coerceConfigValue(
  field: string,
  rawValue: unknown,
  currentValue: unknown,
) {
  if (UrlConfigBooleanFields.has(field)) {
    const parsed = parseBooleanParamValue(rawValue)
    return parsed === null ? currentValue : parsed
  }
  if (UrlConfigIntegerFields.has(field)) {
    const parsed = parseIntegerParamValue(rawValue)
    return parsed === null ? currentValue : parsed
  }
  if (UrlConfigFloatFields.has(field)) {
    const parsed = parseFloatParamValue(rawValue)
    return parsed === null ? currentValue : parsed
  }
  if (rawValue === undefined || rawValue === null) {
    return currentValue
  }
  return rawValue
}

function deepCloneConfig(config: SessionConfig): SessionConfig {
  if (typeof structuredClone === 'function') {
    return structuredClone(config)
  }
  return JSON.parse(JSON.stringify(config)) as SessionConfig
}

/**
 * Normalize numeric fields in config to ensure they are numbers, not strings
 */
export function normalizeConfigNumericFields(
  config: SessionConfig,
): SessionConfig {
  return {
    ...config,
    max_tokens:
      typeof config.max_tokens === 'number'
        ? config.max_tokens
        : parseInt(String(config.max_tokens), 10) ||
          DefaultSessionConfig.max_tokens,
    n_contexts:
      typeof config.n_contexts === 'number'
        ? config.n_contexts
        : parseInt(String(config.n_contexts), 10) ||
          DefaultSessionConfig.n_contexts,
    temperature:
      typeof config.temperature === 'number'
        ? config.temperature
        : parseFloat(String(config.temperature)) ||
          DefaultSessionConfig.temperature,
    presence_penalty:
      typeof config.presence_penalty === 'number'
        ? config.presence_penalty
        : parseFloat(String(config.presence_penalty)) ||
          DefaultSessionConfig.presence_penalty,
    frequency_penalty:
      typeof config.frequency_penalty === 'number'
        ? config.frequency_penalty
        : parseFloat(String(config.frequency_penalty)) ||
          DefaultSessionConfig.frequency_penalty,
    chat_switch: {
      ...config.chat_switch,
      draw_n_images:
        typeof config.chat_switch?.draw_n_images === 'number'
          ? config.chat_switch.draw_n_images
          : parseInt(String(config.chat_switch?.draw_n_images), 10) ||
            DefaultSessionConfig.chat_switch.draw_n_images,
    },
  }
}

export function applyUrlOverridesToConfig(config: SessionConfig): {
  config: SessionConfig
  mutated: boolean
} {
  const url = new URL(window.location.href)
  const searchParams = url.searchParams
  const entries = Array.from(searchParams.entries())
  let mutated = false

  if (entries.length === 0) {
    return { config, mutated }
  }

  const updatedConfig = deepCloneConfig(config)

  entries.forEach(([rawKey, rawValue]) => {
    const normalizedKey = normalizeUrlParamKey(rawKey)
    const targetPath = UrlParamAliasMap.get(normalizedKey) || normalizedKey
    if (!targetPath) {
      return
    }

    const pathSegments = targetPath.split('.')
    const rootKey = pathSegments[0]
    if (rootKey !== 'chat_switch' && !(rootKey in updatedConfig)) {
      return
    }

    if (rootKey === 'chat_switch') {
      if (
        !updatedConfig.chat_switch ||
        typeof updatedConfig.chat_switch !== 'object'
      ) {
        updatedConfig.chat_switch = { ...DefaultSessionConfig.chat_switch }
      }
    }

    const currentValue = getNestedConfigValue(
      updatedConfig as unknown as Record<string, unknown>,
      pathSegments,
    )
    const coercedValue = coerceConfigValue(
      pathSegments[pathSegments.length - 1],
      rawValue,
      currentValue,
    )
    if (coercedValue === currentValue) {
      searchParams.delete(rawKey)
      return
    }

    setNestedConfigValue(
      updatedConfig as unknown as Record<string, unknown>,
      pathSegments,
      coercedValue,
    )
    if (
      targetPath === 'selected_model' &&
      typeof coercedValue === 'string' &&
      coercedValue &&
      !AllModels.includes(coercedValue)
    ) {
      AllModels.push(coercedValue)
    }

    mutated = true
    searchParams.delete(rawKey)
  })

  if (mutated) {
    const newSearch = searchParams.toString()
    window.history.replaceState(
      {},
      document.title,
      `${url.pathname}${newSearch ? `?${newSearch}` : ''}${url.hash}`,
    )
  }

  return {
    config: mutated ? updatedConfig : config,
    mutated,
  }
}

/**
 * Get the active session ID
 */
export async function getActiveSessionId(): Promise<number> {
  const selectedSession = await kvGet<number | string>(
    StorageKeys.SELECTED_SESSION,
  )

  if (selectedSession === null || selectedSession === undefined) {
    return DEFAULT_SESSION_ID
  }

  const parsed =
    typeof selectedSession === 'number'
      ? selectedSession
      : parseInt(selectedSession, 10)

  return isNaN(parsed) ? DEFAULT_SESSION_ID : parsed
}

/**
 * Get session config key for a session ID
 */
export function getSessionConfigKey(sessionId: number): string {
  return `${StorageKeys.SESSION_CONFIG_PREFIX}${sessionId}`
}
