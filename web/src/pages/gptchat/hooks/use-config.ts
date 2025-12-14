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
  const [isLoading, setIsLoading] = useState(true)

  // Load configuration on mount
  useEffect(() => {
    const loadConfig = async () => {
      try {
        const activeSessionId = await getActiveSessionId()
        setSessionId(activeSessionId)

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

        // Auto-generate FREETIER token if missing
        if (!finalConfig.api_token || finalConfig.api_token === 'DEFAULT_PROXY_TOKEN') {
          const randomStr = Math.random().toString(36).substring(2, 18) + Math.random().toString(36).substring(2, 18)
          finalConfig.api_token = `FREETIER-${randomStr}`
          console.debug('Generated new FREETIER token:', finalConfig.api_token)
          // Save immediately so it persists
          await kvSet(key, finalConfig)
        }

        // Ensure api_base is set
        if (!finalConfig.api_base) {
            finalConfig.api_base = 'https://api.openai.com'
        }

        setConfigState(finalConfig)

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
    [config, sessionId]
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
          chat_switch: {
            ...DefaultSessionConfig.chat_switch,
            ...savedConfig.chat_switch,
          },
        })
      } else {
        setConfigState(DefaultSessionConfig)
      }
    } finally {
      setIsLoading(false)
    }
  }, [])

  /**
   * Create a new session
   */
  const createSession = useCallback(async (): Promise<number> => {
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
    await kvSet(key, DefaultSessionConfig)

    return newId
  }, [])

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
      if (targetSessionId === sessionId) {
        await switchSession(DEFAULT_SESSION_ID)
      }
    },
    [sessionId, switchSession]
  )

  return {
    config,
    sessionId,
    isLoading,
    updateConfig,
    switchSession,
    createSession,
    deleteSession,
  }
}
