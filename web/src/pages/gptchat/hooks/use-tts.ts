/**
 * useTTS hook for text-to-speech functionality.
 *
 * This hook provides TTS capabilities using the server-side Azure TTS API.
 * It fetches audio from the backend and manages playback state, providing
 * a visible audio player for both desktop and mobile compatibility.
 */
import { useCallback, useRef, useState } from 'react'

import { fetchTTS } from '@/utils/api'

export interface UseTTSOptions {
  /** API token for authentication */
  apiToken: string
  /** Callback when TTS starts loading */
  onLoadStart?: () => void
  /** Callback when TTS finishes loading */
  onLoadEnd?: () => void
  /** Callback when an error occurs */
  onError?: (error: string) => void
}

export interface UseTTSReturn {
  /** Whether TTS is currently loading */
  isLoading: boolean
  /** Whether audio is currently playing */
  isPlaying: boolean
  /** Error message if any */
  error: string | null
  /** The audio element URL (for displaying an audio player) */
  audioUrl: string | null
  /** Request TTS for the given text */
  requestTTS: (text: string) => Promise<void>
  /** Stop and clear current audio */
  stopTTS: () => void
  /** Toggle play/pause */
  togglePlayback: () => void
  /** Set playing state (called by audio player component) */
  setIsPlaying: (playing: boolean) => void
}

/**
 * Strips markdown formatting from text for cleaner TTS output.
 *
 * @param input - Markdown text to strip
 * @returns Plain text without markdown formatting
 */
export function stripMarkdownForTTS(input: string): string {
  if (typeof input !== 'string') {
    return ''
  }
  return (
    input
      // Remove code block delimiters but keep content
      .replace(/```(?:\w+\s*\n)?([\s\S]*?)```/g, '$1')
      // Remove inline code delimiters but keep content
      .replace(/`([^`]*)`/g, '$1')
      // Remove images
      .replace(/!\[[^\]]*\]\([^)]*\)/g, '')
      // Convert links to just their text
      .replace(/\[([^\]]+)\]\([^)]*\)/g, '$1')
      // Remove markdown formatting characters
      .replace(/[>*_~`#]/g, '')
      // Normalize whitespace
      .replace(/\s+/g, ' ')
      .trim()
  )
}

/**
 * useTTS provides text-to-speech functionality using server-side Azure TTS.
 *
 * @param options - Configuration options for TTS
 * @returns TTS state and control functions
 */
export function useTTS({
  apiToken,
  onLoadStart,
  onLoadEnd,
  onError,
}: UseTTSOptions): UseTTSReturn {
  const [isLoading, setIsLoading] = useState(false)
  const [isPlaying, setIsPlaying] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [audioUrl, setAudioUrl] = useState<string | null>(null)

  const currentUrlRef = useRef<string | null>(null)

  // Cleanup function to revoke object URLs
  const cleanup = useCallback(() => {
    if (currentUrlRef.current) {
      console.debug('[useTTS] Revoking object URL:', currentUrlRef.current)
      URL.revokeObjectURL(currentUrlRef.current)
      currentUrlRef.current = null
    }
  }, [])

  const stopTTS = useCallback(() => {
    console.debug('[useTTS] Stopping TTS')
    cleanup()
    setAudioUrl(null)
    setIsPlaying(false)
    setError(null)
  }, [cleanup])

  const requestTTS = useCallback(
    async (text: string) => {
      if (!apiToken) {
        const msg = 'API token is required for TTS'
        console.debug('[useTTS] Error:', msg)
        setError(msg)
        onError?.(msg)
        return
      }

      const plainText = stripMarkdownForTTS(text)
      if (!plainText) {
        const msg = 'No text content to speak'
        console.debug('[useTTS] Error:', msg)
        setError(msg)
        onError?.(msg)
        return
      }

      // Stop any existing playback
      stopTTS()

      setIsLoading(true)
      setError(null)
      onLoadStart?.()

      try {
        console.debug(
          '[useTTS] Requesting TTS for text length:',
          plainText.length,
        )
        const url = await fetchTTS(plainText, apiToken)
        currentUrlRef.current = url
        setAudioUrl(url)
        console.debug('[useTTS] TTS audio ready:', url)
      } catch (err) {
        const msg = err instanceof Error ? err.message : String(err)
        console.debug('[useTTS] Request error:', msg)
        setError(msg)
        onError?.(msg)
      } finally {
        setIsLoading(false)
        onLoadEnd?.()
      }
    },
    [apiToken, onLoadStart, onLoadEnd, onError, stopTTS],
  )

  // togglePlayback is a no-op in this simplified version
  // The TTSAudioPlayer component handles its own playback
  const togglePlayback = useCallback(() => {
    console.debug('[useTTS] togglePlayback called - handled by audio element')
  }, [])

  return {
    isLoading,
    isPlaying,
    error,
    audioUrl,
    requestTTS,
    stopTTS,
    togglePlayback,
    setIsPlaying,
  }
}
