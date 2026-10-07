import { kvGet, kvSet, StorageKeys } from '@/utils/storage'
import { act, cleanup, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { useVersionCheck } from '../use-version-check'

vi.mock('@/utils/api', () => ({ getApiBase: () => '/gptchat' }))
vi.mock('@/utils/storage', () => ({
  kvGet: vi.fn(),
  kvSet: vi.fn(),
  StorageKeys: {
    VERSION_DATE: 'config_version_date',
    IGNORED_VERSION: 'config_ignored_version',
  },
}))

/** deferred exposes settlement controls for testing requests across unmount. */
function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason: Error) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

/** versionResponse builds the server response used by version polling. */
function versionResponse(version: string): Response {
  return Response.json({ Settings: [{ Key: 'vcs.time', Value: version }] })
}

describe('useVersionCheck lifecycle', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.mocked(kvGet).mockReset().mockResolvedValue(null)
    vi.mocked(kvSet).mockReset().mockResolvedValue(undefined)
  })

  afterEach(async () => {
    cleanup()
    await act(async () => {})
    vi.useRealTimers()
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('aborts an in-flight request and stops polling on unmount', async () => {
    const request = deferred<Response>()
    const fetchMock = vi.fn<typeof fetch>().mockReturnValue(request.promise)
    vi.stubGlobal('fetch', fetchMock)
    const { unmount } = renderHook(() => useVersionCheck())
    const signal = fetchMock.mock.calls[0]?.[1]?.signal

    expect(signal).toBeInstanceOf(AbortSignal)
    expect(signal?.aborted).toBe(false)
    unmount()
    expect(signal?.aborted).toBe(true)
    await act(async () => {
      request.reject(new DOMException('Aborted', 'AbortError'))
      await vi.advanceTimersByTimeAsync(3600000)
    })
    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(vi.getTimerCount()).toBe(0)
  })

  it('does not log a late request failure after unmount', async () => {
    const request = deferred<Response>()
    vi.stubGlobal('fetch', vi.fn<typeof fetch>().mockReturnValue(request.promise))
    const warning = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const { unmount } = renderHook(() => useVersionCheck())
    unmount()

    await act(async () => {
      request.reject(new Error('connection closed after navigation'))
    })
    expect(warning).not.toHaveBeenCalled()
    expect(kvSet).not.toHaveBeenCalled()
  })

  it('does not persist a response body that arrives after unmount', async () => {
    const body = deferred<{ Settings: Array<{ Key: string; Value: string }> }>()
    const response = versionResponse('unused')
    vi.spyOn(response, 'json').mockReturnValue(body.promise)
    vi.stubGlobal('fetch', vi.fn<typeof fetch>().mockResolvedValue(response))
    const { unmount } = renderHook(() => useVersionCheck())
    await act(async () => {})
    expect(response.json).toHaveBeenCalledTimes(1)
    unmount()

    await act(async () => {
      body.resolve({ Settings: [{ Key: 'vcs.time', Value: 'late' }] })
    })
    expect(kvSet).not.toHaveBeenCalled()
  })

  it('still records the running version, reports upgrades, and honors ignores', async () => {
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(versionResponse('v1'))
      .mockResolvedValueOnce(versionResponse('v2'))
      .mockResolvedValueOnce(versionResponse('v2'))
    vi.stubGlobal('fetch', fetchMock)
    const { result } = renderHook(() => useVersionCheck())
    await act(async () => {})
    expect(kvSet).toHaveBeenCalledWith(StorageKeys.VERSION_DATE, 'v1')
    expect(result.current.upgradeInfo).toBeNull()

    await act(async () => {
      await vi.advanceTimersByTimeAsync(3600000)
    })
    expect(result.current.upgradeInfo).toEqual({ from: 'v1', to: 'v2' })
    await act(async () => {
      await result.current.ignoreVersion('v2')
    })
    expect(kvSet).toHaveBeenCalledWith(StorageKeys.IGNORED_VERSION, 'v2')
    expect(result.current.upgradeInfo).toBeNull()
    vi.mocked(kvGet).mockResolvedValue('v2')
    await act(async () => {
      await vi.advanceTimersByTimeAsync(3600000)
    })
    expect(result.current.upgradeInfo).toBeNull()
    expect(fetchMock).toHaveBeenCalledTimes(3)
  })
})
