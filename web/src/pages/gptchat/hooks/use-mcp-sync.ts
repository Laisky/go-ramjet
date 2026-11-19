import { useEffect, useRef } from 'react'
import type { SessionConfig } from '../types'
import { syncMCPServerTools } from '../utils/mcp'

/**
 * useMcpSync automatically syncs MCP server tools in the background.
 */
export function useMcpSync(
  config: SessionConfig,
  configLoading: boolean,
  updateConfig: (updates: Partial<SessionConfig>) => void,
) {
  // unmountRef aborts in-flight syncs when the component unmounts, so no
  // request is sent and no config is written after that. Dependency changes
  // do not abort a sync, which keeps the behavior while mounted unchanged.
  const unmountRef = useRef<AbortController | null>(null)
  useEffect(() => {
    const controller = new AbortController()
    unmountRef.current = controller
    return () => controller.abort()
  }, [])

  useEffect(() => {
    if (configLoading || !config.mcp_servers) return
    const signal = unmountRef.current?.signal

    const syncMetrics = async () => {
      let hasUpdates = false
      const updatedServers = [...(config.mcp_servers || [])]

      for (let i = 0; i < updatedServers.length; i++) {
        const srv = updatedServers[i]
        // If enabled and no tools, try to sync
        if (srv.enabled && (!srv.tools || srv.tools.length === 0)) {
          if (signal?.aborted) return
          try {
            const { updatedServer } = await syncMCPServerTools(srv, signal)
            updatedServers[i] = updatedServer
            hasUpdates = true
            console.log(`[MCP] Auto-synced tools for ${srv.name}`)
          } catch (e) {
            if (signal?.aborted) return
            console.warn(`[MCP] Failed to auto-sync ${srv.name}:`, e)
          }
        }
      }

      if (hasUpdates && !signal?.aborted) {
        updateConfig({ mcp_servers: updatedServers })
      }
    }

    syncMetrics()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [configLoading])
}
