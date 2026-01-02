import { act, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { stripMarkdownForTTS, useTTS } from '../use-tts'

// Mock the API module
vi.mock('@/utils/api', () => ({
  fetchTTS: vi.fn(),
}))

import { fetchTTS } from '@/utils/api'

// Mock URL.revokeObjectURL for all tests
const originalRevokeObjectURL = URL.revokeObjectURL

describe('stripMarkdownForTTS', () => {
  it('should return empty string for non-string input', () => {
    expect(stripMarkdownForTTS(null as unknown as string)).toBe('')
    expect(stripMarkdownForTTS(undefined as unknown as string)).toBe('')
    expect(stripMarkdownForTTS(123 as unknown as string)).toBe('')
  })

  it('should keep code blocks content', () => {
    const text = 'Hello ```const x = 1;``` world'
    expect(stripMarkdownForTTS(text)).toBe('Hello const x = 1; world')
  })

  it('should keep inline code content', () => {
    const text = 'Use `console.log()` for debugging'
    expect(stripMarkdownForTTS(text)).toBe('Use console.log() for debugging')
  })

  it('should remove images but keep alt text', () => {
    const text = 'Check this ![cat image](http://example.com/cat.png) out'
    expect(stripMarkdownForTTS(text)).toBe('Check this out')
  })

  it('should convert links to just their text', () => {
    const text = 'Visit [Google](https://google.com) for search'
    expect(stripMarkdownForTTS(text)).toBe('Visit Google for search')
  })

  it('should remove markdown formatting characters', () => {
    const text = '**bold** and *italic* and ~strikethrough~'
    expect(stripMarkdownForTTS(text)).toBe('bold and italic and strikethrough')
  })

  it('should normalize whitespace', () => {
    const text = 'Hello    world\n\n\ntest'
    expect(stripMarkdownForTTS(text)).toBe('Hello world test')
  })

  it('should handle complex markdown', () => {
    const text = `
# Hello World

This is a **test** with \`code\` and [links](http://example.com).

\`\`\`javascript
const x = 1;
\`\`\`

More text here.
    `
    const result = stripMarkdownForTTS(text)
    expect(result).toBe(
      'Hello World This is a test with code and links. const x = 1; More text here.',
    )
  })
})

describe('useTTS', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    // Mock URL.revokeObjectURL
    URL.revokeObjectURL = vi.fn()
  })

  afterEach(() => {
    // Restore URL.revokeObjectURL
    URL.revokeObjectURL = originalRevokeObjectURL
  })

  it('should initialize with default state', () => {
    const { result } = renderHook(() => useTTS({ apiToken: 'test-token' }))

    expect(result.current.isLoading).toBe(false)
    expect(result.current.isPlaying).toBe(false)
    expect(result.current.error).toBe(null)
    expect(result.current.audioUrl).toBe(null)
  })

  it('should set error when apiToken is empty', async () => {
    const { result } = renderHook(() => useTTS({ apiToken: '' }))

    await act(async () => {
      await result.current.requestTTS('Hello world')
    })

    expect(result.current.error).toBe('API token is required for TTS')
    expect(result.current.audioUrl).toBe(null)
  })

  it('should set error when text is empty after stripping markdown', async () => {
    const { result } = renderHook(() => useTTS({ apiToken: 'test-token' }))

    await act(async () => {
      await result.current.requestTTS('![image](http://example.com)')
    })

    expect(result.current.error).toBe('No text content to speak')
    expect(result.current.audioUrl).toBe(null)
  })

  it('should fetch TTS audio successfully', async () => {
    const mockUrl = 'blob:http://localhost/test-audio'
    ;(fetchTTS as ReturnType<typeof vi.fn>).mockResolvedValue(mockUrl)

    const { result } = renderHook(() => useTTS({ apiToken: 'test-token' }))

    await act(async () => {
      await result.current.requestTTS('Hello world')
    })

    expect(fetchTTS).toHaveBeenCalledWith('Hello world', 'test-token')
    expect(result.current.audioUrl).toBe(mockUrl)
    expect(result.current.error).toBe(null)
  })

  it('should set loading state during request', async () => {
    const mockUrl = 'blob:http://localhost/test-audio'
    let resolvePromise: (value: string) => void
    const promise = new Promise<string>((resolve) => {
      resolvePromise = resolve
    })
    ;(fetchTTS as ReturnType<typeof vi.fn>).mockReturnValue(promise)

    const { result } = renderHook(() => useTTS({ apiToken: 'test-token' }))

    // Start the request
    let requestPromise: Promise<void>
    act(() => {
      requestPromise = result.current.requestTTS('Hello world')
    })

    // Should be loading
    expect(result.current.isLoading).toBe(true)

    // Resolve the promise
    await act(async () => {
      resolvePromise!(mockUrl)
      await requestPromise
    })

    // Should no longer be loading
    expect(result.current.isLoading).toBe(false)
    expect(result.current.audioUrl).toBe(mockUrl)
  })

  it('should handle API errors', async () => {
    const errorMessage = 'Server error'
    ;(fetchTTS as ReturnType<typeof vi.fn>).mockRejectedValue(
      new Error(errorMessage),
    )

    const onError = vi.fn()
    const { result } = renderHook(() =>
      useTTS({ apiToken: 'test-token', onError }),
    )

    await act(async () => {
      await result.current.requestTTS('Hello world')
    })

    expect(result.current.error).toBe(errorMessage)
    expect(result.current.audioUrl).toBe(null)
    expect(onError).toHaveBeenCalledWith(errorMessage)
  })

  it('should stop TTS and clear state', async () => {
    const mockUrl = 'blob:http://localhost/test-audio'
    ;(fetchTTS as ReturnType<typeof vi.fn>).mockResolvedValue(mockUrl)

    const { result } = renderHook(() => useTTS({ apiToken: 'test-token' }))

    // Request TTS
    await act(async () => {
      await result.current.requestTTS('Hello world')
    })

    expect(result.current.audioUrl).toBe(mockUrl)

    // Stop TTS
    act(() => {
      result.current.stopTTS()
    })

    expect(result.current.audioUrl).toBe(null)
    expect(result.current.isPlaying).toBe(false)
    expect(result.current.error).toBe(null)
  })

  it('should call onLoadStart and onLoadEnd callbacks', async () => {
    const mockUrl = 'blob:http://localhost/test-audio'
    ;(fetchTTS as ReturnType<typeof vi.fn>).mockResolvedValue(mockUrl)

    const onLoadStart = vi.fn()
    const onLoadEnd = vi.fn()

    const { result } = renderHook(() =>
      useTTS({ apiToken: 'test-token', onLoadStart, onLoadEnd }),
    )

    await act(async () => {
      await result.current.requestTTS('Hello world')
    })

    expect(onLoadStart).toHaveBeenCalledTimes(1)
    expect(onLoadEnd).toHaveBeenCalledTimes(1)
  })

  it('should stop previous TTS before starting new one', async () => {
    const mockUrl1 = 'blob:http://localhost/test-audio-1'
    const mockUrl2 = 'blob:http://localhost/test-audio-2'
    ;(fetchTTS as ReturnType<typeof vi.fn>)
      .mockResolvedValueOnce(mockUrl1)
      .mockResolvedValueOnce(mockUrl2)

    const { result } = renderHook(() => useTTS({ apiToken: 'test-token' }))

    // First request
    await act(async () => {
      await result.current.requestTTS('Hello world')
    })

    expect(result.current.audioUrl).toBe(mockUrl1)

    // Second request
    await act(async () => {
      await result.current.requestTTS('Goodbye world')
    })

    expect(result.current.audioUrl).toBe(mockUrl2)
    expect(fetchTTS).toHaveBeenCalledTimes(2)
  })

  it('should strip markdown before sending to API', async () => {
    const mockUrl = 'blob:http://localhost/test-audio'
    ;(fetchTTS as ReturnType<typeof vi.fn>).mockResolvedValue(mockUrl)

    const { result } = renderHook(() => useTTS({ apiToken: 'test-token' }))

    await act(async () => {
      await result.current.requestTTS('**Bold** and `code`')
    })

    // Should be called with stripped text
    expect(fetchTTS).toHaveBeenCalledWith('Bold and code', 'test-token')
  })
})
