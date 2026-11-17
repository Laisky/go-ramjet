/**
 * React hook behind the API key reset alert: lists the keys other sessions
 * hold that the user may choose for the active session, and applies a
 * choice in a single update.
 */
import { useCallback, useEffect, useState } from 'react'

import type { SessionConfig } from '../types'
import {
  type ApiTokenChoice,
  listApiTokenChoices,
  readAdoptableApiToken,
} from './session-credentials'

/**
 * UseApiTokenRecoveryOptions configures useApiTokenRecovery: sessionId and
 * config describe the active session, and updateConfig persists an update.
 */
export interface UseApiTokenRecoveryOptions {
  sessionId: number
  config: SessionConfig
  updateConfig: (updates: Partial<SessionConfig>) => Promise<void>
}

/**
 * useApiTokenRecovery loads the key choices while the active session awaits
 * an API key reset. Nothing is adopted without the user's choice. Another
 * session on the same endpoint may belong to another account, so only the
 * user can say whether its key is the right one.
 * It returns the choices, and adopt(sourceSessionId), which resolves to
 * false when that key is no longer a valid choice.
 */
export function useApiTokenRecovery({
  sessionId,
  config,
  updateConfig,
}: UseApiTokenRecoveryOptions) {
  const [choices, setChoices] = useState<ApiTokenChoice[]>([])
  const [reloadToken, setReloadToken] = useState(0)
  const pending = config.api_token_reset_required === true
  const apiBase = config.api_base
  const syncKey = config.sync_key

  useEffect(() => {
    let cancelled = false
    if (!pending) return
    listApiTokenChoices(sessionId, { apiBase, syncKey })
      .then((next) => {
        if (!cancelled) setChoices(next)
      })
      .catch((error: unknown) => {
        console.error(`[useApiTokenRecovery] listing failed: ${String(error)}`)
      })
    return () => {
      cancelled = true
    }
  }, [pending, sessionId, apiBase, syncKey, reloadToken])

  const adopt = useCallback(
    async (sourceSessionId: number): Promise<boolean> => {
      const token = await readAdoptableApiToken(sourceSessionId, {
        apiBase,
        syncKey,
      })
      if (!token) {
        setReloadToken((n) => n + 1)
        return false
      }
      await updateConfig({ api_token: token, api_token_reset_required: false })
      return true
    },
    [apiBase, syncKey, updateConfig],
  )

  return { choices: pending ? choices : [], adopt }
}
