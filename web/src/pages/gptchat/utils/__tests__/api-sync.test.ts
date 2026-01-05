import { beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '../api'

describe('api.downloadUserData', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
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
