/**
 * Helper functions for chat configuration.
 */
import { kvGet, StorageKeys } from '@/utils/storage'
import {
  AllModels,
  DefaultModel,
  ImageModelFluxDev,
  isRetiredResearchModel,
} from '../models'
import { DefaultSessionConfig, type SessionConfig } from '../types'
import { isValidApiBase } from './api-token'

export const DEFAULT_SESSION_ID = 1

const UrlConfigBooleanFields = new Set([
  'all_in_one',
  'disable_https_crawler',
  'enable_talk',
  'enable_mcp',
  'enable_memory',
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
  ['enable_memory', 'chat_switch.enable_memory'],
  ['enablememory', 'chat_switch.enable_memory'],
  ['chat_switch.enable_memory', 'chat_switch.enable_memory'],
  ['chat_switch.enablememory', 'chat_switch.enable_memory'],
  ['draw_model', 'selected_draw_model'],
  ['imagemodel', 'selected_draw_model'],
  ['selected_draw_model', 'selected_draw_model'],
  ['selected_chat_model', 'selected_chat_model'],
])

/**
 * UrlProtectedConfigKeys lists config fields a URL may never set. A link is
 * untrusted input: the sync key decides where the cloud backup is uploaded
 * and who can decrypt it, MCP servers carry their own endpoints and
 * credentials, and the remaining fields are internal state.
 */
const UrlProtectedConfigKeys = new Set([
  'sync_key',
  'mcp_servers',
  'api_token_reset_required',
  'updated_at',
])

/**
 * UrlForbiddenPathSegments lists path segments that reach an object's
 * prototype chain. A URL path containing any of them is always rejected.
 */
const UrlForbiddenPathSegments = new Set([
  '__proto__',
  'prototype',
  'constructor',
])

/**
 * UrlConfigTarget is one config field a URL may set: a top-level field of
 * SessionConfig, or a field of SessionConfig.chat_switch when parent is set.
 * defaultValue is the field's default, whose type the URL value must match.
 */
interface UrlConfigTarget {
  parent?: 'chat_switch'
  field: string
  defaultValue: unknown
}

/**
 * isUrlScalarDefault reports whether value is a primitive a URL string can
 * stand for. It takes a field's default value and returns false for objects,
 * arrays, null and undefined, which a URL must never replace.
 */
function isUrlScalarDefault(value: unknown): boolean {
  return (
    typeof value === 'string' ||
    typeof value === 'number' ||
    typeof value === 'boolean'
  )
}

/**
 * buildUrlSettableConfigPaths returns the explicit allowlist of config paths
 * a URL may set, keyed by dotted path. It contains the own scalar fields of
 * DefaultSessionConfig that are not in UrlProtectedConfigKeys, plus
 * "chat_switch.<field>" for each own scalar field of the default chat_switch.
 * Fields absent from DefaultSessionConfig are never URL-settable.
 */
function buildUrlSettableConfigPaths(): ReadonlyMap<string, UrlConfigTarget> {
  const paths = new Map<string, UrlConfigTarget>()
  for (const [field, value] of Object.entries(DefaultSessionConfig)) {
    if (
      UrlProtectedConfigKeys.has(field) ||
      UrlForbiddenPathSegments.has(field) ||
      !isUrlScalarDefault(value)
    ) {
      continue
    }
    paths.set(field, { field, defaultValue: value })
  }
  for (const [field, value] of Object.entries(
    DefaultSessionConfig.chat_switch,
  )) {
    if (UrlForbiddenPathSegments.has(field) || !isUrlScalarDefault(value)) {
      continue
    }
    paths.set(`chat_switch.${field}`, {
      parent: 'chat_switch',
      field,
      defaultValue: value,
    })
  }
  return paths
}

/** UrlSettableConfigPaths is the allowlist built by buildUrlSettableConfigPaths. */
const UrlSettableConfigPaths = buildUrlSettableConfigPaths()

/**
 * readOwnField returns record[field] when field is an own property of record,
 * and undefined otherwise, so inherited prototype members are never read.
 */
function readOwnField(record: Record<string, unknown>, field: string): unknown {
  return Object.hasOwn(record, field) ? record[field] : undefined
}

/**
 * resolveUrlTargetRecord returns the object that owns target's field inside
 * config. For a chat_switch field it first replaces a missing or malformed
 * chat_switch with a copy of the defaults. config is mutated in that case.
 */
function resolveUrlTargetRecord(
  config: SessionConfig,
  target: UrlConfigTarget,
): Record<string, unknown> {
  const root = config as unknown as Record<string, unknown>
  if (!target.parent) {
    return root
  }
  const parent = readOwnField(root, target.parent)
  if (typeof parent !== 'object' || parent === null || Array.isArray(parent)) {
    config.chat_switch = { ...DefaultSessionConfig.chat_switch }
  }
  return config.chat_switch as unknown as Record<string, unknown>
}

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

