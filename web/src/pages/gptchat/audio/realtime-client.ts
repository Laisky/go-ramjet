import { PcmAudioPlayer, pcm16ToBase64 } from './pcm-audio'
import { abortError, stopMediaStream, waitForMedia } from './media-lifecycle'
import {
  buildRealtimeWebSocketURL,
  createRealtimeSessionUpdate,
} from './realtime-session'

export {
  buildRealtimeWebSocketURL,
  createRealtimeSessionUpdate,
  DEFAULT_REALTIME_AUDIO_MODEL,
  REALTIME_AUDIO_VOICE,
  REALTIME_INPUT_TRANSCRIPTION_MODEL,
} from './realtime-session'
// The capture worklet is served verbatim from /public, not bundled. Vite's worker
// transform injects an `import` that AudioWorkletGlobalScope cannot execute, which
// leaves 'gptchat-microphone' unregistered even though addModule() resolves.
const WORKLET_URL = '/audio/pcm-worklet.js'

export type RealtimeAudioState =
  | 'idle'
  | 'connecting'
  | 'listening'
  | 'thinking'
  | 'speaking'
  | 'ending'
export type CallEndReason = 'user' | 'assistant' | 'remote' | 'error'

/** RealtimeAudioClientCallbacks reports call state, captions, errors, and terminal reasons. */
export interface RealtimeAudioClientCallbacks {
  onStateChange: (state: RealtimeAudioState) => void
  onTranscriptChange: (transcript: string) => void
  onError: (message: string) => void
  onEnded?: (reason: CallEndReason, detail?: string) => void
  /**
   * onUserTurn reports the caller's own speech.
   *
   * `open` fires when the turn is committed, before any text exists, so a
   * consumer can reserve an ordered slot. Transcription runs asynchronously from
   * the reply, so `final` can arrive after the assistant has already answered
   * and must never be appended in arrival order.
   */
  onUserTurn?: (event: UserTurnEvent) => void
  /** onAssistantTurn reports the assistant's spoken text, final at the end of a turn. */
  onAssistantTurn?: (text: string, done: boolean) => void
}

/** UserTurnEvent carries one stage of a caller utterance, keyed by its conversation item. */
export interface UserTurnEvent {
  itemID: string
  phase: 'open' | 'final' | 'failed'
  text: string
}

/** RealtimeAudioClientOptions captures the immutable account and prompt for one call. */
export interface RealtimeAudioClientOptions {
  apiBase: string
  apiToken: string
  instructions: string
  /** model is the server-configured realtime model for this deployment. */
  model?: string
  callbacks: RealtimeAudioClientCallbacks
}

/** RealtimeItem contains only the model output fields needed for audio and hang-up. */
interface RealtimeItem {
  id?: string
  type?: string
  role?: string
  name?: string
  call_id?: string
  arguments?: string
}

/** RealtimeServerEvent is the validated subset of GA Realtime events consumed here. */
interface RealtimeServerEvent {
  type?: string
  delta?: string
  transcript?: string
  response_id?: string
  item_id?: string
  content_index?: number
  error?: { message?: string }
  item?: RealtimeItem
  response?: {
    id?: string
    status?: string
    output?: RealtimeItem[]
    status_details?: { error?: { message?: string } }
  }
}

export const MAX_REALTIME_BUFFERED_BYTES = 512 * 1024
const CONNECTION_TIMEOUT_MS = 20_000
const GOODBYE_TIMEOUT_MS = 15_000

/** RealtimeAudioClient owns a single cancellable, full-duplex native-audio call. */
export class RealtimeAudioClient {
  private readonly options: RealtimeAudioClientOptions
  private readonly player = new PcmAudioPlayer()
  private readonly abort = new AbortController()
  private started = false
  private closed = false
  private ending = false
  private ready = false
  private muted = false
  private socket: WebSocket | null = null
  private mediaStream: MediaStream | null = null
  private source: MediaStreamAudioSourceNode | null = null
  private processor: AudioWorkletNode | null = null
  private rejectReady: ((error: Error) => void) | null = null
  private acceptReady: (() => void) | null = null
  private closePromise: Promise<void> | null = null
  private goodbyeTimer: ReturnType<typeof setTimeout> | null = null
  private playbackQueue: Promise<void> = Promise.resolve()
  private playbackGeneration = 0
  private responseID = ''
  private outputBlocked = false
  private assistantItemID = ''
  private assistantContentIndex = 0
  private transcript = ''
  private readonly handledCalls = new Set<string>()

