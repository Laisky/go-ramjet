import { useEffect, useRef } from 'react'
import type { McpServerConfig, SessionConfig } from '../types'
import { syncMCPServerTools } from '../utils/mcp'
import type { ConfigUpdateOwner } from './session-config-ownership'

/**
 * useMcpSync discovers tools for one rendered session activation/server list.
 * Cleanup saves requests, while the forwarded guard prevents stale writes even
 * before React cleans up or when a transport ignores its abort signal.
 */
export function useMcpSync(
  config: SessionConfig,
  configLoading: boolean,
  updateConfig: (
    updates: Partial<SessionConfig>,
    canApply?: () => boolean,
  ) => void,
  owner?: ConfigUpdateOwner | null,
) {
  const attempts = useRef<{
    owner: ConfigUpdateOwner | null | undefined
    completed: WeakSet<McpServerConfig>
  }>({ owner, completed: new WeakSet() })

  useEffect(() => {
    if (configLoading || !config.mcp_servers) return
    if (attempts.current.owner !== owner) {
      attempts.current = { owner, completed: new WeakSet() }
    }
    const completed = attempts.current.completed
    const servers = config.mcp_servers
    const controller = new AbortController()
    let active = true
    const canApply = () =>
      active &&
      !controller.signal.aborted &&
      (owner === undefined || (owner !== null && owner.isCurrent(servers)))

    const syncMetrics = async () => {
      let hasUpdates = false
      const successful: McpServerConfig[] = []
      const updatedServers = [...servers]
      for (let i = 0; i < updatedServers.length; i++) {
        const server = updatedServers[i]
        if (
          server.enabled &&
          (!server.tools || server.tools.length === 0) &&
          !completed.has(server)
        ) {
          if (!canApply()) return
          try {
            const { updatedServer } = await syncMCPServerTools(
              server,
              controller.signal,
            )
            if (!canApply()) return
            completed.add(updatedServer)
            successful.push(server)
            updatedServers[i] = updatedServer
            hasUpdates = true
            console.log(`[MCP] Auto-synced tools for ${server.name}`)
          } catch (error) {
            if (!canApply()) return
            completed.add(server)
            console.warn(
              `[MCP] Failed to auto-sync ${server.name}: ${String(error)}`,
            )
          }
        }
      }
      if (hasUpdates && canApply()) {
        // Mark originals only at whole-batch admission: discarded batches
        // remain retryable, but an admitted write's rollback cannot loop.
        for (const server of successful) completed.add(server)
        updateConfig({ mcp_servers: updatedServers }, canApply)
      }
    }
    void syncMetrics()
    return () => {
      active = false
      controller.abort()
    }
  }, [configLoading, config.mcp_servers, owner, updateConfig])
}
