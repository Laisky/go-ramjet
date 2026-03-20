/**
 * React hook for managing chat configuration.
 */
import { useCallback, useEffect, useRef, useState } from 'react'

import { kvDel, kvGet, kvList, kvSet, StorageKeys } from '@/utils/storage'
import { DefaultModel, ImageModelFluxDev, isImageModel } from '../models'
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
import {
  applyUrlOverridesToConfig,
  DEFAULT_SESSION_ID,
  getActiveSessionId,
  getSessionConfigKey,
  normalizeConfigNumericFields,
} from '../utils/config-helpers'
import {
  exportAllData as exportData,
  importAllData as importData,
} from '../utils/data-sync'

async function getSessionOrderFromKv(): Promise<number[]> {
  const raw = await kvGet<number[] | { data: number[]; updated_at: number }>(
    StorageKeys.SESSION_ORDER,
  )
  return Array.isArray(raw) ? raw : raw?.data || []
}

async function setSessionOrderToKv(order: number[]): Promise<void> {
  await kvSet(StorageKeys.SESSION_ORDER, {
    data: order,
    updated_at: Date.now(),
  })
}

/**
 * createFreeTierToken builds a random free-tier token for anonymous sessions.
 */
function createFreeTierToken(): string {
  const randomStr =
    Math.random().toString(36).substring(2, 18) +
    Math.random().toString(36).substring(2, 18)
  return `FREETIER-${randomStr}`
}

/**
 * describeApiTokenKind summarizes the token shape without exposing secret material.
 */
function describeApiTokenKind(token?: string): string {
  const trimmed = token?.trim() || ''
  if (!trimmed) return 'missing'
  if (trimmed === 'DEFAULT_PROXY_TOKEN') return 'legacy-default'
  if (trimmed.startsWith('FREETIER-')) return 'freetier'
  if (trimmed.startsWith('sk-') || trimmed.startsWith('laisky-')) {
    return 'byok'
  }
  return 'custom'
}

interface HydratedSessionConfigResult {
  config: SessionConfig
  savedConfig: SessionConfig | null
  changed: boolean
}

/**
 * mergeSessionConfigs overlays the current config onto a loaded config while keeping nested switches intact.
 */
function mergeSessionConfigs(
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
 */
async function hydrateSessionConfig(
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

  let configChanged = false

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
      globalSyncKey =
        'sync-' +
        Math.random().toString(36).substring(2, 15) +
        Math.random().toString(36).substring(2, 15)
    }
    await kvSet(StorageKeys.SYNC_KEY, globalSyncKey)
  }
  finalConfig.sync_key = globalSyncKey

  const fallbackApiToken = fallbackConfig?.api_token?.trim() || ''
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

  return {
    config: finalConfig,
    savedConfig,
    changed: configChanged,
  }
}

/**
 * Hook for managing session configuration
 */
