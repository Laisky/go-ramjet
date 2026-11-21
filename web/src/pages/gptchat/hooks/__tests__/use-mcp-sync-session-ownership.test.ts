import { StrictMode, createElement } from 'react'
import { act, renderHook, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import {
  installStore,
  kvGet,
  kvSet,
  sessionConfig,
  storedConfig,
  SYNC_KEY,
} from '../../__tests__/fixtures/credential-store'
import type { McpServerConfig } from '../../types'
import { syncMCPServerTools } from '../../utils/mcp'
import { useConfig } from '../use-config'
import { useConfigHandlers } from '../use-config-handlers'
import { useMcpSync } from '../use-mcp-sync'

vi.mock(
  '@/utils/storage',
  () => import('../../__tests__/fixtures/storage-mock'),
)
vi.mock('@/pages/gptchat/utils/migration', () => ({
  migrateLegacyData: vi.fn().mockResolvedValue(undefined),
}))
vi.mock('@/utils/api', () => ({ getApiBase: () => 'http://localhost:24456' }))
vi.mock('@/pages/gptchat/utils/mcp', () => ({ syncMCPServerTools: vi.fn() }))

/** serverConfig builds an ordinary synthetic discovery endpoint. */
function serverConfig(id: string, ready = false): McpServerConfig {
  return {
    id,
    name: `Ordinary server ${id}`,
    url: `https://mcp-${id}.example`,
    enabled: true,
    tools: ready ? [{ name: `ready_${id}` }] : [],
  }
}

/** useBackgroundConfig exposes the actual page's guarded background-write chain. */
function useBackgroundConfig() {
  const state = useConfig()
  const { applyConfigInBackground } = useConfigHandlers({
    config: state.config,
    updateConfig: state.updateConfig,
    exportAllData: state.exportAllData,
    importAllData: state.importAllData,
  })
  return { ...state, applyConfigInBackground }
}

/** useSyncedConfig adds discovery to the real background persistence chain. */
function useSyncedConfig() {
  const state = useBackgroundConfig()
  useMcpSync(
    state.config,
    state.isLoading,
    state.applyConfigInBackground,
    state.configUpdateOwner,
  )
  return state
}

/** pendingDiscovery models a transport that can finish after cancellation. */
function pendingDiscovery() {
  let resolve!: (value: Awaited<ReturnType<typeof syncMCPServerTools>>) => void
  const promise = new Promise<Awaited<ReturnType<typeof syncMCPServerTools>>>(
    (done) => {
      resolve = done
    },
  )
  return {
    promise,
    finish: (server: McpServerConfig, name = 'discovered') =>
      resolve({ updatedServer: { ...server, tools: [{ name }] }, count: 1 }),
  }
}

/** seedSessions isolates two ordinary stored sessions for each hook instance. */
function seedSessions(servers: McpServerConfig[] = [serverConfig('a')]) {
  return installStore({
    config_selected_session: 1,
    config_sync_key: SYNC_KEY,
    chat_user_config_1: sessionConfig({ mcp_servers: servers }),
    chat_user_config_2: sessionConfig({
      mcp_servers: [serverConfig('b', true)],
    }),
  })
}

/** flushCompletion drains discovery and its persistence continuation. */
async function flushCompletion(finish: () => void) {
  await act(async () => {
    finish()
    await new Promise((resolve) => setTimeout(resolve, 0))
  })
}

describe('MCP discovery session ownership', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    vi.clearAllMocks()
    window.history.replaceState({}, '', '/')
  })

  it('does not write delayed A discovery into B when transport ignores abort', async () => {
    const a = serverConfig('a')
    const b = serverConfig('b', true)
    const store = installStore({
      config_selected_session: 1,
      config_sync_key: SYNC_KEY,
      chat_user_config_1: sessionConfig({ mcp_servers: [a] }),
      chat_user_config_2: sessionConfig({ mcp_servers: [b] }),
    })
    let releaseA: () => void = () => undefined
    vi.mocked(syncMCPServerTools).mockImplementation(
      (server) =>
        new Promise((resolve) => {
          releaseA = () =>
            resolve({
              updatedServer: { ...server, tools: [{ name: 'discovered_a' }] },
              count: 1,
            })
        }),
    )
    const { result } = renderHook(useSyncedConfig)
    await waitFor(() => expect(result.current.isLoading).toBe(false))
    await waitFor(() => expect(syncMCPServerTools).toHaveBeenCalledTimes(1))
    await act(async () => {
      await result.current.switchSession(2)
    })
    expect(result.current.sessionId).toBe(2)
    await act(async () => {
      releaseA()
      await new Promise((resolve) => setTimeout(resolve, 0))
    })
    expect(result.current.config.mcp_servers).toEqual([b])
    expect(storedConfig(store, 2).mcp_servers).toEqual([b])
    expect(storedConfig(store, 1).mcp_servers).toEqual([a])
  })

  it('rejects A completion while B hydration is blocked before the B render', async () => {
    const store = seedSessions()
    const pending = pendingDiscovery()
    vi.mocked(syncMCPServerTools).mockReturnValue(pending.promise)
    const { result } = renderHook(useSyncedConfig)
    await waitFor(() => expect(syncMCPServerTools).toHaveBeenCalledTimes(1))
    const originalGet = kvGet.getMockImplementation()!
    let releaseHydration!: () => void
    let hydrationStarted = false
    kvGet.mockImplementation((key: string) => {
      if (key === 'chat_user_config_2' && !hydrationStarted) {
        hydrationStarted = true
        return new Promise((resolve) => {
          releaseHydration = () => resolve(store[key])
        })
      }
      return originalGet(key)
    })
    let switching!: Promise<void>
    act(() => {
      switching = result.current.switchSession(2)
    })
    await waitFor(() => expect(hydrationStarted).toBe(true))
    expect(result.current.sessionId).toBe(1)
    await flushCompletion(() => pending.finish(serverConfig('a')))
    expect(storedConfig(store, 2).mcp_servers).toEqual([
      serverConfig('b', true),
    ])
    await act(async () => {
      releaseHydration()
      await switching
    })
    expect(result.current.config.mcp_servers).toEqual([serverConfig('b', true)])
  })

  it('keeps valid discovery after an unrelated same-session config edit', async () => {
    const store = seedSessions()
    const pending = pendingDiscovery()
    vi.mocked(syncMCPServerTools).mockReturnValue(pending.promise)
    const { result } = renderHook(useSyncedConfig)
    await waitFor(() => expect(syncMCPServerTools).toHaveBeenCalledTimes(1))
    await act(async () => {
      await result.current.updateConfig({
        system_prompt: 'Ordinary edited prompt',
      })
    })
    await flushCompletion(() => pending.finish(serverConfig('a')))
    await waitFor(() =>
      expect(storedConfig(store, 1).mcp_servers?.[0].tools).toEqual([
        { name: 'discovered' },
      ]),
    )
    expect(storedConfig(store, 1).system_prompt).toBe('Ordinary edited prompt')
    expect(syncMCPServerTools).toHaveBeenCalledTimes(1)
  })

  it('does not replace a same-session manual server edit with old discovery', async () => {
    const store = seedSessions()
    const pending = pendingDiscovery()
    vi.mocked(syncMCPServerTools).mockReturnValue(pending.promise)
    const { result } = renderHook(useSyncedConfig)
    await waitFor(() => expect(syncMCPServerTools).toHaveBeenCalledTimes(1))
    const edited = { ...serverConfig('a', true), url: 'https://edited.example' }
    await act(async () => {
      await result.current.updateConfig({ mcp_servers: [edited] })
    })
    await flushCompletion(() => pending.finish(serverConfig('a'), 'old_result'))
    expect(storedConfig(store, 1).mcp_servers).toEqual([edited])
  })

  it('requires a fresh activation when returning A to B to A', async () => {
    const store = seedSessions()
    const first = pendingDiscovery()
    const second = pendingDiscovery()
    vi.mocked(syncMCPServerTools)
      .mockReturnValueOnce(first.promise)
      .mockReturnValueOnce(second.promise)
    const { result } = renderHook(useSyncedConfig)
    await waitFor(() => expect(syncMCPServerTools).toHaveBeenCalledTimes(1))
    await act(async () => {
      await result.current.switchSession(2)
    })
    await act(async () => {
      await result.current.switchSession(1)
    })
    await waitFor(() => expect(syncMCPServerTools).toHaveBeenCalledTimes(2))
    await flushCompletion(() =>
      first.finish(serverConfig('a'), 'old_activation'),
    )
    expect(storedConfig(store, 1).mcp_servers?.[0].tools).toEqual([])
    await flushCompletion(() =>
      second.finish(serverConfig('a'), 'new_activation'),
    )
    await waitFor(() =>
      expect(storedConfig(store, 1).mcp_servers?.[0].tools).toEqual([
        { name: 'new_activation' },
      ]),
    )
  })

  it('does not loop on an empty result and retries a manually replaced server', async () => {
    const store = seedSessions()
    vi.mocked(syncMCPServerTools).mockImplementation(async (server) => ({
      updatedServer: { ...server, tools: [] },
      count: 0,
    }))
    const { result } = renderHook(useSyncedConfig)
    await waitFor(() => expect(syncMCPServerTools).toHaveBeenCalledTimes(1))
    await flushCompletion(() => undefined)
    expect(syncMCPServerTools).toHaveBeenCalledTimes(1)
    await act(async () => {
      await result.current.updateConfig({
        mcp_servers: [{ ...storedConfig(store, 1).mcp_servers![0] }],
      })
    })
    await waitFor(() => expect(syncMCPServerTools).toHaveBeenCalledTimes(2))
    await flushCompletion(() => undefined)
    expect(syncMCPServerTools).toHaveBeenCalledTimes(2)
  })

  it('retries a successful result discarded by a mid-batch server-list edit', async () => {
    const a = serverConfig('a')
    const c = serverConfig('c')
    const store = seedSessions([a, c])
    const blocked = pendingDiscovery()
    vi.mocked(syncMCPServerTools).mockImplementation(async (server) => {
      if (server.id === 'c') return blocked.promise
      return {
        updatedServer: { ...server, tools: [{ name: 'ready_a' }] },
        count: 1,
      }
    })
    const { result } = renderHook(useSyncedConfig)
    await waitFor(() => expect(syncMCPServerTools).toHaveBeenCalledTimes(2))
    const liveA = result.current.config.mcp_servers![0]
    await act(async () => {
      await result.current.updateConfig({
        mcp_servers: [liveA, { ...c, enabled: false }],
      })
    })
    await waitFor(() => expect(syncMCPServerTools).toHaveBeenCalledTimes(3))
    await waitFor(() =>
      expect(storedConfig(store, 1).mcp_servers?.[0].tools).toEqual([
        { name: 'ready_a' },
      ]),
    )
    await flushCompletion(() => blocked.finish(c))
    expect(storedConfig(store, 1).mcp_servers?.[1].enabled).toBe(false)
  })

  it('does not revive an old completion during StrictMode replay or final unmount', async () => {
    const store = seedSessions()
    const requests: ReturnType<typeof pendingDiscovery>[] = []
    vi.mocked(syncMCPServerTools).mockImplementation(() => {
      const request = pendingDiscovery()
      requests.push(request)
      return request.promise
    })
    const { result, unmount } = renderHook(useSyncedConfig, {
      wrapper: ({ children }) => createElement(StrictMode, null, children),
    })
    await waitFor(() => expect(result.current.isLoading).toBe(false))
    await waitFor(() => expect(requests.length).toBeGreaterThan(0))
    unmount()
    await flushCompletion(() =>
      requests.forEach((request) => request.finish(serverConfig('a'))),
    )
    expect(storedConfig(store, 1).mcp_servers?.[0].tools).toEqual([])
  })

  it('recovers with a fresh owner after selected-session storage fails', async () => {
    const store = seedSessions()
    const old = pendingDiscovery()
    const retry = pendingDiscovery()
    vi.mocked(syncMCPServerTools)
      .mockReturnValueOnce(old.promise)
      .mockReturnValueOnce(retry.promise)
    const { result } = renderHook(useSyncedConfig)
    await waitFor(() => expect(syncMCPServerTools).toHaveBeenCalledTimes(1))
    const originalSet = kvSet.getMockImplementation()!
    kvSet.mockImplementation((key: string, value: unknown) => {
      if (key === 'config_selected_session')
        return Promise.reject(new Error('Synthetic local storage unavailable'))
      return originalSet(key, value)
    })
    await act(async () => {
      await expect(result.current.switchSession(2)).rejects.toThrow(
        'Synthetic local storage unavailable',
      )
    })
    expect(result.current.sessionId).toBe(1)
    await waitFor(() => expect(syncMCPServerTools).toHaveBeenCalledTimes(2))
    await flushCompletion(() => old.finish(serverConfig('a'), 'obsolete'))
    expect(storedConfig(store, 1).mcp_servers?.[0].tools).toEqual([])
    await flushCompletion(() => retry.finish(serverConfig('a'), 'fresh_retry'))
    await waitFor(() =>
      expect(storedConfig(store, 1).mcp_servers?.[0].tools).toEqual([
        { name: 'fresh_retry' },
      ]),
    )
  })

  it('does not let an earlier failed switch supersede a newer activation', async () => {
    const store = seedSessions()
    const old = pendingDiscovery()
    vi.mocked(syncMCPServerTools).mockReturnValue(old.promise)
    const { result } = renderHook(useSyncedConfig)
    await waitFor(() => expect(syncMCPServerTools).toHaveBeenCalledTimes(1))
    const originalSet = kvSet.getMockImplementation()!
    let rejectFirst!: (error: Error) => void
    let first = true
    kvSet.mockImplementation((key: string, value: unknown) => {
      if (key === 'config_selected_session' && first) {
        first = false
        return new Promise((_, reject) => {
          rejectFirst = reject
        })
      }
      return originalSet(key, value)
    })
    let failed!: Promise<unknown>
    act(() => {
      failed = result.current.switchSession(2).catch((error: unknown) => error)
    })
    await act(async () => {
      await result.current.switchSession(2)
    })
    await act(async () => {
      rejectFirst(new Error('Synthetic first switch failure'))
      await failed
    })
    await flushCompletion(() => old.finish(serverConfig('a')))
    expect(result.current.sessionId).toBe(2)
    expect(result.current.config.mcp_servers).toEqual([serverConfig('b', true)])
    expect(storedConfig(store, 2).mcp_servers).toEqual([
      serverConfig('b', true),
    ])
    expect(syncMCPServerTools).toHaveBeenCalledTimes(1)
  })

  it('preserves an already-admitted A persistence write while switching to B', async () => {
    const store = seedSessions()
    const pending = pendingDiscovery()
    vi.mocked(syncMCPServerTools).mockReturnValue(pending.promise)
    const { result } = renderHook(useSyncedConfig)
    await waitFor(() => expect(syncMCPServerTools).toHaveBeenCalledTimes(1))
    const originalSet = kvSet.getMockImplementation()!
    let releaseWrite!: () => void
    let blocked = false
    kvSet.mockImplementation((key: string, value: unknown) => {
      if (key === 'chat_user_config_1' && !blocked) {
        blocked = true
        return new Promise((resolve) => {
          releaseWrite = () => {
            originalSet(key, value)
            resolve(undefined)
          }
        })
      }
      return originalSet(key, value)
    })
    await flushCompletion(() => pending.finish(serverConfig('a'), 'admitted_a'))
    await waitFor(() => expect(blocked).toBe(true))
    let switching!: Promise<void>
    act(() => {
      switching = result.current.switchSession(2)
    })
    await act(async () => {
      releaseWrite()
      await switching
    })
    expect(storedConfig(store, 1).mcp_servers?.[0].tools).toEqual([
      { name: 'admitted_a' },
    ])
    expect(storedConfig(store, 2).mcp_servers).toEqual([
      serverConfig('b', true),
    ])
    expect(result.current.sessionId).toBe(2)
  })

  it('does not repeatedly rediscover unchanged servers after local saving fails', async () => {
    const store = seedSessions()
    const pending = pendingDiscovery()
    vi.mocked(syncMCPServerTools).mockReturnValue(pending.promise)
    const { result } = renderHook(useSyncedConfig)
    await waitFor(() => expect(syncMCPServerTools).toHaveBeenCalledTimes(1))
    const originalSet = kvSet.getMockImplementation()!
    let failed = false
    let rejectWrite!: (error: Error) => void
    kvSet.mockImplementation((key: string, value: unknown) => {
      if (key === 'chat_user_config_1' && !failed) {
        failed = true
        return new Promise((_, reject) => {
          rejectWrite = reject
        })
      }
      return originalSet(key, value)
    })
    await flushCompletion(() => pending.finish(serverConfig('a')))
    await waitFor(() =>
      expect(result.current.config.mcp_servers?.[0].tools).toEqual([
        { name: 'discovered' },
      ]),
    )
    await flushCompletion(() =>
      rejectWrite(new Error('Synthetic save failure')),
    )
    expect(syncMCPServerTools).toHaveBeenCalledTimes(1)
    expect(result.current.config.mcp_servers?.[0].tools).toEqual([])
    expect(storedConfig(store, 1).mcp_servers?.[0].tools).toEqual([])
  })

  it('keeps discovery fenced after an import partly commits before failing', async () => {
    const store = seedSessions()
    const old = pendingDiscovery()
    vi.mocked(syncMCPServerTools).mockReturnValue(old.promise)
    const { result } = renderHook(useSyncedConfig)
    await waitFor(() => expect(syncMCPServerTools).toHaveBeenCalledTimes(1))
    const imported = serverConfig('imported', true)
    const originalSet = kvSet.getMockImplementation()!
    kvSet.mockImplementation((key: string, value: unknown) => {
      if (key === 'config_selected_session')
        return Promise.reject(new Error('Synthetic final import write failure'))
      return originalSet(key, value)
    })
    await act(async () => {
      await expect(
        result.current.importAllData(
          {
            chat_user_config_1: sessionConfig({ mcp_servers: [imported] }),
          },
          'download',
        ),
      ).rejects.toThrow('Synthetic final import write failure')
    })
    expect(storedConfig(store, 1).mcp_servers).toEqual([imported])
    await flushCompletion(() => old.finish(serverConfig('a'), 'obsolete'))
    expect(syncMCPServerTools).toHaveBeenCalledTimes(1)
    expect(storedConfig(store, 1).mcp_servers).toEqual([imported])
  })

  it('refuses a captured owner at persistence even without an effect cleanup guard', async () => {
    const store = seedSessions()
    const { result } = renderHook(useBackgroundConfig)
    await waitFor(() => expect(result.current.isLoading).toBe(false))
    const owner = result.current.configUpdateOwner!
    const servers = result.current.config.mcp_servers
    const originalGet = kvGet.getMockImplementation()!
    let releaseHydration!: () => void
    let hydrationStarted = false
    kvGet.mockImplementation((key: string) => {
      if (key === 'chat_user_config_2' && !hydrationStarted) {
        hydrationStarted = true
        return new Promise((resolve) => {
          releaseHydration = () => resolve(store[key])
        })
      }
      return originalGet(key)
    })
    let switching!: Promise<void>
    act(() => {
      switching = result.current.switchSession(2)
    })
    await waitFor(() => expect(hydrationStarted).toBe(true))
    expect(result.current.sessionId).toBe(1)
    await act(async () => {
      result.current.applyConfigInBackground(
        {
          mcp_servers: [
            { ...serverConfig('a'), tools: [{ name: 'obsolete' }] },
          ],
        },
        () => owner.isCurrent(servers),
      )
      await new Promise((resolve) => setTimeout(resolve, 0))
    })
    expect(storedConfig(store, 1).mcp_servers?.[0].tools).toEqual([])
    expect(storedConfig(store, 2).mcp_servers).toEqual([
      serverConfig('b', true),
    ])
    await act(async () => {
      releaseHydration()
      await switching
    })
    expect(result.current.config.mcp_servers).toEqual([serverConfig('b', true)])
  })
})
