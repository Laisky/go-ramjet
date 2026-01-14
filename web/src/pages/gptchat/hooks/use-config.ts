/**
 * React hook for managing chat configuration.
 */
import { useCallback, useEffect, useState } from 'react'

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
 * Hook for managing session configuration
 */
export function useConfig() {
  const [config, setConfigState] = useState<SessionConfig>(DefaultSessionConfig)
  const [sessionId, setSessionId] = useState<number>(DEFAULT_SESSION_ID)
  const [sessions, setSessions] = useState<
    { id: number; name: string; visible: boolean }[]
  >([])
  const [isLoading, setIsLoading] = useState(true)

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
        const key = getSessionConfigKey(activeSessionId)
        const savedConfig = await kvGet<SessionConfig>(key)
        console.debug(`[useConfig] load session ${activeSessionId} config`, {
          hasSavedConfig: !!savedConfig,
          // @ts-ignore
          legacySelectedModel: savedConfig?.selected_model,
        })

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
        // If savedConfig exists but doesn't have the new split model fields,
        // we must derive them from the legacy selected_model.
        const hasSelectedChatModel =
          savedConfig &&
          Object.prototype.hasOwnProperty.call(
            savedConfig,
            'selected_chat_model',
          )
        if (!hasSelectedChatModel) {
          console.debug(
            `[useConfig] migrating legacy selected_model to selected_chat_model: ${finalConfig.selected_model}`,
          )
          finalConfig.selected_chat_model = isImageModel(
            finalConfig.selected_model,
          )
            ? DefaultModel
            : finalConfig.selected_model || DefaultModel
          configChanged = true
        }

        const hasSelectedDrawModel =
          savedConfig &&
          Object.prototype.hasOwnProperty.call(
            savedConfig,
            'selected_draw_model',
          )
        if (!hasSelectedDrawModel) {
          console.debug(
            `[useConfig] migrating legacy selected_model to selected_draw_model: ${finalConfig.selected_model}`,
          )
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
        updated_at: Date.now(),
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
            updated_at: Date.now(),
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
        updated_at: Date.now(),
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
      const duplicatedConfig: SessionConfig = {
        ...sourceConfig,
        session_name: `${sourceConfig.session_name || `Chat Session ${sourceSessionId}`} Copy`,
        updated_at: Date.now(),
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
    const newConfig: SessionConfig = {
      ...DefaultSessionConfig,
      api_token: preservedToken,
      api_base: preservedBase,
      session_name: DefaultSessionConfig.session_name,
      updated_at: Date.now(),
    }

    await kvSet(getSessionConfigKey(DEFAULT_SESSION_ID), newConfig)
    await kvSet(getSessionHistoryKey(DEFAULT_SESSION_ID), [])
    await kvSet(StorageKeys.SELECTED_SESSION, DEFAULT_SESSION_ID)
    await setSessionOrderToKv([DEFAULT_SESSION_ID])

    setSessionId(DEFAULT_SESSION_ID)
    setConfigState(newConfig)
    setSessions([
      {
        id: DEFAULT_SESSION_ID,
        name: newConfig.session_name || `Chat Session ${DEFAULT_SESSION_ID}`,
        visible: true,
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
      conf.updated_at = Date.now()
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
    return exportData()
  }, [])

  /**
   * Import data (overwrite existing)
   */
  const importAllData = useCallback(
    async (data: Record<string, unknown>, mode: 'merge' | 'download' = 'merge') => {
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
    purgeAllSessions,
    exportAllData,
    importAllData,
  }
}