export function useConfig() {
  const [config, setConfigState] = useState<SessionConfig>(DefaultSessionConfig)
  const [sessionId, setSessionId] = useState<number>(DEFAULT_SESSION_ID)
  const [sessions, setSessions] = useState<
    { id: number; name: string; visible: boolean }[]
  >([])
  const [isLoading, setIsLoading] = useState(true)
  const configRef = useRef(config)
  const sessionIdRef = useRef(sessionId)
  const hydratedRef = useRef(false)

  useEffect(() => {
    configRef.current = config
  }, [config])

  useEffect(() => {
    sessionIdRef.current = sessionId
  }, [sessionId])

  /**
   * applySessionState updates refs and React state for the active session atomically.
   */
  const applySessionState = useCallback(
    (nextSessionId: number, nextConfig: SessionConfig) => {
      sessionIdRef.current = nextSessionId
      configRef.current = nextConfig
      setSessionId(nextSessionId)
      setConfigState(nextConfig)
      hydratedRef.current = true
    },
    [],
  )

  /**
   * Load all available sessions
   */
  const loadSessions = useCallback(async () => {
    const keys = await kvList()
    const configKeys = keys.filter((k) =>
      k.startsWith(StorageKeys.SESSION_CONFIG_PREFIX),
    )

    const sessionOrder = await getSessionOrderFromKv()

    // Sort keys by ID to keep order stable
    configKeys.sort((a, b) => {
      const idA = parseInt(a.replace(StorageKeys.SESSION_CONFIG_PREFIX, ''), 10)
      const idB = parseInt(b.replace(StorageKeys.SESSION_CONFIG_PREFIX, ''), 10)

      const indexA = sessionOrder.indexOf(idA)
      const indexB = sessionOrder.indexOf(idB)

      if (indexA !== -1 && indexB !== -1) {
        return indexA - indexB
      }
      if (indexA !== -1) return -1
      if (indexB !== -1) return 1

      return idA - idB
    })

    const loadedSessions: { id: number; name: string; visible: boolean }[] = []

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
          visible: conf?.session_visible ?? true,
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
        const hydratedConfig = await hydrateSessionConfig(activeSessionId, {
          fallbackConfig: configRef.current,
          applyUrlOverrides: true,
        })
        let finalConfig = hydratedConfig.config
        const { savedConfig } = hydratedConfig
        let { changed } = hydratedConfig

        const currentConfig = configRef.current
        const currentUpdatedAt =
          typeof currentConfig.updated_at === 'number'
            ? currentConfig.updated_at
            : 0
        const loadedUpdatedAt =
          typeof finalConfig.updated_at === 'number' ? finalConfig.updated_at : 0
        if (
          hydratedRef.current &&
          sessionIdRef.current === activeSessionId &&
          currentUpdatedAt > loadedUpdatedAt
        ) {
          finalConfig = mergeSessionConfigs(finalConfig, currentConfig)
          changed = true
          console.debug('[useConfig] keeping newer in-memory config after load', {
            sessionId: activeSessionId,
            currentUpdatedAt,
            loadedUpdatedAt,
            apiTokenKind: describeApiTokenKind(finalConfig.api_token),
          })
        }

        if (changed || !savedConfig) {
          await kvSet(getSessionConfigKey(activeSessionId), finalConfig)
        }

        applySessionState(activeSessionId, finalConfig)

        // Load all sessions
        await loadSessions()
      } catch (error) {
        console.error('Failed to load config:', error)
      } finally {
        setIsLoading(false)
      }
    }

    loadConfig()
  }, [applySessionState, loadSessions])

  /**
   * Apply URL parameter overrides to the current config
   */

  /**
   * Update and persist configuration
   */
  const updateConfig = useCallback(async (updates: Partial<SessionConfig>) => {
    let activeSessionId = sessionIdRef.current
    if (!hydratedRef.current) {
      if (!activeSessionId || activeSessionId === DEFAULT_SESSION_ID) {
        activeSessionId = await getActiveSessionId()
      }

      const hydrated = await hydrateSessionConfig(activeSessionId, {
        fallbackConfig: configRef.current,
        applyUrlOverrides: false,
      })
      configRef.current = hydrated.config
      sessionIdRef.current = activeSessionId
      setConfigState(hydrated.config)
      setSessionId(activeSessionId)
      hydratedRef.current = true

      console.debug('[useConfig] hydrated config before queued update', {
        sessionId: activeSessionId,
        apiTokenKind: describeApiTokenKind(hydrated.config.api_token),
      })
    }

    const baseConfig = configRef.current
    const newConfig = {
      ...baseConfig,
      ...updates,
      chat_switch: {
        ...baseConfig.chat_switch,
        ...(updates.chat_switch || {}),
      },
      sync_key: updates.sync_key ?? baseConfig.sync_key,
      updated_at: Date.now(),
    }

    configRef.current = newConfig
    setConfigState(newConfig)

    console.debug('[useConfig] queued config update', {
      sessionId: sessionIdRef.current,
      updatedKeys: Object.keys(updates),
      chatSwitchKeys: updates.chat_switch
        ? Object.keys(updates.chat_switch)
        : [],
      hasApiTokenUpdate: updates.api_token !== undefined,
      apiTokenLength:
        typeof newConfig.api_token === 'string'
          ? newConfig.api_token.length
          : 0,
      systemPromptLength:
        typeof newConfig.system_prompt === 'string'
          ? newConfig.system_prompt.length
          : 0,
    })

    try {
      if (updates.sync_key !== undefined) {
        await kvSet(StorageKeys.SYNC_KEY, updates.sync_key)
      }

      const key = getSessionConfigKey(sessionIdRef.current)
      await kvSet(key, newConfig)
      console.debug('[useConfig] persisted config update', {
        sessionId: sessionIdRef.current,
        updatedAt: newConfig.updated_at,
        updatedKeys: Object.keys(updates),
        apiTokenKind: describeApiTokenKind(newConfig.api_token),
      })
    } catch (error) {
      console.error('Failed to save config:', error)
    }
  }, [])

  /**
   * Reorder sessions
   */
  const reorderSessions = useCallback(
    async (newOrder: number[]) => {
      await setSessionOrderToKv(newOrder)
      await loadSessions()
    },
    [loadSessions],
  )

  /**
   * Switch to a different session
   */
  const switchSession = useCallback(
    async (newSessionId: number) => {
      setIsLoading(true)
      try {
        await kvSet(StorageKeys.SELECTED_SESSION, newSessionId)
        sessionIdRef.current = newSessionId

        const { config: nextConfig, savedConfig, changed } =
          await hydrateSessionConfig(newSessionId, {
            fallbackConfig: configRef.current,
            applyUrlOverrides: false,
          })

        applySessionState(newSessionId, {
          ...nextConfig,
          updated_at: nextConfig.updated_at || Date.now(),
        })

        if (changed || !savedConfig) {
          await kvSet(getSessionConfigKey(newSessionId), configRef.current)
        }

        console.debug('[useConfig] switched session', {
          sessionId: newSessionId,
          hasSavedConfig: !!savedConfig,
          apiTokenKind: describeApiTokenKind(configRef.current.api_token),
        })

        // Refresh list to ensure names are up to date if we defaulted above
        await loadSessions()
      } finally {
        setIsLoading(false)
      }
    },
    [applySessionState, loadSessions],
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
      const globalSyncKey = await kvGet<string>(StorageKeys.SYNC_KEY)
      const newConfig = {
        ...DefaultSessionConfig,
        api_token:
          configRef.current.api_token ||
          createFreeTierToken(),
        api_base:
          configRef.current.api_base ||
          DefaultSessionConfig.api_base,
        session_name: name || `Chat Session ${newId}`,
        updated_at: Date.now(),
        sync_key: globalSyncKey || '',
      }
      await kvSet(key, newConfig)

      // Update session order
      const sessionOrder = await getSessionOrderFromKv()
      if (!sessionOrder.includes(newId)) {
        sessionOrder.push(newId)
        await setSessionOrderToKv(sessionOrder)
      }

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

      // Update session order
      const sessionOrder = await getSessionOrderFromKv()
      if (sessionOrder) {
        const newOrder = sessionOrder.filter((id) => id !== targetSessionId)
        await setSessionOrderToKv(newOrder)
      }

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
      const globalSyncKey = await kvGet<string>(StorageKeys.SYNC_KEY)
      const duplicatedConfig: SessionConfig = {
        ...sourceConfig,
        session_name: `${sourceConfig.session_name || `Chat Session ${sourceSessionId}`} Copy`,
        updated_at: Date.now(),
        sync_key: globalSyncKey || sourceConfig.sync_key,
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

      // Update session order
      const sessionOrder = await getSessionOrderFromKv()
      if (!sessionOrder.includes(newSessionId)) {
        sessionOrder.push(newSessionId)
        await setSessionOrderToKv(sessionOrder)
      }

      await loadSessions()
      return newSessionId
    },
    [loadSessions],
  )

  const forkSession = useCallback(
    async (
      sourceSessionId: number,
      upToChatId: string,
      upToRole: 'user' | 'assistant',
    ) => {
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
      const globalSyncKey = await kvGet<string>(StorageKeys.SYNC_KEY)
      const forkedConfig: SessionConfig = {
        ...sourceConfig,
        session_name: `${sourceConfig.session_name || `Chat Session ${sourceSessionId}`} Fork`,
        updated_at: Date.now(),
        sync_key: globalSyncKey || sourceConfig.sync_key,
      }

      await kvSet(getSessionConfigKey(newSessionId), forkedConfig)

      const sourceHistory =
        (await kvGet<SessionHistoryItem[]>(
          getSessionHistoryKey(sourceSessionId),
        )) || []
      const historyKey = getSessionHistoryKey(newSessionId)
      const idMap = new Map<string, string>()
      const forkedHistory: SessionHistoryItem[] = []

      for (const item of sourceHistory) {
        if (!item.chatID) {
          continue
        }

        let nextChatId = idMap.get(item.chatID)
        if (!nextChatId) {
          nextChatId = generateChatId()
          idMap.set(item.chatID, nextChatId)
        }

        forkedHistory.push({ ...item, chatID: nextChatId })

        const chatData = await kvGet<ChatMessageData>(
          getChatDataKey(item.chatID, item.role),
        )
        if (chatData) {
          await kvSet(getChatDataKey(nextChatId, item.role), {
            ...chatData,
            chatID: nextChatId,
          })
        }

        if (item.chatID === upToChatId && item.role === upToRole) {
          break
        }
      }

      await kvSet(historyKey, forkedHistory)

      // Update session order
      const sessionOrder = await getSessionOrderFromKv()
      if (!sessionOrder.includes(newSessionId)) {
        sessionOrder.push(newSessionId)
        await setSessionOrderToKv(sessionOrder)
      }

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
        key.startsWith(StorageKeys.CHAT_DATA_PREFIX) ||
        key === StorageKeys.SESSION_ORDER
      ) {
        await kvDel(key)
      }
    }

    const preservedToken = config.api_token
    const preservedBase = config.api_base
    const globalSyncKey = await kvGet<string>(StorageKeys.SYNC_KEY)
    const newConfig: SessionConfig = {
      ...DefaultSessionConfig,
      api_token: preservedToken,
      api_base: preservedBase,
      sync_key: globalSyncKey || '',
      session_name: DefaultSessionConfig.session_name,
      updated_at: Date.now(),
    }

    await kvSet(getSessionConfigKey(DEFAULT_SESSION_ID), newConfig)
    await kvSet(getSessionHistoryKey(DEFAULT_SESSION_ID), [])
    await kvSet(StorageKeys.SELECTED_SESSION, DEFAULT_SESSION_ID)
    await setSessionOrderToKv([DEFAULT_SESSION_ID])

    applySessionState(DEFAULT_SESSION_ID, newConfig)
    setSessions([
      {
        id: DEFAULT_SESSION_ID,
        name: newConfig.session_name || `Chat Session ${DEFAULT_SESSION_ID}`,
        visible: true,
      },
    ])
    await loadSessions()
  }, [applySessionState, config.api_base, config.api_token, loadSessions])

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
      conf.updated_at = Date.now()
      await kvSet(key, conf)

      // If it's the current session, update state too
      if (targetId === sessionId) {
        applySessionState(targetId, conf)
      }

      await loadSessions()
    },
    [applySessionState, sessionId, loadSessions],
  )

  /**
   * Update session visibility
   */
  const updateSessionVisibility = useCallback(
    async (targetId: number, visible: boolean) => {
      const key = getSessionConfigKey(targetId)
      let conf = await kvGet<SessionConfig>(key)
      if (!conf) {
        conf = { ...DefaultSessionConfig }
      }

      conf.session_visible = visible
      conf.updated_at = Date.now()
      await kvSet(key, conf)

      // If it's the current session, update state too
      if (targetId === sessionId) {
        applySessionState(targetId, conf)
      }

      await loadSessions()
    },
    [applySessionState, sessionId, loadSessions],
  )

  /**
   * Export all data (sessions, configs, shortcuts) for sync
   */
  const exportAllData = useCallback(async () => {
    return exportData()
  }, [])

  /**
   * Import data (overwrite existing)
   */
  const importAllData = useCallback(
    async (
      data: Record<string, unknown>,
      mode: 'merge' | 'download' = 'merge',
    ) => {
      return importData(data, sessionId, mode)
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
    reorderSessions,
    createSession,
    deleteSession,
    renameSession,
    updateSessionVisibility,
    duplicateSession,
    forkSession,
    purgeAllSessions,
    exportAllData,
    importAllData,
  }
}
