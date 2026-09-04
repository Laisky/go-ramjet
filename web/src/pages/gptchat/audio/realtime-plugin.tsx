import { Loader2, Radio, Square } from 'lucide-react'
import { useCallback, useEffect, useRef, useState } from 'react'

import { Button } from '@/components/ui/button'
import type { SessionConfig, UserConfig } from '../types'
import type { AudioPluginProps } from './plugin-types'
import { RealtimeAudioClient, type RealtimeAudioState } from './realtime-client'

/** resolveRealtimeAPIBase selects an explicit session override before the account default. */
export function resolveRealtimeAPIBase(
  config: SessionConfig,
  user?: UserConfig,
): string {
  const configured = config.api_base?.trim() || ''
  if (configured && configured !== 'https://api.openai.com') {
    return configured
  }
  return user?.api_base?.trim() || configured
}

/** formatRealtimeStatus combines transport state with a compact native transcript preview. */
export function formatRealtimeStatus(
  state: RealtimeAudioState,
  transcript: string,
): string | null {
  const labels: Record<RealtimeAudioState, string | null> = {
    idle: null,
    connecting: 'Connecting…',
    listening: 'Listening…',
    thinking: 'Thinking…',
    speaking: 'Speaking…',
  }
  const label = labels[state]
  if (!label) {
    return null
  }

  const compactTranscript = transcript.replace(/\s+/g, ' ').trim()
  if (!compactTranscript || (state !== 'speaking' && state !== 'listening')) {
    return label
  }
  const preview =
    compactTranscript.length > 84
      ? `…${compactTranscript.slice(-83)}`
      : compactTranscript
  return `${label} ${preview}`
}

/** RealtimeAudioPlugin controls one native speech-to-speech model session. */
export function RealtimeAudioPlugin({
  config,
  user,
  disabled,
  onError,
  onBusyChange,
  onActivityChange,
  onStatusChange,
}: AudioPluginProps) {
  const [state, setState] = useState<RealtimeAudioState>('idle')
  const [transcript, setTranscript] = useState('')
  const clientRef = useRef<RealtimeAudioClient | null>(null)
  const mountedRef = useRef(true)
  const isActive = state !== 'idle'

  const stopClient = useCallback(async () => {
    const client = clientRef.current
    clientRef.current = null
    if (client) {
      await client.stop()
    }
    if (mountedRef.current) {
      setState('idle')
      setTranscript('')
    }
  }, [])

  const startClient = useCallback(async () => {
    if (!config.api_token) {
      onError('API token is required for Realtime audio.')
      return
    }
    if (!user) {
      onError('Account details are still loading. Please try again.')
      return
    }
    if (user.is_free) {
      onError(
        'Realtime audio is unavailable on the free tier. Use the Whisper plugin instead.',
      )
      return
    }
    if (user.byok === false) {
      onError('Realtime audio requires a user-provided API token.')
      return
    }

    const apiBase = resolveRealtimeAPIBase(config, user)
    if (!apiBase) {
      onError('Realtime API base is missing.')
      return
    }

    onError(null)
    setTranscript('')
    const client = new RealtimeAudioClient({
      apiBase,
      apiToken: config.api_token,
      instructions: config.system_prompt,
      callbacks: {
        onStateChange: (nextState) => {
          if (mountedRef.current) {
            setState(nextState)
          }
        },
        onTranscriptChange: (nextTranscript) => {
          if (mountedRef.current) {
            setTranscript(nextTranscript)
          }
        },
        onError: (message) => {
          if (mountedRef.current) {
            onError(message)
          }
        },
      },
    })
    clientRef.current = client

    try {
      await client.start()
    } catch (error) {
      console.error('Failed to start Realtime audio')
      if (mountedRef.current) {
        onError(
          error instanceof Error
            ? error.message
            : 'Failed to start Realtime audio.',
        )
      }
      if (clientRef.current === client) {
        clientRef.current = null
      }
    }
  }, [config, onError, user])

  const handleToggle = useCallback(() => {
    if (isActive) {
      void stopClient()
      return
    }
    void startClient()
  }, [isActive, startClient, stopClient])

  useEffect(() => {
    onBusyChange(state === 'connecting')
    onActivityChange(isActive)
    onStatusChange(formatRealtimeStatus(state, transcript))
  }, [
    isActive,
    onActivityChange,
    onBusyChange,
    onStatusChange,
    state,
    transcript,
  ])

  useEffect(() => {
    mountedRef.current = true
    return () => {
      mountedRef.current = false
      const client = clientRef.current
      clientRef.current = null
      if (client) {
        void client.stop()
      }
      onBusyChange(false)
      onActivityChange(false)
      onStatusChange(null)
    }
  }, [onActivityChange, onBusyChange, onStatusChange])

  return (
    <Button
      type="button"
      onClick={handleToggle}
      disabled={disabled && !isActive}
      variant={isActive ? 'destructive' : 'outline'}
      className="w-10 rounded-md p-0 shadow-sm"
      aria-label={isActive ? 'Stop Realtime audio' : 'Start Realtime audio'}
    >
      {state === 'connecting' ? (
        <Loader2 className="h-5 w-5 animate-spin" />
      ) : isActive ? (
        <Square className="h-5 w-5" />
      ) : (
        <Radio className="h-5 w-5" />
      )}
    </Button>
  )
}
