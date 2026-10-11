import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { getSHA1 } from '@/utils/api'
import { api } from '../api'

vi.mock('@/utils/api', () => ({
  getApiBase: () => '/gptchat',
  getSHA1: vi.fn(async () => 'synthetic-user-id'),
}))

const operations = [
  {
    name: 'uploadDataset',
    method: 'POST',
    endpoint: '/ramjet/gptchat/files',
    invoke: (token: string, base?: string) =>
      api.uploadDataset(
        new File(['synthetic dataset'], 'sample.md'),
        'synthetic-dataset',
        'synthetic-data-key',
        token,
        base,
      ),
  },
  {
    name: 'listDatasets',
    method: 'GET',
    endpoint: '/ramjet/gptchat/files',
    invoke: (token: string, base?: string) =>
      api.listDatasets('synthetic-data-key', token, base),
  },
  {
    name: 'deleteDataset',
    method: 'DELETE',
    endpoint: '/ramjet/gptchat/files',
    invoke: (token: string, base?: string) =>
      api.deleteDataset('synthetic-dataset', 'synthetic-data-key', token, base),
  },
  {
    name: 'listChatbots',
    method: 'GET',
    endpoint: '/ramjet/gptchat/ctx/list',
    invoke: (token: string, base?: string) =>
      api.listChatbots('synthetic-data-key', token, base),
  },
  {
    name: 'setActiveChatbot',
    method: 'POST',
    endpoint: '/ramjet/gptchat/ctx/active',
    invoke: (token: string, base?: string) =>
      api.setActiveChatbot('synthetic-data-key', 'synthetic-bot', token, base),
  },
]

const invalidKeys = [
  { name: 'missing', token: '' },
  { name: 'whitespace only', token: ' \t' },
  { name: 'anonymous free tier', token: 'FREETIER-synthetic-key' },
  { name: 'legacy proxy placeholder', token: 'DEFAULT_PROXY_TOKEN' },
  { name: 'leading whitespace', token: ' synthetic-key' },
  { name: 'trailing whitespace', token: 'synthetic-key ' },
  { name: 'embedded space', token: 'synthetic key' },
  { name: 'embedded tab', token: 'synthetic\tkey' },
  { name: 'embedded newline', token: 'synthetic\nkey' },
  { name: 'embedded control character', token: 'synthetic\u0000key' },
  { name: 'embedded Unicode control character', token: 'synthetic\u0080key' },
]

describe('Ramjet dataset BYOK forwarding', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: true,
        json: async () => ({ datasets: [], chatbots: [] }),
      }),
    )
  })

  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  describe.each(operations)('$name', ({ invoke, endpoint, method }) => {
    it.each(invalidKeys)(
      'rejects $name before any request and keeps the key out of errors and logs',
      async ({ token }) => {
        const debug = vi.spyOn(console, 'debug').mockImplementation(() => {})
        const info = vi.spyOn(console, 'info').mockImplementation(() => {})
        const log = vi.spyOn(console, 'log').mockImplementation(() => {})
        const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
        const error = vi.spyOn(console, 'error').mockImplementation(() => {})

        await expect(
          invoke(token, 'http://100.64.0.10:8080/provider'),
        ).rejects.toThrow(
          'A valid BYOK API key is required for Ramjet operations.',
        )
        expect(fetch).not.toHaveBeenCalled()
        expect(getSHA1).not.toHaveBeenCalled()
        for (const logger of [debug, info, log, warn, error]) {
          expect(logger).not.toHaveBeenCalled()
        }
      },
    )

    it.each([
      'synthetic-provider-key',
      'sk-synthetic-key',
      'laisky-synthetic-key',
    ])(
      'forwards valid key %s and the selected endpoint unchanged',
      async (token) => {
        const base = 'http://100.64.0.10:8080/provider'
        await invoke(token, base)

        expect(getSHA1).toHaveBeenCalledWith(token)
        expect(fetch).toHaveBeenCalledExactlyOnceWith(
          `/gptchat${endpoint}`,
          expect.objectContaining({
            method,
            headers: expect.objectContaining({
              Authorization: `Bearer ${token}`,
              'X-Laisky-User-Id': 'synthetic-user-id',
              'X-Laisky-Api-Base': base,
            }),
          }),
        )
      },
    )

    it('omits an unset endpoint without replacing the supplied key', async () => {
      const token = 'synthetic-provider-key'
      await invoke(token)

      const [, options] = vi.mocked(fetch).mock.calls[0]
      const headers = options?.headers as Record<string, string>
      expect(headers.Authorization).toBe(`Bearer ${token}`)
      expect(headers).not.toHaveProperty('X-Laisky-Api-Base')
    })
  })
})
