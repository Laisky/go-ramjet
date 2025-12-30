import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { fetchTTS } from '../api'

describe('fetchTTS', () => {
  const originalFetch = globalThis.fetch
  const originalCreateObjectURL = URL.createObjectURL

  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    globalThis.fetch = originalFetch
    URL.createObjectURL = originalCreateObjectURL
  })

  it('should fetch TTS audio and return blob URL', async () => {
    const mockBlob = new Blob(['audio data'], { type: 'audio/wav' })
    const mockBlobUrl = 'blob:http://localhost/mock-audio-url'

    const mockResponse = {
      ok: true,
      blob: vi.fn().mockResolvedValue(mockBlob),
    }

    globalThis.fetch = vi.fn().mockResolvedValue(mockResponse)
    URL.createObjectURL = vi.fn().mockReturnValue(mockBlobUrl)

    const result = await fetchTTS('Hello world', 'test-api-token')

    expect(result).toBe(mockBlobUrl)
    expect(globalThis.fetch).toHaveBeenCalledWith(
      expect.stringContaining('/audio/tts'),
      { method: 'GET' },
    )
    expect(URL.createObjectURL).toHaveBeenCalled()
  })

  it('should include apikey and text in URL', async () => {
    const mockBlob = new Blob(['audio data'], { type: 'audio/wav' })
    const mockBlobUrl = 'blob:http://localhost/mock-audio-url'

    const mockResponse = {
      ok: true,
      blob: vi.fn().mockResolvedValue(mockBlob),
    }

    globalThis.fetch = vi.fn().mockResolvedValue(mockResponse)
    URL.createObjectURL = vi.fn().mockReturnValue(mockBlobUrl)

    await fetchTTS('Test message', 'my-token')

    const fetchCall = (globalThis.fetch as ReturnType<typeof vi.fn>).mock
      .calls[0]
    const url = fetchCall[0] as string

    expect(url).toContain('apikey=my-token')
    expect(url).toContain('text=Test%20message')
  })

  it('should throw error on non-ok response', async () => {
    const errorText = 'Unauthorized'
    const mockResponse = {
      ok: false,
      status: 401,
      text: vi.fn().mockResolvedValue(errorText),
    }

    globalThis.fetch = vi.fn().mockResolvedValue(mockResponse)

    await expect(fetchTTS('Hello world', 'bad-token')).rejects.toThrow(
      'TTS request failed [401]: Unauthorized',
    )
  })

  it('should URL-encode special characters in text', async () => {
    const mockBlob = new Blob(['audio data'], { type: 'audio/wav' })
    const mockBlobUrl = 'blob:http://localhost/mock-audio-url'

    const mockResponse = {
      ok: true,
      blob: vi.fn().mockResolvedValue(mockBlob),
    }

    globalThis.fetch = vi.fn().mockResolvedValue(mockResponse)
    URL.createObjectURL = vi.fn().mockReturnValue(mockBlobUrl)

    await fetchTTS('Hello & goodbye?', 'test-token')

    const fetchCall = (globalThis.fetch as ReturnType<typeof vi.fn>).mock
      .calls[0]
    const url = fetchCall[0] as string

    // Check that special characters are encoded
    expect(url).toContain('text=Hello%20%26%20goodbye%3F')
  })

  it('should handle empty text', async () => {
    const mockBlob = new Blob(['audio data'], { type: 'audio/wav' })
    const mockBlobUrl = 'blob:http://localhost/mock-audio-url'

    const mockResponse = {
      ok: true,
      blob: vi.fn().mockResolvedValue(mockBlob),
    }

    globalThis.fetch = vi.fn().mockResolvedValue(mockResponse)
    URL.createObjectURL = vi.fn().mockReturnValue(mockBlobUrl)

    const result = await fetchTTS('', 'test-token')

    expect(result).toBe(mockBlobUrl)
    const fetchCall = (globalThis.fetch as ReturnType<typeof vi.fn>).mock
      .calls[0]
    const url = fetchCall[0] as string
    expect(url).toContain('text=')
  })

  it('should create WAV blob from response', async () => {
    const mockBlob = new Blob(['audio data'], {
      type: 'application/octet-stream',
    })
    const mockBlobUrl = 'blob:http://localhost/mock-audio-url'

    const mockResponse = {
      ok: true,
      blob: vi.fn().mockResolvedValue(mockBlob),
    }

    globalThis.fetch = vi.fn().mockResolvedValue(mockResponse)

    let capturedBlob: Blob | undefined
    URL.createObjectURL = vi.fn((blob: Blob) => {
      capturedBlob = blob
      return mockBlobUrl
    })

    await fetchTTS('Hello world', 'test-token')

    expect(capturedBlob).toBeDefined()
    expect(capturedBlob!.type).toBe('audio/wav')
  })
})
