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
    requiresByok: true,
    method: 'POST',
    endpoint: '/ramjet/gptchat/files',
    invoke: (
      token: string,
      base?: string,
      session = 'synthetic-original-session',
    ) =>
      api.uploadDataset(
        new File(['synthetic dataset'], 'sample.md'),
        'synthetic-dataset',
        'synthetic-data-key',
        token,
        base,
        session,
      ),
  },
  {
    name: 'listDatasets',
    requiresByok: false,
    method: 'GET',
    endpoint: '/ramjet/gptchat/files',
    invoke: (
      token: string,
      base?: string,
      session = 'synthetic-original-session',
    ) => api.listDatasets('synthetic-data-key', token, base, session),
  },
  {
    name: 'deleteDataset',
    requiresByok: false,
    method: 'DELETE',
    endpoint: '/ramjet/gptchat/files',
    invoke: (
      token: string,
      base?: string,
      session = 'synthetic-original-session',
    ) =>
      api.deleteDataset(
        'synthetic-dataset',
        'synthetic-data-key',
        token,
        base,
        session,
      ),
  },
  {
    name: 'listChatbots',
    requiresByok: false,
    method: 'GET',
    endpoint: '/ramjet/gptchat/ctx/list',
    invoke: (
      token: string,
      base?: string,
      session = 'synthetic-original-session',
    ) => api.listChatbots('synthetic-data-key', token, base, session),
  },
  {
    name: 'setActiveChatbot',
    requiresByok: true,
    method: 'POST',
    endpoint: '/ramjet/gptchat/ctx/active',
    invoke: (
      token: string,
      base?: string,
      session = 'synthetic-original-session',
    ) =>
      api.setActiveChatbot(
        'synthetic-data-key',
        'synthetic-bot',
        token,
        base,
        session,
      ),
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
        text: async () => '{}',
      }),
    )
  })

  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  describe.each(operations)(
    '$name',
    ({ invoke, endpoint, method, requiresByok }) => {
      it.each(invalidKeys)(
        'applies the model-key requirement for $name without leaking credentials',
        async ({ token }) => {
          if (!requiresByok) {
            await invoke(token, 'http://100.64.0.10:8080/provider')
            const [, options] = vi.mocked(fetch).mock.calls[0]
            const headers = options?.headers as Record<string, string>
            expect(headers['X-Laisky-SSO-Token']).toBe(
              'Bearer synthetic-original-session',
            )
            expect(headers).not.toHaveProperty('Authorization')
            expect(headers).not.toHaveProperty('X-Laisky-User-Id')
            expect(headers).not.toHaveProperty('X-Laisky-Api-Base')
            expect(getSHA1).not.toHaveBeenCalled()
            return
          }
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

          if (requiresByok) expect(getSHA1).toHaveBeenCalledWith(token)
          else expect(getSHA1).not.toHaveBeenCalled()
          expect(fetch).toHaveBeenCalledExactlyOnceWith(
            `/gptchat${endpoint}`,
            expect.objectContaining({
              method,
              redirect: 'error',
              mode: 'same-origin',
              headers: expect.objectContaining({
                ...(requiresByok
                  ? {
                      Authorization: 'Bearer ' + token,
                      'X-Laisky-User-Id': 'synthetic-user-id',
                      'X-Laisky-Api-Base': base,
                    }
                  : {}),
                'X-Laisky-SSO-Token': 'Bearer synthetic-original-session',
              }),
            }),
          )
        },
      )

      it('rejects a missing separate SSO session before any fetch', async () => {
        await expect(
          invoke('synthetic-provider-key', undefined, ''),
        ).rejects.toThrow('Sign in with Laisky SSO to manage private datasets.')
        expect(fetch).not.toHaveBeenCalled()
      })

      it('omits an unset endpoint without replacing the supplied key', async () => {
        const token = 'synthetic-provider-key'
        await invoke(token)

        const [, options] = vi.mocked(fetch).mock.calls[0]
        const headers = options?.headers as Record<string, string>
        if (requiresByok) expect(headers.Authorization).toBe('Bearer ' + token)
        else expect(headers).not.toHaveProperty('Authorization')
        expect(headers).not.toHaveProperty('X-Laisky-Api-Base')
      })
    },
  )
  it('does not attach an ownership session to generic user or sync requests', async () => {
    localStorage.setItem(
      'gptchat_ownership_sso_token',
      'stored-private-session',
    )
    await api.fetchCurrentUser('model-key')
    await api.uploadUserData('sync-key', { synthetic: true })
    await api.downloadUserData('sync-key')
    for (const [, options] of vi.mocked(fetch).mock.calls) {
      expect(options?.headers).not.toHaveProperty('X-Laisky-SSO-Token')
      expect(options).not.toHaveProperty('redirect')
      expect(options).not.toHaveProperty('mode')
    }
    localStorage.removeItem('gptchat_ownership_sso_token')
  })
})
