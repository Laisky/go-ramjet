import { Loader2, Radio, Square } from 'lucide-react'
import { useCallback, useEffect, useRef, useState } from 'react'

import { Button } from '@/components/ui/button'
import type { AudioPluginProps } from './plugin-types'
import { RealtimeAudioClient, type RealtimeAudioState } from './realtime-client'
import {
  formatRealtimeStatus,
  resolveRealtimeAPIBase,
} from './realtime-plugin-utils'

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
