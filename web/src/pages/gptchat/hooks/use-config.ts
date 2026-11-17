/**
 * React hook for managing chat configuration.
 */
import { useCallback, useEffect, useRef, useState } from 'react'

import { kvDel, kvGet, kvList, kvSet, StorageKeys } from '@/utils/storage'
import {
  DefaultSessionConfig,
  type ChatMessageData,
  type SessionConfig,
  type SessionHistoryItem,
} from '../types'
import {
  apiTokenCollidesWithSyncKey,
  createFreeTierToken,
  describeApiTokenKind,
  isApiTokenClobberedBySyncKey,
} from '../utils/api-token'
import {
  generateChatId,
  getChatDataKey,
  getSessionHistoryKey,
} from '../utils/chat-storage'
import {
  DEFAULT_SESSION_ID,
  getActiveSessionId,
  getSessionConfigKey,
  normalizeRetiredResearchConfig,
} from '../utils/config-helpers'
import {
  exportAllData as exportData,
  importAllData as importData,
} from '../utils/data-sync'
import {
  hydrateSessionConfig,
  mergeSessionConfigs,
} from './session-config-hydration'
import { getSessionOrderFromKv, setSessionOrderToKv } from './session-order'

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
          typeof finalConfig.updated_at === 'number'
            ? finalConfig.updated_at
            : 0
        if (
          hydratedRef.current &&
          sessionIdRef.current === activeSessionId &&
          currentUpdatedAt > loadedUpdatedAt
        ) {
          finalConfig = mergeSessionConfigs(finalConfig, currentConfig)
          changed = true
          console.debug(
            '[useConfig] keeping newer in-memory config after load',
            {
              sessionId: activeSessionId,
              currentUpdatedAt,
              loadedUpdatedAt,
              apiTokenKind: describeApiTokenKind(finalConfig.api_token),
            },
          )
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
   * updateConfig merges updates into the active session config and persists
   * it. An API token that would copy the sync key, or a sync key that would
   * copy a custom API token, is dropped while the other fields still apply.
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
    const acceptedUpdates: Partial<SessionConfig> = { ...updates }
    // The API token and the sync key must never hold the same non-API-shaped
    // secret, whichever field is written; drop the offending field and keep
    // every other change, so auto-save of unrelated settings still works.
    if (
      acceptedUpdates.sync_key !== undefined &&
      apiTokenCollidesWithSyncKey(
        acceptedUpdates.api_token ?? baseConfig.api_token,
        acceptedUpdates.sync_key,
      )
    ) {
      delete acceptedUpdates.sync_key
      console.warn(
        `[useConfig] rejected sync key update copying the API token: session=${sessionIdRef.current}`,
      )
    }
    const nextSyncKey = acceptedUpdates.sync_key ?? baseConfig.sync_key
    if (
      acceptedUpdates.api_token !== undefined &&
      isApiTokenClobberedBySyncKey(acceptedUpdates.api_token, nextSyncKey)
    ) {
      const rejectedKind = describeApiTokenKind(acceptedUpdates.api_token)
      delete acceptedUpdates.api_token
      console.warn(
        `[useConfig] rejected API token update copying sync key: session=${sessionIdRef.current} rejectedKind=${rejectedKind}`,
      )
    }

    // api_token_reset_required only changes through an explicit update: the
    // settings sidebar clears it once the user finishes entering a key, so a
    // half-typed key never silences the alert.
    const newConfig = normalizeRetiredResearchConfig({
      ...baseConfig,
      ...acceptedUpdates,
      chat_switch: {
        ...baseConfig.chat_switch,
        ...(acceptedUpdates.chat_switch || {}),
      },
      sync_key: nextSyncKey,
      updated_at: Date.now(),
    })

    configRef.current = newConfig
    setConfigState(newConfig)

    console.debug('[useConfig] queued config update', {
      sessionId: sessionIdRef.current,
      updatedKeys: Object.keys(acceptedUpdates),
      chatSwitchKeys: acceptedUpdates.chat_switch
        ? Object.keys(acceptedUpdates.chat_switch)
        : [],
      hasApiTokenUpdate: acceptedUpdates.api_token !== undefined,
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
      if (acceptedUpdates.sync_key !== undefined) {
        await kvSet(StorageKeys.SYNC_KEY, acceptedUpdates.sync_key)
      }

      const key = getSessionConfigKey(sessionIdRef.current)
      await kvSet(key, newConfig)
      console.debug('[useConfig] persisted config update', {
        sessionId: sessionIdRef.current,
        updatedAt: newConfig.updated_at,
        updatedKeys: Object.keys(acceptedUpdates),
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

        const {
          config: nextConfig,
          savedConfig,
          changed,
        } = await hydrateSessionConfig(newSessionId, {
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
        api_token: configRef.current.api_token || createFreeTierToken(),
        api_base: configRef.current.api_base || DefaultSessionConfig.api_base,
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
