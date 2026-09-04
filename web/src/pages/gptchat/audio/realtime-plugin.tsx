import {
  Captions,
  Maximize2,
  Mic,
  MicOff,
  Minimize2,
  PhoneOff,
  X,
} from 'lucide-react'
import { createPortal } from 'react-dom'
import {
  useCallback,
  useEffect,
  useImperativeHandle,
  useRef,
  useState,
} from 'react'
import { Button } from '@/components/ui/button'
import type { AudioPluginProps } from './plugin-types'
import { RealtimeAudioClient, type RealtimeAudioState } from './realtime-client'
import {
  formatRealtimeStatus,
  resolveRealtimeAPIBase,
} from './realtime-plugin-utils'

/** RealtimeAudioPlugin owns a persistent call; props only affect the next explicit start. */
export function RealtimeAudioPlugin({
  config,
  user,
  disabled,
  controlRef,
  sessionLabel,
  onError,
  onBusyChange,
  onActivityChange,
  onStatusChange,
}: AudioPluginProps) {
  const [state, setState] = useState<RealtimeAudioState>('idle')
  const [transcript, setTranscript] = useState('')
  const [muted, setMuted] = useState(false)
  const [captions, setCaptions] = useState(true)
  const [minimized, setMinimized] = useState(false)
  const [label, setLabel] = useState('')
  const [ended, setEnded] = useState('')
  const [seconds, setSeconds] = useState(0)
  const clientRef = useRef<RealtimeAudioClient | null>(null)
  const mounted = useRef(true)
  const connectedAt = useRef<number | null>(null)
  const active = state !== 'idle'

  /** startClient starts from a user gesture and freezes the current routing context for the call. */
  const startClient = useCallback(() => {
    if (clientRef.current || disabled) return
    if (!config.api_token) {
      onError('API token is required for Realtime audio.')
      return
    }
    if (config.token_type === 'proxy' && !user) {
      onError('Account details are still loading. Please try again.')
      return
    }
    // Eligibility is enforced by the gateway/provider, not guessed from stale UI account flags.
    const apiBase = resolveRealtimeAPIBase(config, user)
    if (!apiBase) {
      onError('Realtime API base is missing.')
      return
    }
    setEnded('')
    setTranscript('')
    setMuted(false)
    setMinimized(false)
    setSeconds(0)
    setLabel(sessionLabel || config.session_name || 'Voice call')
    connectedAt.current = null
    onError(null)
    const client = new RealtimeAudioClient({
      apiBase,
      apiToken: config.api_token,
      instructions: config.system_prompt,
      callbacks: {
        onStateChange: (next) => {
          if (!mounted.current || clientRef.current !== client) return
          if (next === 'listening' && connectedAt.current === null)
            connectedAt.current = Date.now()
          setState(next)
        },
        onTranscriptChange: (text) => {
          if (mounted.current && clientRef.current === client)
            setTranscript(text)
        },
        onError: (message) => {
          if (mounted.current && clientRef.current === client) onError(message)
        },
        onEnded: (reason, detail) => {
          if (!mounted.current || clientRef.current !== client) return
          clientRef.current = null
          setState('idle')
          setEnded(
            reason === 'assistant'
              ? `AI ended the call${detail ? `: ${detail}` : '.'}`
              : reason === 'user'
                ? 'Call ended.'
                : reason === 'remote'
                  ? 'The provider ended the call. You can start a new call.'
                  : 'Call disconnected. Check the connection and start a new call.',
          )
        },
      },
    })
    clientRef.current = client
    void client.start().catch((error: unknown) => {
      if (!mounted.current) return
      if (error instanceof DOMException && error.name === 'AbortError') return
      if (clientRef.current && clientRef.current !== client) return
      onError(
        error instanceof Error
          ? error.message.replaceAll(config.api_token, '[redacted]')
          : 'Failed to start voice call.',
      )
      if (clientRef.current === client) {
        clientRef.current = null
        setState('idle')
      }
    })
  }, [config, user, disabled, onError, sessionLabel])

  useImperativeHandle(
    controlRef,
    () => ({ start: startClient, reveal: () => setMinimized(false) }),
    [startClient],
  )

  useEffect(() => {
    onBusyChange(state === 'connecting' || state === 'ending')
    onActivityChange(active)
    onStatusChange(
      active
        ? muted
          ? 'Microphone muted'
          : formatRealtimeStatus(state, '')
        : null,
    )
  }, [state, active, muted, onBusyChange, onActivityChange, onStatusChange])

  useEffect(() => {
    if (!active) return
    const tick = setInterval(() => {
      if (connectedAt.current !== null)
        setSeconds(Math.floor((Date.now() - connectedAt.current) / 1000))
    }, 1000)
    const beforeUnload = (event: BeforeUnloadEvent) => {
      event.preventDefault()
      event.returnValue = ''
    }
    const pageHide = () => {
      void clientRef.current?.stop('user')
    }
    window.addEventListener('beforeunload', beforeUnload)
    window.addEventListener('pagehide', pageHide)
    return () => {
      clearInterval(tick)
      window.removeEventListener('beforeunload', beforeUnload)
      window.removeEventListener('pagehide', pageHide)
    }
  }, [active])

  useEffect(() => {
    mounted.current = true
    return () => {
      mounted.current = false
      const client = clientRef.current
      clientRef.current = null
      if (client) void client.stop('user')
      onActivityChange(false)
      onBusyChange(false)
      onStatusChange(null)
    }
  }, [onActivityChange, onBusyChange, onStatusChange])

  if (!active && !ended) return null
  return createPortal(
    <section
      role="dialog"
      aria-label="Voice call"
      aria-modal="false"
      onKeyDown={(event) => {
        if (event.key === 'Escape' && active) setMinimized(true)
      }}
      className="fixed bottom-4 right-4 z-50 w-[calc(100%-2rem)] max-w-sm rounded-2xl border border-border bg-background p-4 text-foreground shadow-xl"
    >
      <div className="flex items-center justify-between gap-2">
        <div className="min-w-0">
          <h2 className="truncate font-semibold">{label}</h2>
          <p className="text-xs text-muted-foreground">
            AI voice call · {String(Math.floor(seconds / 60)).padStart(2, '0')}:
            {String(seconds % 60).padStart(2, '0')}
          </p>
        </div>
        {active ? (
          <Button
            variant="ghost"
            onClick={() => setMinimized(!minimized)}
            aria-label={minimized ? 'Restore call' : 'Minimize call'}
          >
            {minimized ? (
              <Maximize2 className="h-4 w-4" />
            ) : (
              <Minimize2 className="h-4 w-4" />
            )}
          </Button>
        ) : (
          <Button
            variant="ghost"
            onClick={() => setEnded('')}
            aria-label="Dismiss ended call"
          >
            <X className="h-4 w-4" />
          </Button>
        )}
      </div>
      <p role="status" aria-live="polite" className="my-3 text-sm">
        {active
          ? state === 'ending'
            ? 'Ending call…'
            : muted
              ? 'Microphone muted'
              : formatRealtimeStatus(state, '')
          : ended}
      </p>
      {active && !minimized && (
        <>
          <div
            aria-hidden="true"
            className={`mx-auto my-5 h-16 w-16 rounded-full bg-primary/20 ring-4 ring-primary/10 ${state === 'speaking' ? 'motion-safe:animate-pulse' : ''}`}
          />
          <p className="mb-3 text-xs text-muted-foreground">
            Speak naturally. You can interrupt the AI. This call stays with the
            account and prompt it started with.
          </p>
          {captions && transcript && (
            <p
              aria-label="AI captions"
              className="mb-3 max-h-40 overflow-auto whitespace-pre-wrap text-sm"
            >
              {transcript}
            </p>
          )}
        </>
      )}
      {active && (
        <div className="flex flex-wrap gap-2">
          <Button
            variant={muted ? 'default' : 'outline'}
            aria-pressed={muted}
            aria-label={muted ? 'Unmute microphone' : 'Mute microphone'}
            disabled={state === 'connecting' || state === 'ending'}
            onClick={() => {
              clientRef.current?.setMuted(!muted)
              setMuted(!muted)
            }}
          >
            {muted ? (
              <MicOff className="h-4 w-4" />
            ) : (
              <Mic className="h-4 w-4" />
            )}
          </Button>
          {!minimized && (
            <Button
              variant="outline"
              aria-label="Toggle captions"
              aria-pressed={captions}
              onClick={() => setCaptions(!captions)}
            >
              <Captions className="h-4 w-4" />
            </Button>
          )}
          <Button
            variant="destructive"
            className="ml-auto"
            onClick={() => {
              void clientRef.current?.stop('user')
            }}
            aria-label="Hang up"
          >
            <PhoneOff className="h-4 w-4" />
            Hang up
          </Button>
        </div>
      )}
    </section>,
    document.body,
  )
}
