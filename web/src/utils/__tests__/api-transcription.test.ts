import { afterEach, describe, expect, it, vi } from 'vitest'

import { transcribeAudio } from '../api'
import { usesOneAPIRelay } from '@/pages/gptchat/utils/api-token'

describe('GPTChat transcription credential routing', () => {
  afterEach(() => vi.unstubAllGlobals())

  it.each([
    ['FREETIER-synthetic-test', undefined, '/audio/transcriptions', undefined],
    ['DEFAULT_PROXY_TOKEN', undefined, '/audio/transcriptions', undefined],
    [
      'sk-synthetic-own-key',
      undefined,
      '/oneapi/v1/audio/transcriptions',
      undefined,
    ],
    [
      'sk-synthetic-own-key',
      'https://oneapi.laisky.com/',
      '/oneapi/v1/audio/transcriptions',
      undefined,
    ],
    [
      'sk-synthetic-provider-key',
      'https://provider.invalid',
      '/audio/transcriptions',
      'https://provider.invalid',
    ],
  ])(
    'isolates the credential and preserves multipart audio',
    async (key, base, path, forwardedBase) => {
      const response = vi
        .fn<(path: string, init: RequestInit) => Promise<Response>>()
        .mockResolvedValue(Response.json({ text: 'synthetic transcript' }))
      vi.stubGlobal('fetch', response)
      vi.stubGlobal('window', { location: { pathname: '/gptchat' } })
      const file = new File(['audio fixture'], 'sample.webm')
      await expect(transcribeAudio(file, key, base)).resolves.toBe(
        'synthetic transcript',
      )
      expect(response).toHaveBeenCalledTimes(1)
      const [actualPath, init] = response.mock.calls[0]
      expect(actualPath).toBe('/gptchat' + path)
      const headers = new Headers(init.headers)
      expect(headers.get('Authorization')).toBe(`Bearer ${key}`)
      expect(headers.get('X-Laisky-Api-Base')).toBe(forwardedBase ?? null)
      expect(headers.get('X-Laisky-User-Id')).toBeNull()
      expect((init.body as FormData).get('model')).toBe('whisper-1')
    },
  )

  it('limits One API cost lookup eligibility to the One API route', () => {
    expect(usesOneAPIRelay('https://api.openai.com')).toBe(true)
    expect(usesOneAPIRelay('https://oneapi.laisky.com/')).toBe(true)
    expect(usesOneAPIRelay('https://provider.invalid')).toBe(false)
    expect(usesOneAPIRelay('https://oneapi.laisky.com/another-base')).toBe(
      false,
    )
  })
})
