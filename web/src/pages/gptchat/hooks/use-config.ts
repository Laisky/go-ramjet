/**
 * React hook for managing chat configuration.
 */
import { useCallback, useEffect, useRef, useState } from 'react'

import { createLatestValueWriter } from '@/utils/latest-value-writer'
import { kvDel, kvGet, kvList, kvSet, StorageKeys } from '@/utils/storage'
import {
  DefaultSessionConfig,
  type SessionConfig,
  type SessionHistoryItem,
} from '../types'
import { createFreeTierToken, describeApiTokenKind } from '../utils/api-token'
import { getChatDataKey, getSessionHistoryKey } from '../utils/chat-storage'
import {
  DEFAULT_SESSION_ID,
  getActiveSessionId,
  getSessionConfigKey,
  normalizeRetiredResearchConfig,
} from '../utils/config-helpers'
import {
  type ConfigUpdateResult,
  validateCredentialUpdate,
} from '../utils/credential-policy'
import {
  exportAllData as exportData,
  importAllData as importData,
} from '../utils/data-sync'
import {
  hydrateSessionConfig,
  mergeSessionConfigs,
  type PendingLinkChange,
} from './session-config-hydration'
import { copyStoredSession } from './session-copy'
import {
  listStoredApiTokens,
  rebindStoredSyncKey,
  repairStoredSessionCredentials,
} from './session-credentials'
import { getSessionOrderFromKv, setSessionOrderToKv } from './session-order'

/**
 * alwaysActive is the default liveness check for loads whose caller cannot
 * be unmounted before they finish.
 */
