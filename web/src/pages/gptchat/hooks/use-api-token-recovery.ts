/**
 * React hook behind the API key reset alert: lists the keys other sessions
 * hold that the user may choose for the active session, and applies a
 * choice in a single update.
 */
import { useCallback, useEffect, useState } from 'react'

import type { SessionConfig } from '../types'
import {
  type ConfigUpdateHandler,
  restoringApiTokenNeedsRotation,
} from '../utils/credential-policy'
import {
  type ApiTokenChoice,
  listApiTokenChoices,
  readAdoptableApiToken,
} from './session-credentials'

/**
 * UseApiTokenRecoveryOptions configures useApiTokenRecovery: sessionId and
 * config describe the active session, updateConfig stores an update, and
 * rotateSyncKey moves the cloud backup to a new sync key (see
 * useSyncKeyRotation); it is needed only to restore a key that is also the
 * sync key.
 */
export interface UseApiTokenRecoveryOptions {
  sessionId: number
  config: SessionConfig
  updateConfig: ConfigUpdateHandler
  rotateSyncKey?: (oldKey: string) => Promise<unknown>
}

/**
 * useApiTokenRecovery loads the key choices while the active session awaits
 * an API key reset. Nothing is adopted without the user's choice. Another
 * session on the same endpoint may belong to another account, so only the
 * user can say whether its key is the right one.
 * It returns the choices; adopt(sourceSessionId), which resolves to false
 * when that key is no longer a valid choice; needsRotation, which tells
 * whether restoring the set-aside key rotates the sync key first; and
 * restorePrevious(), which restores the key the reset set aside
 * (api_token_before_reset), rotating the sync key first when the two are
 * still equal. restorePrevious resolves to false when there is no such key
 * and rejects when rotating or storing fails, leaving the reset pending.
 */
export function useApiTokenRecovery({
  sessionId,
  config,
  updateConfig,
  rotateSyncKey,
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

  const previousToken = config.api_token_before_reset?.trim() || ''
  const needsRotation = restoringApiTokenNeedsRotation(config)

  const restorePrevious = useCallback(async (): Promise<boolean> => {
    if (!previousToken) return false
    if (needsRotation) {
      if (!rotateSyncKey) {
        throw new Error('restoring this API key needs a sync key rotation')
      }
      await rotateSyncKey(syncKey || '')
    }
    const { rejections } = await updateConfig({ api_token: previousToken })
    if (rejections.length > 0) {
      throw new Error(
        `the API key was refused: ${rejections.map((r) => r.reason).join(',')}`,
      )
    }
    return true
  }, [needsRotation, previousToken, rotateSyncKey, syncKey, updateConfig])

  return {
    choices: pending ? choices : [],
    adopt,
    needsRotation,
    restorePrevious,
  }
}
