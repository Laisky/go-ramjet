import { useEffect } from 'react'
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
  useEffect(() => {
    if (configLoading || !config.mcp_servers) return

    const syncMetrics = async () => {
      let hasUpdates = false
      const updatedServers = [...(config.mcp_servers || [])]

      for (let i = 0; i < updatedServers.length; i++) {
        const srv = updatedServers[i]
        // If enabled and no tools, try to sync
        if (srv.enabled && (!srv.tools || srv.tools.length === 0)) {
          try {
            const { updatedServer } = await syncMCPServerTools(srv)
            updatedServers[i] = updatedServer
            hasUpdates = true
            console.log(`[MCP] Auto-synced tools for ${srv.name}`)
          } catch (e) {
            console.warn(`[MCP] Failed to auto-sync ${srv.name}:`, e)
          }
        }
      }

      if (hasUpdates) {
        updateConfig({ mcp_servers: updatedServers })
      }
    }

    syncMetrics()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [configLoading])
}
