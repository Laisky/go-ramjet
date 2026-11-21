import { renderHook } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { DefaultSessionConfig } from '../../types'
import { useMcpSync } from '../use-mcp-sync'

/**
 * stubMcpServer stubs fetch with an MCP server that lists one tool. Like real
 * fetch, it rejects a request whose signal is aborted, and only counts the
 * requests it actually receives. Every initialize request is held until the
 * returned release function is called.
 */
function stubMcpServer() {
  const heldInitializes: Array<() => void> = []
  const received: string[] = []
  const fetchMock = vi.fn(
    async (_input: RequestInfo | URL, init?: RequestInit) => {
      const signal = init?.signal
      signal?.throwIfAborted()
      const method = JSON.parse(String(init?.body)).method as string
      received.push(method)
      if (method === 'initialize') {
        await new Promise<void>((resolve, reject) => {
          heldInitializes.push(resolve)
          signal?.addEventListener('abort', () => reject(signal.reason))
        })
        return Response.json({ jsonrpc: '2.0', id: 0, result: {} })
      }
      if (method === 'notifications/initialized') {
        return new Response(null, { status: 202 })
      }
      return Response.json({
        jsonrpc: '2.0',
        id: 1,
        result: {
          tools: [{ name: 'web_search', inputSchema: { type: 'object' } }],
        },
      })
    },
  )
  vi.stubGlobal('fetch', fetchMock)
  return {
    received,
    release: () => heldInitializes.splice(0).forEach((resolve) => resolve()),
  }
}

describe('useMcpSync', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('syncs the tools of an enabled server while mounted', async () => {
    const server = stubMcpServer()
    const updateConfig = vi.fn()
    renderHook(() => useMcpSync(DefaultSessionConfig, false, updateConfig))

    await vi.waitFor(() => expect(server.received).toEqual(['initialize']))
    server.release()

    await vi.waitFor(() => expect(updateConfig).toHaveBeenCalledTimes(1))
    expect(server.received).toEqual([
      'initialize',
      'notifications/initialized',
      'tools/list',
    ])
    const [{ mcp_servers }] = updateConfig.mock.calls[0]
    expect(mcp_servers[0].tools.map((t: { name: string }) => t.name)).toEqual([
      'web_search',
    ])
  })

  it('syncs once under StrictMode, whose remount aborts the first sync', async () => {
    vi.spyOn(console, 'warn').mockImplementation(() => undefined)
    const server = stubMcpServer()
    const updateConfig = vi.fn()
    renderHook(() => useMcpSync(DefaultSessionConfig, false, updateConfig), {
      reactStrictMode: true,
    })

    await vi.waitFor(() => expect(server.received).toContain('initialize'))
    server.release()

    await vi.waitFor(() => expect(updateConfig).toHaveBeenCalledTimes(1))
    await new Promise((resolve) => setTimeout(resolve, 50))
    expect(updateConfig).toHaveBeenCalledTimes(1)
    expect(server.received.filter((m) => m === 'tools/list')).toEqual([
      'tools/list',
    ])
  })

  it('sends no request and writes no config after unmount', async () => {
    vi.spyOn(console, 'warn').mockImplementation(() => undefined)
    const server = stubMcpServer()
    const updateConfig = vi.fn()
    const { unmount } = renderHook(() =>
      useMcpSync(DefaultSessionConfig, false, updateConfig),
    )

    await vi.waitFor(() => expect(server.received).toEqual(['initialize']))
    unmount()
    server.release()
    await new Promise((resolve) => setTimeout(resolve, 50))

    expect(server.received).toEqual(['initialize'])
    expect(updateConfig).not.toHaveBeenCalled()
  })
})
