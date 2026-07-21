import { beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '../api'

describe('api.downloadUserData', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('bypasses caches when fetching current user entitlements', async () => {
    vi.spyOn(Date, 'now').mockReturnValue(12345)
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      text: async () =>
        JSON.stringify({
          username: 'FREETIER-test',
          allowed_models: ['gpt-5.6-terra'],
        }),
    })
    vi.stubGlobal('fetch', fetchMock)

    const user = await api.fetchCurrentUser('FREETIER-test-token')

    expect(user.allowed_models).toContain('gpt-5.6-terra')
    expect(fetchMock).toHaveBeenCalledWith(
      '/user/me?_=12345',
      expect.objectContaining({
        cache: 'no-store',
        headers: expect.objectContaining({
          Authorization: 'Bearer FREETIER-test-token',
          'Cache-Control': 'no-cache',
          Pragma: 'no-cache',
        }),
      }),
    )
  })

  it('returns empty object when cloud config does not exist', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: false,
      status: 400,
      text: async () =>
        JSON.stringify({ err: 'read body: The specified key does not exist.' }),
    })
    vi.stubGlobal('fetch', fetchMock)

    const data = await api.downloadUserData('sync-test-key')
    expect(data).toEqual({})
  })

  it('rethrows other errors', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: false,
      status: 500,
      text: async () => 'internal error',
    })
    vi.stubGlobal('fetch', fetchMock)

    await expect(api.downloadUserData('sync-test-key')).rejects.toBeTruthy()
  })
})