  /** constructor captures settings so later UI/session edits cannot reroute an active call. */
  constructor(options: RealtimeAudioClientOptions) {
    this.options = { ...options, callbacks: { ...options.callbacks } }
  }

  /** start unlocks audio on the user gesture, acquires media, and waits for server configuration. */
  async start(): Promise<void> {
    if (this.started || this.closed)
      throw new Error('Create a new client for each voice call')
    if (!navigator.mediaDevices?.getUserMedia)
      throw new Error(
        // Browsers hide mediaDevices outside a secure context, so an insecure
        // origin is indistinguishable from a missing feature without this check.
        window.isSecureContext === false
          ? 'Microphone capture needs a secure page. Open this site over HTTPS or on localhost.'
          : 'This browser does not support microphone capture',
      )
    const url = buildRealtimeWebSocketURL(
      this.options.apiBase,
      this.options.model,
    )
    if (!/^[!#$%&'*+.^_`|~0-9A-Za-z-]+$/.test(this.options.apiToken)) {
      throw new Error('API token contains unsupported WebSocket characters')
    }
    this.started = true
    this.options.callbacks.onStateChange('connecting')
    try {
      // start() executes synchronously until its first await, preserving user activation.
      const outputReady = this.player.start()
      const microphoneReady = navigator.mediaDevices
        .getUserMedia({
          audio: {
            echoCancellation: true,
            noiseSuppression: true,
            autoGainControl: true,
          },
          video: false,
        })
        .then((stream) => {
          if (this.closed) {
            stopMediaStream(stream)
            throw abortError()
          }
          this.mediaStream = stream
          stream.getTracks().forEach((track) => {
            track.onended = () =>
              this.fail(
                'The microphone was disconnected. Start a new call to reconnect.',
              )
          })
        })
      await waitForMedia(
        Promise.all([outputReady, microphoneReady]),
        this.abort.signal,
      )
      await waitForMedia(this.openSocket(url), this.abort.signal)
      await this.openCapture()
      if (this.closed) throw abortError()
      this.ready = true
      this.options.callbacks.onStateChange('listening')
    } catch (error) {
      if (!this.closed) await this.stop('error')
      throw error
    }
  }

  /** stop immediately silences and releases media, then reports the end exactly once. */
  stop(reason: CallEndReason = 'user', detail?: string): Promise<void> {
    if (this.closePromise) return this.closePromise
    this.closed = true
    this.ready = false
    this.abort.abort()
    this.playbackGeneration += 1
    this.rejectReady?.(abortError())
    this.rejectReady = null
    this.acceptReady = null
    if (this.goodbyeTimer) clearTimeout(this.goodbyeTimer)
    this.goodbyeTimer = null
    const socket = this.socket
    this.socket = null
    if (socket) {
      socket.onopen = socket.onmessage = socket.onerror = socket.onclose = null
      if (
        socket.readyState === WebSocket.OPEN ||
        socket.readyState === WebSocket.CONNECTING
      )
        socket.close(1000, 'call ended')
    }
    if (this.processor) {
      this.processor.port.onmessage = null
      this.processor.port.close()
      this.processor.disconnect()
      this.processor = null
    }
    this.source?.disconnect()
    this.source = null
    if (this.mediaStream) stopMediaStream(this.mediaStream)
    this.mediaStream = null
    this.options.callbacks.onStateChange('ending')
    this.closePromise = this.player
      .close()
      .catch(() => {
        this.options.callbacks.onError(
          'Could not close the audio context; microphone capture has stopped.',
        )
      })
      .then(() => {
        this.options.callbacks.onStateChange('idle')
        this.options.callbacks.onEnded?.(reason, detail)
      })
    return this.closePromise
  }

  /** setMuted gates outgoing frames and hardware tracks without replacing the call connection. */
  setMuted(muted: boolean): void {
    this.muted = muted
    this.mediaStream?.getAudioTracks().forEach((track) => {
      track.enabled = !muted
    })
    if (muted && this.ready)
      this.sendEvent({ type: 'input_audio_buffer.clear' })
  }

  /** openSocket resolves only after the server acknowledges the requested GA audio configuration. */
  private openSocket(url: string): Promise<void> {
    return new Promise((resolve, reject) => {
      const socket = new WebSocket(url, [
        'realtime',
        `openai-insecure-api-key.${this.options.apiToken}`,
      ])
      this.socket = socket
      const timer = setTimeout(
        () =>
          finish(new Error('Realtime connection timed out. Please try again.')),
        CONNECTION_TIMEOUT_MS,
      )
      let settled = false
      const finish = (error?: Error) => {
        if (settled) return
        settled = true
        clearTimeout(timer)
        this.rejectReady = null
        this.acceptReady = null
        if (error) reject(error)
        else resolve()
      }
      this.rejectReady = (error) => finish(error)
      this.acceptReady = () => finish()
      socket.onopen = () => {
        if (!this.closed)
          this.sendEvent(createRealtimeSessionUpdate(this.options.instructions))
      }
      socket.onmessage = (message) => this.handleMessage(message.data)
      socket.onerror = () => {
        if (!settled)
          finish(
            new Error(
              'Realtime connection failed. Check the API base, token, and model access.',
            ),
          )
        else
          this.fail(
            'Realtime connection failed. Start a new call to reconnect.',
          )
      }
      socket.onclose = (event) => {
        if (this.closed) return
        if (!settled)
          finish(
            new Error(
              `Realtime connection closed during setup (${event.code})`,
            ),
          )
        if (event.code !== 1000)
          this.options.callbacks.onError(
            `Voice connection lost (${event.code}). Start a new call to reconnect.`,
          )
        void this.stop(event.code === 1000 ? 'remote' : 'error')
      }
    })
  }

  /** openCapture installs a worklet; cancellation at the async module boundary prevents late graphs. */
  private async openCapture(): Promise<void> {
    const context = this.player.getContext()
    if (!context.audioWorklet || typeof AudioWorkletNode === 'undefined') {
      throw new Error(
        'Voice calls require AudioWorklet support and HTTPS (or localhost).',
      )
    }
    await waitForMedia(
      context.audioWorklet.addModule(WORKLET_URL),
      this.abort.signal,
    )
    if (this.closed || !this.mediaStream) throw abortError()
    this.source = context.createMediaStreamSource(this.mediaStream)
    this.processor = new AudioWorkletNode(context, 'gptchat-microphone', {
      numberOfInputs: 1,
      numberOfOutputs: 1,
      outputChannelCount: [1],
    })
    this.processor.port.onmessage = (event: MessageEvent<ArrayBuffer>) => {
      if (this.closed || !this.ready || this.muted || this.ending) return
      if ((this.socket?.bufferedAmount ?? 0) > MAX_REALTIME_BUFFERED_BYTES) {
        this.fail(
          'Voice connection is too slow. The call was stopped to avoid sending delayed audio.',
        )
        return
      }
      this.sendEvent({
        type: 'input_audio_buffer.append',
        audio: pcm16ToBase64(new Int16Array(event.data)),
      })
    }
    this.source.connect(this.processor)
    // The worklet outputs silence; the microphone is never monitored through speakers.
    this.processor.connect(context.destination)
  }

  /** sendEvent writes a client event or fails the call without exposing credentials. */
  private sendEvent(event: Record<string, unknown>): void {
    if (this.closed || this.socket?.readyState !== WebSocket.OPEN) return
    try {
      this.socket.send(JSON.stringify(event))
    } catch {
      this.fail('Could not send voice data. Start a new call to reconnect.')
    }
  }

  /** fail presents a transport/media error and releases this call's resources. */
  private fail(message: string): void {
    if (this.closed) return
    this.options.callbacks.onError(
      message.replaceAll(this.options.apiToken, '[redacted]'),
    )
    void this.stop('error')
  }

  /** handleMessage routes valid GA events while ignoring stale output after interruption. */
  private handleMessage(raw: unknown): void {
    if (this.closed || typeof raw !== 'string') return
    let event: RealtimeServerEvent
    try {
      const value: unknown = JSON.parse(raw)
      if (!value || typeof value !== 'object' || Array.isArray(value))
        throw new Error('Invalid event')
      event = value as RealtimeServerEvent
    } catch {
      this.fail('Realtime returned an invalid event.')
      return
    }
    if (event.type === 'session.updated') {
      this.acceptReady?.()
      return
    }
    if (event.type === 'error') {
      const message =
        typeof event.error?.message === 'string'
          ? event.error.message.replaceAll(this.options.apiToken, '[redacted]')
          : 'Realtime API returned an error'
      if (!this.ready) {
        // Configuration never completed, so there is no usable call to keep.
        this.rejectReady?.(new Error(message))
        this.fail(message)
        return
      }
      // Realtime errors are per-event and mostly recoverable; the session stays
      // open. A provider that considers the call over closes the socket, and
      // onclose already ends it. Tearing down here dropped healthy calls.
      this.options.callbacks.onError(message)
      return
    }
    if (!this.ready || this.ending) return
    if (event.type === 'input_audio_buffer.speech_started') {
      this.interruptAssistant()
      this.options.callbacks.onStateChange('listening')
      return
    }
    if (event.type === 'input_audio_buffer.speech_stopped') {
      this.options.callbacks.onStateChange('thinking')
      return
    }
    // These must stay above the response-scoping gate below. They carry no
    // response_id, so the gate's outputBlocked branch would drop every one of
    // them that lands during a barge-in window.
    if (event.type === 'input_audio_buffer.committed') {
      if (event.item_id)
        this.options.callbacks.onUserTurn?.({
          itemID: event.item_id,
          phase: 'open',
          text: '',
        })
      return
    }
    if (
      event.type === 'conversation.item.input_audio_transcription.completed'
    ) {
      if (event.item_id)
        this.options.callbacks.onUserTurn?.({
          itemID: event.item_id,
          phase: 'final',
          text: event.transcript ?? '',
        })
      return
    }
    if (event.type === 'conversation.item.input_audio_transcription.failed') {
      if (event.item_id)
        this.options.callbacks.onUserTurn?.({
          itemID: event.item_id,
          phase: 'failed',
          text: '',
        })
      return
    }
    if (event.type === 'response.created') {
      this.playbackGeneration += 1
      this.player.interrupt()
      this.player.beginResponse()
      this.responseID = event.response?.id ?? ''
      this.outputBlocked = false
      this.assistantItemID = ''
      this.assistantContentIndex = 0
      this.transcript = ''
      this.options.callbacks.onTranscriptChange('')
      this.options.callbacks.onStateChange('thinking')
      return
    }
    const eventResponseID = event.response_id ?? event.response?.id
    if (
      this.outputBlocked ||
      (eventResponseID &&
        this.responseID &&
        eventResponseID !== this.responseID)
    )
      return
    switch (event.type) {
      case 'response.output_item.added':
        if (event.item?.role === 'assistant')
          this.assistantItemID = event.item.id ?? ''
        break
      case 'response.output_audio.delta':
      case 'response.audio.delta':
        if (typeof event.delta !== 'string') break
        this.assistantItemID = event.item_id ?? this.assistantItemID
        this.assistantContentIndex = event.content_index ?? 0
        this.queueAudio(event.delta)
        break
      case 'response.output_audio_transcript.delta':
      case 'response.audio_transcript.delta':
        if (typeof event.delta === 'string') this.transcript += event.delta
        this.options.callbacks.onTranscriptChange(this.transcript)
        this.options.callbacks.onAssistantTurn?.(this.transcript, false)
        break
      case 'response.output_audio_transcript.done':
      case 'response.audio_transcript.done':
        // `.done` carries the authoritative final text, so prefer it over the
        // accumulated deltas rather than trusting our own concatenation.
        if (typeof event.transcript === 'string')
          this.transcript = event.transcript
        this.options.callbacks.onTranscriptChange(this.transcript)
        if (this.transcript)
          this.options.callbacks.onAssistantTurn?.(this.transcript, true)
        break
      case 'response.done':
        this.finishResponse(event)
        break
    }
  }

  /** queueAudio serializes scheduling and invalidates deltas queued before an interruption. */
  private queueAudio(delta: string): void {
    const generation = this.playbackGeneration
    this.options.callbacks.onStateChange('speaking')
    this.playbackQueue = this.playbackQueue
      .then(() => {
        if (this.closed || generation !== this.playbackGeneration) return
        return this.player.append(delta)
      })
      .catch(() =>
        this.fail('Could not play voice audio. Please start a new call.'),
      )
  }

  /** finishResponse waits for audible playback, rather than mistaking generation completion for hang-up. */
  private finishResponse(event: RealtimeServerEvent): void {
    if (event.response?.status === 'completed') {
      const call = event.response.output?.find(
        (item) => item.type === 'function_call' && item.name === 'end_call',
      )
      if (call && this.endByAssistant(call)) return
    }
    if (event.response?.status === 'failed') {
      this.options.callbacks.onError(
        'The voice response failed. You can try speaking again.',
      )
    }
    const generation = this.playbackGeneration
    void this.playbackQueue
      .then(() => this.player.whenDrained())
      .then(() => {
        if (
          !this.closed &&
          !this.ending &&
          generation === this.playbackGeneration
        )
          this.options.callbacks.onStateChange('listening')
      })
      .catch(() => this.fail('Could not finish voice playback.'))
  }

  /** endByAssistant accepts one completed hang-up tool and drains goodbye audio before disposal. */
  private endByAssistant(item: RealtimeItem): boolean {
    if (!item.call_id || this.handledCalls.has(item.call_id)) return this.ending
    let reason: string
    try {
      const args = JSON.parse(item.arguments ?? '{}') as { reason?: unknown }
      if (
        typeof args.reason !== 'string' ||
        !args.reason.trim() ||
        args.reason.length > 240
      )
        return false
      reason = args.reason.trim()
    } catch {
      return false
    }
    this.handledCalls.add(item.call_id)
    this.ending = true
    this.setMuted(true)
    this.options.callbacks.onStateChange('ending')
    this.sendEvent({
      type: 'conversation.item.create',
      item: {
        type: 'function_call_output',
        call_id: item.call_id,
        output: JSON.stringify({ ended: true }),
      },
    })
    this.goodbyeTimer = setTimeout(() => {
      void this.stop('assistant', reason)
    }, GOODBYE_TIMEOUT_MS)
    void this.playbackQueue
      .then(() => this.player.whenDrained())
      .then(() => this.stop('assistant', reason))
      .catch(() => this.fail('Could not finish the voice call.'))
    return true
  }

  /** interruptAssistant removes unheard output, including a zero-millisecond interruption. */
  private interruptAssistant(): void {
    this.playbackGeneration += 1
    this.outputBlocked = true
    const milliseconds = this.player.interrupt()
    if (this.assistantItemID)
      this.sendEvent({
        type: 'conversation.item.truncate',
        item_id: this.assistantItemID,
        content_index: this.assistantContentIndex,
        audio_end_ms: milliseconds,
      })
    this.assistantItemID = ''
    // Emit before the wipe. This is the only place partially spoken text still
    // exists, and a log that drops interrupted answers misrepresents the call.
    if (this.transcript)
      this.options.callbacks.onAssistantTurn?.(this.transcript, true)
    this.transcript = ''
    this.options.callbacks.onTranscriptChange('')
  }
}