const alwaysActive = () => true

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
  // What a link asked to change on a stored session (I4); it waits for the
  // user to confirm or dismiss it.
  const [pendingLinkChange, setPendingLinkChange] =
    useState<PendingLinkChange | null>(null)
  const configRef = useRef(config)
  const sessionIdRef = useRef(sessionId)
  const hydratedRef = useRef(false)
  // Instance liveness differs from an individual StrictMode effect run.
  const mountedRef = useRef(false)
  // Every config write goes through one ordered queue: a write that retries
  // inside IndexedDB can then never land after a newer one, and readers can
  // wait for pending writes before reading storage.
  const [persistQueue] = useState(() =>
    createLatestValueWriter<unknown>((key, value) => kvSet(key, value)),
  )

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
   * loadSessions reloads the session list from storage into state. isActive
   * is checked once the list is read; when it returns false the list is
   * dropped, so a load that outlives its component never sets state.
   */
  const loadSessions = useCallback(async (isActive = alwaysActive) => {
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
        console.error(
          `[useConfig] failed to load session config ${key}: ${String(e)}`,
        )
      }
    }

    // If no sessions found, at least include current default (should exist after load logic)
    if (loadedSessions.length === 0) {
      // We defer to what loadConfig created
    }

    if (isActive()) {
      setSessions(loadedSessions)
    }
  }, [])

  // Load configuration on mount. The load can outlive the component (a route
  // change, or a test whose environment is torn down), so once the effect is
  // cleaned up it keeps storing data but stops setting state.
  useEffect(() => {
    mountedRef.current = true
    let active = true
    const isActive = () => active
    const loadConfig = async () => {
      try {
        // Run migration first
        const { migrateLegacyData } =
          await import('@/pages/gptchat/utils/migration')
        await migrateLegacyData()

        const activeSessionId = await getActiveSessionId()
        await persistQueue.whenIdle()
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
            `[useConfig] keeping newer in-memory config after load: session=${activeSessionId} current=${currentUpdatedAt} loaded=${loadedUpdatedAt} apiTokenKind=${describeApiTokenKind(finalConfig.api_token)}`,
          )
        }

        if (changed || !savedConfig) {
          await persistQueue.write(
            getSessionConfigKey(activeSessionId),
            finalConfig,
          )
        }
        // Keep the first load's captured prompt across StrictMode replay,
        // even when that individual effect run was cleaned up. A genuine
        // unmount ends the instance's lifetime and must suppress this setter.
        if (mountedRef.current && hydratedConfig.pendingLinkChange) {
          setPendingLinkChange({
            ...hydratedConfig.pendingLinkChange,
            sessionId: activeSessionId,
          })
        }

        if (active) {
          applySessionState(activeSessionId, finalConfig)
        }

        // Repair inactive sessions too, so none of them is exported,
        // uploaded, or switched to with a sync key as its API token.
        try {
          const repaired = await repairStoredSessionCredentials(
            finalConfig.sync_key || '',
            activeSessionId,
            persistQueue.write,
          )
          if (repaired.length > 0) {
            console.warn(
              `[useConfig] repaired clobbered API tokens of inactive sessions: ${repaired.join(',')}`,
            )
          }
        } catch (error) {
          console.error(
            `[useConfig] failed to repair stored session credentials: ${String(error)}`,
          )
        }

        // Load all sessions
        await loadSessions(isActive)
      } catch (error) {
        console.error(`[useConfig] failed to load config: ${String(error)}`)
      } finally {
        if (active) {
          setIsLoading(false)
        }
      }
    }

    loadConfig()
    return () => {
      active = false
      mountedRef.current = false
    }
  }, [applySessionState, loadSessions, persistQueue])

  /**
   * updateConfig merges updates into the active session config and stores
   * it. Credential fields go through validateCredentialUpdate: a field that
   * would make an API token and the sync key equal is dropped while every
   * other field still applies, and saving a key of the user's own completes
   * a pending API key reset. A new sync key is checked against the API key
   * of every stored session, and once stored it is copied into every
   * session config. The write targets the session that was active when the
   * update was applied, even if the user switches sessions meanwhile.
   * It resolves with the dropped credential fields once the update is
   * stored. It rejects when storing fails, and the in-memory config then
   * reverts unless a newer update has been applied on top of it.
   */
  const updateConfig = useCallback(
    async (updates: Partial<SessionConfig>): Promise<ConfigUpdateResult> => {
      if (!hydratedRef.current) {
        let hydrateSessionId = sessionIdRef.current
        if (!hydrateSessionId || hydrateSessionId === DEFAULT_SESSION_ID) {
          hydrateSessionId = await getActiveSessionId()
        }

        const hydrated = await hydrateSessionConfig(hydrateSessionId, {
          fallbackConfig: configRef.current,
          applyUrlOverrides: false,
        })
        configRef.current = hydrated.config
        sessionIdRef.current = hydrateSessionId
        setConfigState(hydrated.config)
        setSessionId(hydrateSessionId)
        hydratedRef.current = true

        console.debug(
          `[useConfig] hydrated config before queued update: session=${hydrateSessionId} apiTokenKind=${describeApiTokenKind(hydrated.config.api_token)}`,
        )
      }

      // A new sync key must not copy the API key of any stored session.
      // Pending writes land first, so the check sees every key.
      let otherApiTokens: string[] = []
      if (
        updates.sync_key !== undefined &&
        updates.sync_key !== configRef.current.sync_key
      ) {
        await persistQueue.whenIdle()
        otherApiTokens = await listStoredApiTokens()
      }

      const targetSessionId = sessionIdRef.current
      const baseConfig = configRef.current
      const { accepted, rejections } = validateCredentialUpdate(
        baseConfig,
        updates,
        { otherApiTokens },
      )
      for (const rejection of rejections) {
        console.warn(
          `[useConfig] rejected ${rejection.field} update: session=${targetSessionId} reason=${rejection.reason}`,
        )
      }
      if (Object.keys(accepted).length === 0) {
        return { rejections }
      }

      const newConfig = normalizeRetiredResearchConfig({
        ...baseConfig,
        ...accepted,
        chat_switch: {
          ...baseConfig.chat_switch,
          ...(accepted.chat_switch || {}),
        },
        sync_key: accepted.sync_key ?? baseConfig.sync_key,
        updated_at: Date.now(),
      })
      const nextSyncKey =
        accepted.sync_key !== undefined &&
        accepted.sync_key !== baseConfig.sync_key
          ? accepted.sync_key
          : undefined

      configRef.current = newConfig
      setConfigState(newConfig)

      console.debug(
        `[useConfig] queued config update: session=${targetSessionId} keys=${Object.keys(accepted).join(',')} apiTokenKind=${describeApiTokenKind(newConfig.api_token)}`,
      )

      try {
        // The config write is enqueued before any await, so a later update
        // can never slip an older config in between. The browser-wide sync
        // key follows only once the config holding it is stored.
        await persistQueue.write(
          getSessionConfigKey(targetSessionId),
          newConfig,
        )
        if (nextSyncKey !== undefined) {
          await persistQueue.write(StorageKeys.SYNC_KEY, nextSyncKey)
        }
      } catch (error) {
        if (configRef.current === newConfig) {
          configRef.current = baseConfig
          setConfigState(baseConfig)
        }
        throw new Error(
          `saving the config of session ${targetSessionId} failed: ${String(error)}`,
          { cause: error },
        )
      }

      if (nextSyncKey !== undefined) {
        try {
          await persistQueue.whenIdle()
          await rebindStoredSyncKey(
            nextSyncKey,
            targetSessionId,
            persistQueue.write,
          )
        } catch (error) {
          // The key itself is stored; loading or exporting a session binds
          // a stale copy again, so this only needs a note.
          console.warn(
            `[useConfig] failed to copy the new sync key into every session: ${String(error)}`,
          )
        }
      }

      console.debug(
        `[useConfig] persisted config update: session=${targetSessionId} updatedAt=${newConfig.updated_at}`,
      )
      return { rejections }
    },
    [persistQueue],
  )

  /**
   * confirmPendingLinkChange applies the link-supplied endpoint and key the
   * user confirmed, in one write, but only to the session the link was
   * opened in. A key the policy refuses aborts the whole change, so the
   * stored key never moves to the new endpoint alone. The prompt stays when
   * the change is refused or storing fails, and the returned promise rejects.
   */
  const confirmPendingLinkChange = useCallback(async () => {
    const pending = pendingLinkChange
    if (!pending || pending.sessionId !== sessionIdRef.current) {
      setPendingLinkChange(null)
      return
    }
    const updates: Partial<SessionConfig> = {}
    if (pending.apiBase !== undefined) updates.api_base = pending.apiBase
    if (pending.apiToken !== undefined) updates.api_token = pending.apiToken
    if (
      validateCredentialUpdate(configRef.current, updates).rejections.length
    ) {
      throw new Error('the API key from the link was refused')
    }
    await updateConfig(updates)
    setPendingLinkChange(null)
  }, [pendingLinkChange, updateConfig])

  /** dismissPendingLinkChange keeps the current settings and drops the prompt. */
  const dismissPendingLinkChange = useCallback(() => {
    setPendingLinkChange(null)
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
   * loadSessionIntoState makes targetSessionId the session held in memory:
   * pending writes land, its stored config is hydrated and repaired, and a
   * corrected config is written back. It also refreshes the session list.
   */
  const loadSessionIntoState = useCallback(
    async (targetSessionId: number) => {
      await persistQueue.whenIdle()
      sessionIdRef.current = targetSessionId

      const {
        config: nextConfig,
        savedConfig,
        changed,
      } = await hydrateSessionConfig(targetSessionId, {
        fallbackConfig: configRef.current,
        applyUrlOverrides: false,
      })

      applySessionState(targetSessionId, {
        ...nextConfig,
        updated_at: nextConfig.updated_at || Date.now(),
      })

      if (changed || !savedConfig) {
        await persistQueue.write(
          getSessionConfigKey(targetSessionId),
          configRef.current,
        )
      }

      console.debug(
        `[useConfig] loaded session ${targetSessionId}: saved=${!!savedConfig} apiTokenKind=${describeApiTokenKind(configRef.current.api_token)}`,
      )

      await loadSessions()
    },
    [applySessionState, loadSessions, persistQueue],
  )

  /**
   * switchSession makes newSessionId the active session.
   */
  const switchSession = useCallback(
    async (newSessionId: number) => {
      setIsLoading(true)
      try {
        await kvSet(StorageKeys.SELECTED_SESSION, newSessionId)
        await loadSessionIntoState(newSessionId)
      } finally {
        setIsLoading(false)
      }
    },
    [loadSessionIntoState],
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
      // A link prompt belongs to the config it was raised for; a session
      // recreated under the same id must not inherit it.
      setPendingLinkChange((pending) =>
        pending?.sessionId === targetSessionId ? null : pending,
      )
      // A pending write must not resurrect the deleted config.
      await persistQueue.whenIdle()
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
    [sessionId, switchSession, loadSessions, persistQueue],
  )

  /**
   * duplicateSession copies sourceSessionId, history included, into a new
   * session. It returns the new session id.
   */
  const duplicateSession = useCallback(
    async (sourceSessionId: number) => {
      await persistQueue.whenIdle()
      const newSessionId = await copyStoredSession(sourceSessionId, {
        nameSuffix: 'Copy',
      })
      await loadSessions()
      return newSessionId
    },
    [loadSessions, persistQueue],
  )

  /**
   * forkSession copies sourceSessionId into a new session, keeping the
   * history up to the message upToChatId/upToRole. It returns the new
   * session id.
   */
  const forkSession = useCallback(
    async (
      sourceSessionId: number,
      upToChatId: string,
      upToRole: 'user' | 'assistant',
    ) => {
      await persistQueue.whenIdle()
      const newSessionId = await copyStoredSession(sourceSessionId, {
        nameSuffix: 'Fork',
        upTo: { chatId: upToChatId, role: upToRole },
      })
      await loadSessions()
      return newSessionId
    },
    [loadSessions, persistQueue],
  )

  const purgeAllSessions = useCallback(async () => {
    setPendingLinkChange(null)
    await persistQueue.whenIdle()
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
  }, [
    applySessionState,
    config.api_base,
    config.api_token,
    loadSessions,
    persistQueue,
  ])

  /**
   * Rename a session
   */
  const renameSession = useCallback(
    async (targetId: number, newName: string) => {
      const key = getSessionConfigKey(targetId)
      // Read after pending writes land, to preserve every other field.
      await persistQueue.whenIdle()
      let conf = await kvGet<SessionConfig>(key)
      if (!conf) {
        conf = { ...DefaultSessionConfig }
      }

      conf.session_name = newName
      conf.updated_at = Date.now()
      await persistQueue.write(key, conf)

      // If it's the current session, update state too
      if (targetId === sessionId) {
        applySessionState(targetId, conf)
      }

      await loadSessions()
    },
    [applySessionState, sessionId, loadSessions, persistQueue],
  )

  /**
   * Update session visibility
   */
  const updateSessionVisibility = useCallback(
    async (targetId: number, visible: boolean) => {
      const key = getSessionConfigKey(targetId)
      await persistQueue.whenIdle()
      let conf = await kvGet<SessionConfig>(key)
      if (!conf) {
        conf = { ...DefaultSessionConfig }
      }

      conf.session_visible = visible
      conf.updated_at = Date.now()
      await persistQueue.write(key, conf)

      // If it's the current session, update state too
      if (targetId === sessionId) {
        applySessionState(targetId, conf)
      }

      await loadSessions()
    },
    [applySessionState, sessionId, loadSessions, persistQueue],
  )

  /**
   * exportAllData collects all local data (sessions, configs, shortcuts)
   * for sync once every pending config write has landed, so the snapshot
   * holds the newest config. It returns the export.
   */
  const exportAllData = useCallback(async () => {
    await persistQueue.whenIdle()
    return exportData()
  }, [persistQueue])

  /**
   * importAllData applies a cloud backup to local data in mode ('merge' or
   * 'download') once pending config writes have landed, then reloads the
   * active session from storage, so a later update builds on the imported
   * config instead of overwriting it with the stale one held in memory.
   */
  const importAllData = useCallback(
    async (
      data: Record<string, unknown>,
      mode: 'merge' | 'download' = 'merge',
    ) => {
      // Imported configs replace the one a pending link prompt describes.
      setPendingLinkChange(null)
      await persistQueue.whenIdle()
      const activeSessionId = sessionIdRef.current
      await importData(data, activeSessionId, mode)
      await loadSessionIntoState(activeSessionId)
    },
    [loadSessionIntoState, persistQueue],
  )

  return {
    config,
    sessionId,
    sessions,
    isLoading,
    updateConfig,
    pendingLinkChange:
      pendingLinkChange?.sessionId === sessionId ? pendingLinkChange : null,
    confirmPendingLinkChange,
    dismissPendingLinkChange,
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
