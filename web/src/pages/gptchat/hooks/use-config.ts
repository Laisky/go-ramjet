/**
 * React hook for managing chat configuration.
 */
import { useCallback, useEffect, useState } from 'react'

import { kvDel, kvGet, kvList, kvSet, StorageKeys } from '@/utils/storage'
import {
  AllModels,
  DefaultModel,
  ImageModelFluxDev,
  isImageModel,
} from '../models'
import {
  DefaultSessionConfig,
  type ChatMessageData,
  type SessionConfig,
  type SessionHistoryItem,
} from '../types'
import {
  generateChatId,
  getChatDataKey,
  getSessionHistoryKey,
} from '../utils/chat-storage'

const DEFAULT_SESSION_ID = 1

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
function normalizeConfigNumericFields(config: SessionConfig): SessionConfig {
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

function applyUrlOverridesToConfig(config: SessionConfig): {
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
async function getActiveSessionId(): Promise<number> {
  const selectedSession = await kvGet<number>(StorageKeys.SELECTED_SESSION)
  return selectedSession ?? DEFAULT_SESSION_ID
}

/**
 * Get session config key for a session ID
 */
function getSessionConfigKey(sessionId: number): string {
  return `${StorageKeys.SESSION_CONFIG_PREFIX}${sessionId}`
}

/**
 * Hook for managing session configuration
 */
export function useConfig() {
  const [config, setConfigState] = useState<SessionConfig>(DefaultSessionConfig)
  const [sessionId, setSessionId] = useState<number>(DEFAULT_SESSION_ID)
  const [sessions, setSessions] = useState<{ id: number; name: string }[]>([])
  const [isLoading, setIsLoading] = useState(true)

  /**
   * Load all available sessions
   */
  const loadSessions = useCallback(async () => {
    const keys = await kvList()
    const configKeys = keys.filter((k) =>
      k.startsWith(StorageKeys.SESSION_CONFIG_PREFIX),
    )

    // Sort keys by ID to keep order stable
    configKeys.sort((a, b) => {
      const idA = parseInt(a.replace(StorageKeys.SESSION_CONFIG_PREFIX, ''), 10)
      const idB = parseInt(b.replace(StorageKeys.SESSION_CONFIG_PREFIX, ''), 10)
      return idA - idB
    })

    const loadedSessions: { id: number; name: string }[] = []

    for (const key of configKeys) {
      try {
        const id = parseInt(
          key.replace(StorageKeys.SESSION_CONFIG_PREFIX, ''),
          10,
        )
        if (isNaN(id)) continue

        const conf = await kvGet<SessionConfig>(key)
        loadedSessions.push({
          id,
          name: conf?.session_name || `Chat Session ${id}`,
        })
      } catch (e) {
        console.error('Failed to load session config for key', key, e)
      }
    }

    // If no sessions found, at least include current default (should exist after load logic)
    if (loadedSessions.length === 0) {
      // We defer to what loadConfig created
    }

    setSessions(loadedSessions)
  }, [])

  // Load configuration on mount
  useEffect(() => {
    const loadConfig = async () => {
      try {
        // Run migration first
        const { migrateLegacyData } =
          await import('@/pages/gptchat/utils/migration')
        await migrateLegacyData()

        const activeSessionId = await getActiveSessionId()
        const key = getSessionConfigKey(activeSessionId)
        const savedConfig = await kvGet<SessionConfig>(key)

        let finalConfig = {
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

        let configChanged = false

        // Seed split chat/draw model selectors while keeping backwards compatibility
        if (!finalConfig.selected_chat_model) {
          finalConfig.selected_chat_model = isImageModel(
            finalConfig.selected_model,
          )
            ? DefaultModel
            : finalConfig.selected_model || DefaultModel
          configChanged = true
        }

        if (!finalConfig.selected_draw_model) {
          finalConfig.selected_draw_model = isImageModel(
            finalConfig.selected_model,
          )
            ? finalConfig.selected_model
            : ImageModelFluxDev
          configChanged = true
        }

        if (!finalConfig.selected_model) {
          finalConfig.selected_model = finalConfig.selected_chat_model
          configChanged = true
        }

        // Generate sync_key if missing
        if (!finalConfig.sync_key) {
          finalConfig.sync_key =
            'sync-' +
            Math.random().toString(36).substring(2, 15) +
            Math.random().toString(36).substring(2, 15)
          configChanged = true
        }

        // Auto-generate FREETIER token if missing
        if (
          !finalConfig.api_token ||
          finalConfig.api_token === 'DEFAULT_PROXY_TOKEN'
        ) {
          const randomStr =
            Math.random().toString(36).substring(2, 18) +
            Math.random().toString(36).substring(2, 18)
          finalConfig.api_token = `FREETIER-${randomStr}`
          console.debug('Generated new FREETIER token:', finalConfig.api_token)
          configChanged = true
        }

        // Ensure api_base is set
        if (!finalConfig.api_base) {
          finalConfig.api_base = 'https://api.openai.com'
          configChanged = true
        }

        // Ensure session_name is set
        if (!finalConfig.session_name) {
          finalConfig.session_name = `Chat Session ${activeSessionId}`
          configChanged = true
        }

        const overrideResult = applyUrlOverridesToConfig(finalConfig)
        if (overrideResult.mutated) {
          finalConfig = overrideResult.config
          configChanged = true
        }

        if (configChanged || !savedConfig || overrideResult.mutated) {
          await kvSet(key, finalConfig)
        }

        setConfigState(finalConfig)
        setSessionId(activeSessionId)

        // Load all sessions
        await loadSessions()
      } catch (error) {
        console.error('Failed to load config:', error)
      } finally {
        setIsLoading(false)
      }
    }

    loadConfig()
  }, [loadSessions])

  /**
   * Apply URL parameter overrides to the current config
   */

  /**
   * Update and persist configuration
   */
  const updateConfig = useCallback(
    async (updates: Partial<SessionConfig>) => {
      const newConfig = {
        ...config,
        ...updates,
        chat_switch: {
          ...config.chat_switch,
          ...(updates.chat_switch || {}),
        },
      }

      setConfigState(newConfig)

      try {
        const key = getSessionConfigKey(sessionId)
        await kvSet(key, newConfig)
      } catch (error) {
        console.error('Failed to save config:', error)
      }
    },
    [config, sessionId, loadSessions],
  )

  /**
   * Switch to a different session
   */
  const switchSession = useCallback(
    async (newSessionId: number) => {
      setIsLoading(true)
      try {
        await kvSet(StorageKeys.SELECTED_SESSION, newSessionId)
        setSessionId(newSessionId)

        const key = getSessionConfigKey(newSessionId)
        const savedConfig = await kvGet<SessionConfig>(key)

        if (savedConfig) {
          setConfigState(
            normalizeConfigNumericFields({
              ...DefaultSessionConfig,
              ...savedConfig,
              //   session_name: savedConfig.session_name || `Chat Session ${newSessionId}`, // Ensure name exists
              chat_switch: {
                ...DefaultSessionConfig.chat_switch,
                ...savedConfig.chat_switch,
              },
            }),
          )
        } else {
          const newConf = {
            ...DefaultSessionConfig,
            session_name: `Chat Session ${newSessionId}`,
          }
          setConfigState(newConf)
          // Persist if switching to a non-existent session (should rarely happen via UI unless creating)
          await kvSet(key, newConf)
        }

        // Refresh list to ensure names are up to date if we defaulted above
        await loadSessions()
      } finally {
        setIsLoading(false)
      }
    },
    [loadSessions],
  )

  /**
   * Create a new session
   */
  const createSession = useCallback(
    async (name?: string): Promise<number> => {
      // Find the next available session ID
      const keys = await kvList()
      const sessionIds = keys
        .filter((k) => k.startsWith(StorageKeys.SESSION_CONFIG_PREFIX))
        .map((k) =>
          parseInt(k.replace(StorageKeys.SESSION_CONFIG_PREFIX, ''), 10),
        )
        .filter((id) => !isNaN(id))

      const maxId = sessionIds.length > 0 ? Math.max(...sessionIds) : 0
      const newId = maxId + 1

      // Initialize new session with defaults
      const key = getSessionConfigKey(newId)
      const newConfig = {
        ...DefaultSessionConfig,
        session_name: name || `Chat Session ${newId}`,
      }
      await kvSet(key, newConfig)

      await loadSessions()

      return newId
    },
    [loadSessions],
  )

  /**
   * Delete a session
   */
  const deleteSession = useCallback(
    async (targetSessionId: number) => {
      // Delete session config
      await kvDel(getSessionConfigKey(targetSessionId))

      const historyKey = getSessionHistoryKey(targetSessionId)
      const history = await kvGet<SessionHistoryItem[]>(historyKey)
      if (history) {
        const uniqueChatIds = Array.from(
          new Set(history.map((item) => item.chatID).filter(Boolean)),
        )
        for (const chatId of uniqueChatIds) {
          await kvDel(getChatDataKey(chatId!, 'user'))
          await kvDel(getChatDataKey(chatId!, 'assistant'))
        }
      }

      // Delete session history
      await kvDel(historyKey)

      // If deleting current session, switch to session 1
      // If deleting current session, switch to session 1 or first available
      if (targetSessionId === sessionId) {
        // If we have other sessions, switch to first one. Else, re-create session 1.
        // We need the updated list. Filtering `sessions` state might be stale if inside callback?
        // We can fetch list. But let's just default to 1 for simplicity,
        // createSession will handle if 1 is deleted by recreating it.
        // Actually if 1 is deleted, we might want to find another existing one.
        // Let's rely on switchSession(1) which creates it if missing.
        await switchSession(DEFAULT_SESSION_ID)
      }

      await loadSessions()
    },
    [sessionId, switchSession, loadSessions],
  )

  const duplicateSession = useCallback(
    async (sourceSessionId: number) => {
      const sessionKeys = await kvList()
      const sessionIds = sessionKeys
        .filter((k) => k.startsWith(StorageKeys.SESSION_CONFIG_PREFIX))
        .map((k) =>
          parseInt(k.replace(StorageKeys.SESSION_CONFIG_PREFIX, ''), 10),
        )
        .filter((id) => !Number.isNaN(id))

      const newSessionId =
        sessionIds.length > 0 ? Math.max(...sessionIds) + 1 : DEFAULT_SESSION_ID

      const sourceConfigKey = getSessionConfigKey(sourceSessionId)
      const sourceConfig =
        (await kvGet<SessionConfig>(sourceConfigKey)) || DefaultSessionConfig
      const duplicatedConfig: SessionConfig = {
        ...sourceConfig,
        session_name: `${sourceConfig.session_name || `Chat Session ${sourceSessionId}`} Copy`,
      }

      await kvSet(getSessionConfigKey(newSessionId), duplicatedConfig)

      const sourceHistory =
        (await kvGet<SessionHistoryItem[]>(
          getSessionHistoryKey(sourceSessionId),
        )) || []
      const historyKey = getSessionHistoryKey(newSessionId)
      const idMap = new Map<string, string>()
      const duplicatedHistory: SessionHistoryItem[] = []

      for (const item of sourceHistory) {
        if (!item.chatID) {
          continue
        }

        let nextChatId = idMap.get(item.chatID)
        if (!nextChatId) {
          nextChatId = generateChatId()
          idMap.set(item.chatID, nextChatId)
        }

        duplicatedHistory.push({ ...item, chatID: nextChatId })

        const chatData = await kvGet<ChatMessageData>(
          getChatDataKey(item.chatID, item.role),
        )
        if (chatData) {
          await kvSet(getChatDataKey(nextChatId, item.role), {
            ...chatData,
            chatID: nextChatId,
          })
        }
      }

      await kvSet(historyKey, duplicatedHistory)
      await loadSessions()
      return newSessionId
    },
    [loadSessions],
  )

  const purgeAllSessions = useCallback(async () => {
    const keys = await kvList()
    for (const key of keys) {
      if (
        key.startsWith(StorageKeys.SESSION_CONFIG_PREFIX) ||
        key.startsWith(StorageKeys.SESSION_HISTORY_PREFIX) ||
        key.startsWith(StorageKeys.CHAT_DATA_PREFIX)
      ) {
        await kvDel(key)
      }
    }

    const preservedToken = config.api_token
    const preservedBase = config.api_base
    const newConfig: SessionConfig = {
      ...DefaultSessionConfig,
      api_token: preservedToken,
      api_base: preservedBase,
      session_name: DefaultSessionConfig.session_name,
    }

    await kvSet(getSessionConfigKey(DEFAULT_SESSION_ID), newConfig)
    await kvSet(getSessionHistoryKey(DEFAULT_SESSION_ID), [])
    await kvSet(StorageKeys.SELECTED_SESSION, DEFAULT_SESSION_ID)

    setSessionId(DEFAULT_SESSION_ID)
    setConfigState(newConfig)
    setSessions([
      {
        id: DEFAULT_SESSION_ID,
        name: newConfig.session_name || `Chat Session ${DEFAULT_SESSION_ID}`,
      },
    ])
    await loadSessions()
  }, [config.api_base, config.api_token, loadSessions])

  /**
   * Rename a session
   */
  const renameSession = useCallback(
    async (targetId: number, newName: string) => {
      const key = getSessionConfigKey(targetId)
      // We must read it first to preserve other fields
      let conf = await kvGet<SessionConfig>(key)
      if (!conf) {
        conf = { ...DefaultSessionConfig }
      }

      conf.session_name = newName
      await kvSet(key, conf)

      // If it's the current session, update state too
      if (targetId === sessionId) {
        setConfigState(conf)
      }

      await loadSessions()
    },
    [sessionId, loadSessions],
  )

  /**
   * Export all data (sessions, configs, shortcuts) for sync
   */
  const exportAllData = useCallback(async () => {
    const { kvList, kvGet } = await import('@/utils/storage')
    const keys = await kvList()
    const data: Record<string, unknown> = {}

    // keys to exclude
    const excludeKeys = ['MIGRATE_V1_COMPLETED']

    for (const key of keys) {
      if (excludeKeys.includes(key)) continue
      data[key] = await kvGet(key)
    }

    return data
  }, [])

  /**
   * Import data (overwrite existing)
   */
  const importAllData = useCallback(
    async (data: Record<string, unknown>) => {
      const { kvSet } = await import('@/utils/storage')

      // Clear existing data to ensure clean state (optional, but safer for full sync)
      // await kvClear() // Maybe too aggressive? Let's just overwrite.

      for (const [key, val] of Object.entries(data)) {
        await kvSet(key, val)
      }

      // Force reload config if current session was updated
      const currentKey = getSessionConfigKey(sessionId)
      if (data[currentKey]) {
        // Simple way: trigger reload by toggling a dummy state or just re-running load
        window.location.reload() // Easiest way to ensure all states (history, config) are updated
      }
    },
    [sessionId],
  )

  return {
    config,
    sessionId,
    sessions,
    isLoading,
    updateConfig,
    switchSession,
    createSession,
    deleteSession,
    renameSession,
    duplicateSession,
    purgeAllSessions,
    exportAllData,
    importAllData,
  }
}
