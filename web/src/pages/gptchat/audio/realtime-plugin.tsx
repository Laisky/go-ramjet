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
import { generateChatId } from '../utils/chat-storage'
import type { AudioPluginProps } from './plugin-types'
import { RealtimeAudioClient, type RealtimeAudioState } from './realtime-client'
import { DEFAULT_REALTIME_AUDIO_MODEL } from './realtime-session'
import {
  clampFloatingPosition,
  formatRealtimeStatus,
  resolveRealtimeAPIBase,
  VIEWPORT_MARGIN,
  type FloatingPosition,
} from './realtime-plugin-utils'

// How far one arrow-key press nudges the call window, for callers who move it
// without a pointer.
const KEYBOARD_STEP = 16

/** RealtimeAudioPlugin owns a persistent call; props only affect the next explicit start. */
export function RealtimeAudioPlugin({
  config,
  user,
  disabled,
  controlRef,
  sessionLabel,
  sessionId,
  onVoiceMessage,
  onCallSessionChange,
  onError,
  onBusyChange,
  onActivityChange,
  onStatusChange,
}: AudioPluginProps) {
  // The realtime model is a server setting; the browser never chooses it.
  const realtimeModel =
    user?.voice?.realtime_model?.trim() || DEFAULT_REALTIME_AUDIO_MODEL
  const [state, setState] = useState<RealtimeAudioState>('idle')
  const [transcript, setTranscript] = useState('')
  const [muted, setMuted] = useState(false)
  const [captions, setCaptions] = useState(true)
  const [minimized, setMinimized] = useState(false)
  const [label, setLabel] = useState('')
  const [ended, setEnded] = useState('')
  const [seconds, setSeconds] = useState(0)
  // null until the window is first moved, so it keeps its corner placement — and
  // that corner's responsive width — for everyone who never drags it.
  const [position, setPosition] = useState<FloatingPosition | null>(null)
  const panelRef = useRef<HTMLElement | null>(null)
  const orbRef = useRef<HTMLDivElement | null>(null)
  const dragRef = useRef<{
    pointerId: number
    offsetX: number
    offsetY: number
  } | null>(null)
  const clientRef = useRef<RealtimeAudioClient | null>(null)
  const mounted = useRef(true)
  const connectedAt = useRef<number | null>(null)
  // Pinned at call start. The call must keep writing to the session it began in
  // even after the user switches the view to a different session.
  const callSessionRef = useRef<number | null>(null)
  // Conversation item id -> chat turn id. A turn holds the caller's utterance and
  // the assistant's reply under one chat id, which is how text turns are stored.
  const turnsRef = useRef(new Map<string, string>())
  const assistantTurnRef = useRef<string | null>(null)
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
    // Freeze the destination session alongside the routing context above.
    callSessionRef.current = sessionId ?? null
    turnsRef.current = new Map()
    assistantTurnRef.current = null
    onError(null)
    const client = new RealtimeAudioClient({
      apiBase,
      apiToken: config.api_token,
      instructions: config.system_prompt,
      // Server-configured, so a deployment can change models without a rebuild.
      model: realtimeModel,
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
        onUserTurn: (event) => {
          if (clientRef.current !== client) return
          const target = callSessionRef.current
          if (target === null || !onVoiceMessage) return
          if (event.phase === 'open') {
            // Reserve the slot now. Transcription is asynchronous and its result
            // can land after the assistant has already replied, so appending in
            // arrival order would interleave the conversation wrongly.
            const chatID = generateChatId()
            turnsRef.current.set(event.itemID, chatID)
            assistantTurnRef.current = chatID
            onVoiceMessage(
              target,
              { chatID, role: 'user', content: '', timestamp: Date.now() },
              true,
            )
            return
          }
          const chatID = turnsRef.current.get(event.itemID)
          if (!chatID) return
          turnsRef.current.delete(event.itemID)
          if (event.phase === 'failed') {
            onVoiceMessage(
              target,
              {
                chatID,
                role: 'user',
                content: '[speech could not be transcribed]',
                timestamp: Date.now(),
              },
              true,
            )
            return
          }
          onVoiceMessage(
            target,
            {
              chatID,
              role: 'user',
              content: event.text,
              timestamp: Date.now(),
            },
            true,
          )
        },
        onAssistantTurn: (text, done) => {
          if (clientRef.current !== client) return
          const target = callSessionRef.current
          if (target === null || !onVoiceMessage) return
          // The assistant can speak first, before any caller utterance exists.
          let chatID = assistantTurnRef.current
          if (!chatID) {
            chatID = generateChatId()
            assistantTurnRef.current = chatID
          }
          onVoiceMessage(
            target,
            {
              chatID,
              role: 'assistant',
              content: text,
              model: realtimeModel,
              timestamp: Date.now(),
            },
            done,
          )
          // A finished turn releases the id so the next reply opens a new one.
          if (done) assistantTurnRef.current = null
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
  }, [
    config,
    user,
    disabled,
    onError,
    sessionLabel,
    sessionId,
    onVoiceMessage,
    realtimeModel,
  ])

  useImperativeHandle(
    controlRef,
    () => ({ start: startClient, reveal: () => setMinimized(false) }),
    [startClient],
  )

  useEffect(() => {
    onBusyChange(state === 'connecting' || state === 'ending')
    onActivityChange(active)
    // Report the session captured at call start. Reading the live prop here would
    // move the recording destination whenever the user switched sessions.
    onCallSessionChange?.(active ? callSessionRef.current : null)
    onStatusChange(
      active
        ? muted
          ? 'Microphone muted'
          : formatRealtimeStatus(state, '')
        : null,
    )
  }, [
    state,
    active,
    muted,
    onBusyChange,
    onActivityChange,
    onCallSessionChange,
    onStatusChange,
  ])

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

  // Keep the latest reporters in a ref so the teardown below can depend on nothing.
  const reportersRef = useRef({
    onActivityChange,
    onBusyChange,
    onStatusChange,
  })
  useEffect(() => {
    // Written in an effect rather than during render: refs must not be mutated
    // while rendering. The teardown only reads this on unmount, long after this
    // has run, so it always sees the latest reporters.
    reportersRef.current = { onActivityChange, onBusyChange, onStatusChange }
  }, [onActivityChange, onBusyChange, onStatusChange])

  useEffect(() => {
    mounted.current = true
    return () => {
      mounted.current = false
      const client = clientRef.current
      clientRef.current = null
      if (client) void client.stop('user')
      const reporters = reportersRef.current
      reporters.onActivityChange(false)
      reporters.onBusyChange(false)
      reporters.onStatusChange(null)
    }
    // Empty on purpose: this hangs up the call, so it must run only on unmount.
    // Depending on the reporter callbacks made a live call collapse whenever an
    // unrelated prop changed their identity, which broke session pinning.
  }, [])

  // The orb tracks the reply's loudness while the assistant speaks.
  useEffect(() => {
    const orb = orbRef.current
    if (!orb) return
    if (state !== 'speaking') {
      orb.style.setProperty('--voice-level', '0')
      return
    }
    let frame = 0
    const follow = () => {
      // Written straight to the element. A React state update per animation
      // frame would re-render the whole call window sixty times a second to
      // change one number.
      orb.style.setProperty(
        '--voice-level',
        (clientRef.current?.getOutputLevel() ?? 0).toFixed(3),
      )
      frame = requestAnimationFrame(follow)
    }
    frame = requestAnimationFrame(follow)
    return () => {
      cancelAnimationFrame(frame)
      orb.style.setProperty('--voice-level', '0')
    }
    // minimized is a dependency because the orb is unmounted while minimized,
    // so the loop has to re-attach to the element rendered on restore.
  }, [state, minimized])

  // Re-clamped whenever the window can fall outside the viewport: a rotated
  // phone, a resized browser, or the height change from minimizing it.
  const floating = position !== null
  useEffect(() => {
    if (!floating) return
    const reclamp = () => {
      const panel = panelRef.current
      if (!panel) return
      setPosition((current) => {
        if (!current) return current
        const next = clampFloatingPosition(
          current,
          { width: panel.offsetWidth, height: panel.offsetHeight },
          { width: window.innerWidth, height: window.innerHeight },
        )
        return next.x === current.x && next.y === current.y ? current : next
      })
    }
    reclamp()
    window.addEventListener('resize', reclamp)
    return () => window.removeEventListener('resize', reclamp)
  }, [floating, minimized])

  /** moveTo places the window at a viewport point, clamped to stay reachable. */
  const moveTo = useCallback((x: number, y: number) => {
    const panel = panelRef.current
    if (!panel) return
    setPosition(
      clampFloatingPosition(
        { x, y },
        { width: panel.offsetWidth, height: panel.offsetHeight },
        { width: window.innerWidth, height: window.innerHeight },
      ),
    )
  }, [])

  /** beginDrag grabs the window at the point the caller pressed, not at its corner. */
  const beginDrag = (event: React.PointerEvent<HTMLDivElement>) => {
    // A second finger landing mid-drag must not seize the window and snap it to
    // a new grab point; the first one keeps it until it lifts.
    if (dragRef.current) return
    // Buttons live in the drag handle, so a press on one must stay a click.
    if (event.button !== 0 || (event.target as HTMLElement).closest('button'))
      return
    const panel = panelRef.current
    if (!panel) return
    const rect = panel.getBoundingClientRect()
    dragRef.current = {
      pointerId: event.pointerId,
      offsetX: event.clientX - rect.left,
      offsetY: event.clientY - rect.top,
    }
    // Capture keeps the drag alive when the pointer outruns the handle.
    event.currentTarget.setPointerCapture?.(event.pointerId)
    moveTo(rect.left, rect.top)
  }

  const continueDrag = (event: React.PointerEvent<HTMLDivElement>) => {
    const drag = dragRef.current
    if (!drag || drag.pointerId !== event.pointerId) return
    moveTo(event.clientX - drag.offsetX, event.clientY - drag.offsetY)
  }

  const endDrag = (event: React.PointerEvent<HTMLDivElement>) => {
    const drag = dragRef.current
    if (!drag || drag.pointerId !== event.pointerId) return
    dragRef.current = null
    event.currentTarget.releasePointerCapture?.(event.pointerId)
  }

  /** nudge moves the window from the keyboard, the only route without a pointer. */
  const nudge = (event: React.KeyboardEvent<HTMLDivElement>) => {
    const steps: Record<string, [number, number]> = {
      ArrowLeft: [-KEYBOARD_STEP, 0],
      ArrowRight: [KEYBOARD_STEP, 0],
      ArrowUp: [0, -KEYBOARD_STEP],
      ArrowDown: [0, KEYBOARD_STEP],
    }
    const step = steps[event.key]
    const panel = panelRef.current
    if (!step || !panel) return
    event.preventDefault()
    // The first nudge has no stored position yet, so start from where the
    // corner placement actually put the window.
    const rect = panel.getBoundingClientRect()
    const from = position ?? { x: rect.left, y: rect.top }
    moveTo(from.x + step[0], from.y + step[1])
  }

  if (!active && !ended) return null
  return createPortal(
    <section
      role="dialog"
      aria-label="Voice call"
      aria-modal="false"
      onKeyDown={(event) => {
        if (event.key === 'Escape' && active) setMinimized(true)
      }}
      ref={panelRef}
      style={
        position
          ? {
              left: position.x,
              top: position.y,
              maxWidth: `calc(100vw - ${VIEWPORT_MARGIN * 2}px)`,
            }
          : undefined
      }
      className={`fixed z-50 w-[calc(100%-2rem)] max-w-sm rounded-2xl border border-border bg-background p-4 text-foreground shadow-xl ${
        position ? '' : 'bottom-4 right-4'
      }`}
    >
      <div
        role="group"
        aria-label="Move the call window"
        tabIndex={0}
        onPointerDown={beginDrag}
        onPointerMove={continueDrag}
        onPointerUp={endDrag}
        onPointerCancel={endDrag}
        onKeyDown={nudge}
        // touch-none stops the browser scrolling the page instead of dragging.
        className="flex touch-none cursor-grab select-none items-center justify-between gap-2 active:cursor-grabbing"
      >
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
            ref={orbRef}
            aria-hidden="true"
            className={`voice-orb mx-auto my-5 h-16 w-16 rounded-full bg-primary/20 ring-4 ring-primary/10 ${
              state === 'listening' ? 'voice-orb-listening' : ''
            }`}
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
