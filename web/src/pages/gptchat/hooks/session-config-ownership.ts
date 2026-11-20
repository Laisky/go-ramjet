import { useCallback, useEffect, useRef, useState, type RefObject } from 'react'

import type { SessionConfig } from '../types'

/**
 * ConfigUpdateOwner is an in-memory session activation, never persisted.
 * isCurrent also binds discovery to the exact server list it started with.
 */
export interface ConfigUpdateOwner {
  sessionId: number
  isCurrent: (servers: SessionConfig['mcp_servers']) => boolean
}

/**
 * useSessionConfigOwnership invalidates old background work synchronously,
 * then publishes a fresh owner only when a session is applied to memory.
 */
export function useSessionConfigOwnership(
  configRef: RefObject<SessionConfig>,
  sessionIdRef: RefObject<number>,
) {
  const revision = useRef(0)
  const current = useRef<ConfigUpdateOwner | null>(null)
  const [owner, setOwner] = useState<ConfigUpdateOwner | null>(null)
  const invalidate = useCallback(() => {
    revision.current++
    current.current = null
  }, [])
  const activate = useCallback(
    (sessionId: number) => {
      revision.current++
      const next: ConfigUpdateOwner = {
        sessionId,
        isCurrent: (servers) =>
          current.current === next &&
          sessionIdRef.current === sessionId &&
          configRef.current.mcp_servers === servers,
      }
      current.current = next
      setOwner(next)
    },
    [configRef, sessionIdRef],
  )
  // Failed pre-transition storage work may resume the unchanged session, but
  // never revive its old token or supersede a later transition/activation.
  const transition = useCallback(
    async (work: () => Promise<void>) => {
      const config = configRef.current
      const sessionId = sessionIdRef.current
      invalidate()
      const ticket = revision.current
      try {
        await work()
      } catch (error) {
        if (
          revision.current === ticket &&
          configRef.current === config &&
          sessionIdRef.current === sessionId
        )
          activate(sessionId)
        throw error
      }
    },
    [activate, configRef, invalidate, sessionIdRef],
  )
  useEffect(() => invalidate, [invalidate])
  return [owner, invalidate, activate, transition] as const
}
