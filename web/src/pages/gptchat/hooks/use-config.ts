/**
 * React hook for managing chat configuration.
 */
import { useCallback, useEffect, useState } from 'react'

import { kvGet, kvSet, StorageKeys } from '@/utils/storage'
import { DefaultSessionConfig, type SessionConfig } from '../types'

const DEFAULT_SESSION_ID = 1

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

  // Load configuration on mount
  useEffect(() => {
    const loadConfig = async () => {
      try {
        // Run migration first
        const { migrateLegacyData } = await import('@/pages/gptchat/utils/migration')
        await migrateLegacyData()

        const activeSessionId = await getActiveSessionId()
        const key = getSessionConfigKey(activeSessionId)
        const savedConfig = await kvGet<SessionConfig>(key)

        let finalConfig = {
          ...DefaultSessionConfig,
        }

        if (savedConfig) {
          finalConfig = {
            ...finalConfig,
            ...savedConfig,
            chat_switch: {
              ...finalConfig.chat_switch,
              ...savedConfig.chat_switch,
            },
          }
        }

        let configChanged = false

        // Generate sync_key if missing
        if (!finalConfig.sync_key) {
          finalConfig.sync_key = 'sync-' + Math.random().toString(36).substring(2, 15) + Math.random().toString(36).substring(2, 15)
          configChanged = true
        }

        // Auto-generate FREETIER token if missing
        if (!finalConfig.api_token || finalConfig.api_token === 'DEFAULT_PROXY_TOKEN') {
          const randomStr = Math.random().toString(36).substring(2, 18) + Math.random().toString(36).substring(2, 18)
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

        if (configChanged || !savedConfig) {
           await kvSet(key, finalConfig)
        }

        setConfigState(finalConfig)
        setSessionId(activeSessionId)

        // Load all sessions
        await loadSessions()

        // Apply URL parameter overrides (these might overwrite the token if provided in URL)
        applyUrlOverrides()
      } catch (error) {
        console.error('Failed to load config:', error)
      } finally {
        setIsLoading(false)
      }
    }

    loadConfig()
  }, [])

  /**
   * Load all available sessions
   */
  const loadSessions = useCallback(async () => {
    const { kvList, kvGet } = await import('@/utils/storage')
    const keys = await kvList()
    const configKeys = keys.filter(k => k.startsWith(StorageKeys.SESSION_CONFIG_PREFIX))

    // Sort keys by ID to keep order stable
    configKeys.sort((a, b) => {
        const idA = parseInt(a.replace(StorageKeys.SESSION_CONFIG_PREFIX, ''), 10)
        const idB = parseInt(b.replace(StorageKeys.SESSION_CONFIG_PREFIX, ''), 10)
        return idA - idB
    })

    const loadedSessions: { id: number; name: string }[] = []

    for (const key of configKeys) {
        try {
            const id = parseInt(key.replace(StorageKeys.SESSION_CONFIG_PREFIX, ''), 10)
            if (isNaN(id)) continue

            const conf = await kvGet<SessionConfig>(key)
            loadedSessions.push({
                id,
                name: conf?.session_name || `Chat Session ${id}`
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

  /**
   * Apply URL parameter overrides to the current config
   */
  const applyUrlOverrides = useCallback(() => {
    const url = new URL(window.location.href)
    const params = url.searchParams

    setConfigState((prev: SessionConfig) => {
      const updates: Partial<SessionConfig> = {}
      let mutated = false

      // Model
      const model = params.get('model') || params.get('chat_model')
      if (model) {
        updates.selected_model = model
        mutated = true
      }

      // API token
      const apiKey = params.get('api_key') || params.get('token')
      if (apiKey) {
        updates.api_token = apiKey
        mutated = true
      }

      // System prompt
      const systemPrompt = params.get('system_prompt') || params.get('prompt')
      if (systemPrompt) {
        updates.system_prompt = systemPrompt
        mutated = true
      }

      // Temperature
      const temperature = params.get('temperature')
      if (temperature) {
        const val = parseFloat(temperature)
        if (!isNaN(val)) {
          updates.temperature = val
          mutated = true
        }
      }

      // Max tokens
      const maxTokens = params.get('max_tokens')
      if (maxTokens) {
        const val = parseInt(maxTokens, 10)
        if (!isNaN(val)) {
          updates.max_tokens = val
          mutated = true
        }
      }

      // Clear used parameters from URL
      if (mutated) {
        const paramsToRemove = [
          'model',
          'chat_model',
          'api_key',
          'token',
          'system_prompt',
          'prompt',
          'temperature',
          'max_tokens',
        ]
        paramsToRemove.forEach((p) => params.delete(p))

        const newUrl = `${url.pathname}${params.toString() ? `?${params.toString()}` : ''}${url.hash}`
        window.history.replaceState({}, document.title, newUrl)

        return { ...prev, ...updates }
      }

      return prev
    })
  }, [])

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
    [config, sessionId, loadSessions]
  )

  /**
   * Switch to a different session
   */
  const switchSession = useCallback(async (newSessionId: number) => {
    setIsLoading(true)
    try {
      await kvSet(StorageKeys.SELECTED_SESSION, newSessionId)
      setSessionId(newSessionId)

      const key = getSessionConfigKey(newSessionId)
      const savedConfig = await kvGet<SessionConfig>(key)

      if (savedConfig) {
        setConfigState({
          ...DefaultSessionConfig,
          ...savedConfig,
        //   session_name: savedConfig.session_name || `Chat Session ${newSessionId}`, // Ensure name exists
          chat_switch: {
            ...DefaultSessionConfig.chat_switch,
            ...savedConfig.chat_switch,
          },
        })
      } else {
        const newConf = {
            ...DefaultSessionConfig,
            session_name: `Chat Session ${newSessionId}`
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
  }, [loadSessions])

  /**
   * Create a new session
   */
  const createSession = useCallback(async (name?: string): Promise<number> => {
    // Find the next available session ID
    const keys = await import('@/utils/storage').then((m) => m.kvList())
    const sessionIds = keys
      .filter((k) => k.startsWith(StorageKeys.SESSION_CONFIG_PREFIX))
      .map((k) => parseInt(k.replace(StorageKeys.SESSION_CONFIG_PREFIX, ''), 10))
      .filter((id) => !isNaN(id))

    const maxId = sessionIds.length > 0 ? Math.max(...sessionIds) : 0
    const newId = maxId + 1

    // Initialize new session with defaults
    const key = getSessionConfigKey(newId)
    const newConfig = {
        ...DefaultSessionConfig,
        session_name: name || `Chat Session ${newId}`
    }
    await kvSet(key, newConfig)

    await loadSessions()

    return newId
  }, [loadSessions])

  /**
   * Delete a session
   */
  const deleteSession = useCallback(
    async (targetSessionId: number) => {
      const { kvDel } = await import('@/utils/storage')

      // Delete session config
      await kvDel(getSessionConfigKey(targetSessionId))

      // Delete session history
      await kvDel(`${StorageKeys.SESSION_HISTORY_PREFIX}${targetSessionId}`)

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
    [sessionId, switchSession, loadSessions]
  )

  /**
   * Rename a session
   */
  const renameSession = useCallback(async (targetId: number, newName: string) => {
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
  }, [sessionId, loadSessions])



  /**
   * Export all data (sessions, configs, shortcuts) for sync
   */
  const exportAllData = useCallback(async () => {
    const { kvList, kvGet } = await import('@/utils/storage')
    const keys = await kvList()
    const data: Record<string, any> = {}

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
  const importAllData = useCallback(async (data: Record<string, any>) => {
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
  }, [sessionId])

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
    exportAllData,
    importAllData,
  }
}