/**
 * coerceConfigValue converts the URL value rawValue for field to the type
 * the field holds, judged by the field name and by defaultValue's type (a
 * boolean field never stores the string "0"). It returns currentValue when
 * rawValue does not parse.
 */
function coerceConfigValue(
  field: string,
  rawValue: unknown,
  currentValue: unknown,
  defaultValue?: unknown,
) {
  if (UrlConfigBooleanFields.has(field) || typeof defaultValue === 'boolean') {
    const parsed = parseBooleanParamValue(rawValue)
    return parsed === null ? currentValue : parsed
  }
  if (UrlConfigIntegerFields.has(field)) {
    // Support "all" for n_contexts (maps to 31, meaning all history)
    if (
      field === 'n_contexts' &&
      String(rawValue ?? '')
        .trim()
        .toLowerCase() === 'all'
    ) {
      return 31
    }
    const parsed = parseIntegerParamValue(rawValue)
    return parsed === null ? currentValue : parsed
  }
  if (UrlConfigFloatFields.has(field) || typeof defaultValue === 'number') {
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
 * normalizeRetiredResearchConfig returns a config with retired selections replaced
 * by the default chat or image model, preserving all other settings and history.
 */
export function normalizeRetiredResearchConfig(
  config: SessionConfig,
): SessionConfig {
  if (
    ![
      config.selected_model,
      config.selected_chat_model,
      config.selected_draw_model,
    ].some(isRetiredResearchModel)
  ) {
    return config
  }
  return {
    ...config,
    selected_model: isRetiredResearchModel(config.selected_model)
      ? DefaultModel
      : config.selected_model,
    selected_chat_model: isRetiredResearchModel(config.selected_chat_model)
      ? DefaultModel
      : config.selected_chat_model,
    selected_draw_model: isRetiredResearchModel(config.selected_draw_model)
      ? ImageModelFluxDev
      : config.selected_draw_model,
  }
}

/**
 * Normalize numeric fields in config to ensure they are numbers, not strings
 */
export function normalizeConfigNumericFields(
  config: SessionConfig,
): SessionConfig {
  return {
    ...normalizeRetiredResearchConfig(config),
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

/**
 * applyUrlOverridesToConfig applies the page URL's query parameters to config
 * and removes every consumed or protected parameter from the address bar.
 * Only paths in UrlSettableConfigPaths are set; fields in
 * UrlProtectedConfigKeys, and an api_base the server would reject (see
 * isValidApiBase), are stripped from the URL without being applied, and
 * every other parameter is left untouched. config itself is never mutated.
 * It returns the resulting config and whether any field changed.
 */
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
  let strippedParams = false

  entries.forEach(([rawKey, rawValue]) => {
    const normalizedKey = normalizeUrlParamKey(rawKey)
    const targetPath = UrlParamAliasMap.get(normalizedKey) || normalizedKey
    if (!targetPath) {
      return
    }

    const pathSegments = targetPath.split('.')
    const rootKey = pathSegments[0]
    if (UrlProtectedConfigKeys.has(rootKey)) {
      // Drop it from the address bar too, so the value is not kept in
      // history or bookmarks.
      console.warn(`[config] ignored protected URL parameter: ${rootKey}`)
      searchParams.delete(rawKey)
      strippedParams = true
      return
    }
    if (pathSegments.some((segment) => UrlForbiddenPathSegments.has(segment))) {
      console.warn(`[config] ignored unsafe URL parameter path: ${targetPath}`)
      return
    }
    const target = UrlSettableConfigPaths.get(targetPath)
    if (!target) {
      // Not a URL-settable config field; other features may read it.
      return
    }

    if (targetPath === 'api_base' && !isValidApiBase(rawValue)) {
      // The server would ignore it and realtime audio would fail on it.
      console.warn('[config] ignored invalid URL parameter: api_base')
      searchParams.delete(rawKey)
      strippedParams = true
      return
    }

    const record = resolveUrlTargetRecord(updatedConfig, target)
    const currentValue = readOwnField(record, target.field)
    const coercedValue = coerceConfigValue(
      target.field,
      rawValue,
      currentValue,
      target.defaultValue,
    )
    if (coercedValue === currentValue) {
      searchParams.delete(rawKey)
      return
    }

    record[target.field] = coercedValue
    if (
      targetPath === 'selected_model' &&
      typeof coercedValue === 'string' &&
      coercedValue &&
      !isRetiredResearchModel(coercedValue) &&
      !AllModels.includes(coercedValue)
    ) {
      AllModels.push(coercedValue)
    }

    mutated = true
    searchParams.delete(rawKey)
  })

  if (mutated || strippedParams) {
    const newSearch = searchParams.toString()
    window.history.replaceState(
      {},
      document.title,
      `${url.pathname}${newSearch ? `?${newSearch}` : ''}${url.hash}`,
    )
  }

  return {
    config: normalizeRetiredResearchConfig(mutated ? updatedConfig : config),
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
