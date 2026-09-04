import {
  PcmAudioPlayer,
  pcm16ToBase64,
  resampleFloat32ToPCM16,
} from './pcm-audio'

export const REALTIME_AUDIO_MODEL = 'gpt-realtime-2.1'
export const REALTIME_AUDIO_VOICE = 'marin'

export type RealtimeAudioState =
  | 'idle'
  | 'connecting'
  | 'listening'
  | 'thinking'
  | 'speaking'

export interface RealtimeAudioClientCallbacks {
  onStateChange: (state: RealtimeAudioState) => void
  onTranscriptChange: (transcript: string) => void
  onError: (message: string) => void
}

export interface RealtimeAudioClientOptions {
  apiBase: string
  apiToken: string
  instructions: string
  callbacks: RealtimeAudioClientCallbacks
}

interface RealtimeServerEvent {
  type?: string
  delta?: string
  transcript?: string
  item_id?: string
  content_index?: number
  error?: {
    message?: string
  }
  item?: {
    id?: string
    role?: string
  }
  response?: {
    status?: string
    status_details?: {
      error?: {
        message?: string
      }
    }
  }
}

/** buildRealtimeWebSocketURL resolves an OpenAI-compatible API base to Realtime WebSocket. */
export function buildRealtimeWebSocketURL(apiBase: string): string {
  const url = new URL(apiBase)
  if (url.protocol !== 'https:' && url.protocol !== 'http:') {
    throw new Error('Realtime API base must use HTTP or HTTPS')
  }
  if (url.username || url.password) {
    throw new Error('Realtime API base must not contain credentials')
  }
  if (url.hash) {
    throw new Error('Realtime API base must not contain a URL fragment')
  }

  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:'
  let path = url.pathname.replace(/\/+$/, '')
  if (!path) {
    path = '/v1/realtime'
  } else if (path.endsWith('/v1')) {
    path += '/realtime'
  } else if (!path.endsWith('/v1/realtime')) {
    path += '/v1/realtime'
  }
  url.pathname = path
  url.searchParams.set('model', REALTIME_AUDIO_MODEL)
  return url.toString()
}

/** createRealtimeSessionUpdate builds the native speech-to-speech session configuration. */
export function createRealtimeSessionUpdate(
  instructions: string,
): Record<string, unknown> {
  return {
    type: 'session.update',
    session: {
      type: 'realtime',
      model: REALTIME_AUDIO_MODEL,
      output_modalities: ['audio'],
      instructions,
      audio: {
        input: {
          format: {
            type: 'audio/pcm',
            rate: 24_000,
          },
          turn_detection: {
            type: 'semantic_vad',
            create_response: true,
            interrupt_response: true,
          },
        },
        output: {
          format: {
            type: 'audio/pcm',
            rate: 24_000,
          },
          voice: REALTIME_AUDIO_VOICE,
        },
      },
    },
  }
}

/** RealtimeAudioClient owns one browser-to-model native audio conversation. */
export class RealtimeAudioClient {
  private readonly options: RealtimeAudioClientOptions
  private readonly player = new PcmAudioPlayer()
  private socket: WebSocket | null = null
  private mediaStream: MediaStream | null = null
  private captureContext: AudioContext | null = null
  private captureSource: MediaStreamAudioSourceNode | null = null
  private captureProcessor: ScriptProcessorNode | null = null
  private captureSilencer: GainNode | null = null
  private playbackQueue: Promise<void> = Promise.resolve()
  private assistantItemID = ''
  private assistantContentIndex = 0
  private transcript = ''
  private stopping = false

  /** constructor stores connection settings and lifecycle callbacks. */
  constructor(options: RealtimeAudioClientOptions) {
    this.options = options
  }

  /** start acquires the microphone and opens an authenticated Realtime session. */
  async start(): Promise<void> {
    if (this.socket || this.mediaStream) {
      throw new Error('Realtime audio session is already active')
    }
    if (!navigator.mediaDevices?.getUserMedia) {
      throw new Error('This browser does not support microphone capture')
    }
    if (/[,\s]/.test(this.options.apiToken)) {
      throw new Error('API token contains unsupported WebSocket characters')
    }

    this.stopping = false
    this.options.callbacks.onStateChange('connecting')

    try {
      this.mediaStream = await navigator.mediaDevices.getUserMedia({
        audio: {
          echoCancellation: true,
          noiseSuppression: true,
          autoGainControl: true,
        },
        video: false,
      })
      await this.player.start()
      await this.openSocket()
    } catch (error) {
      await this.stop()
      throw error
    }
  }

  /** stop closes transport, microphone, capture graph, and playback resources. */
  async stop(): Promise<void> {
    this.stopping = true

    const socket = this.socket
    this.socket = null
    if (socket) {
      socket.onopen = null
      socket.onmessage = null
      socket.onerror = null
      socket.onclose = null
      if (
        socket.readyState === WebSocket.OPEN ||
        socket.readyState === WebSocket.CONNECTING
      ) {
        socket.close(1000, 'client stopped')
      }
    }

    await this.closeCapture()
    await this.player.close()
    this.playbackQueue = Promise.resolve()
    this.assistantItemID = ''
    this.assistantContentIndex = 0
    this.transcript = ''
    this.options.callbacks.onTranscriptChange('')
    this.options.callbacks.onStateChange('idle')
    this.stopping = false
  }

  /** openSocket connects after microphone permission succeeds and initializes the session. */
  private async openSocket(): Promise<void> {
    const url = buildRealtimeWebSocketURL(this.options.apiBase)
    const protocols = [
      'realtime',
      `openai-insecure-api-key.${this.options.apiToken}`,
      'openai-beta.realtime-v1',
    ]

    await new Promise<void>((resolve, reject) => {
      let settled = false
      const socket = new WebSocket(url, protocols)
      this.socket = socket

      socket.onmessage = (message) => {
        this.handleMessage(message.data)
      }
      socket.onerror = () => {
        const error = new Error('Realtime WebSocket connection failed')
        if (!settled) {
          settled = true
          reject(error)
          return
        }
        this.options.callbacks.onError(error.message)
      }
      socket.onclose = (event) => {
        this.socket = null
        if (!settled) {
          settled = true
          reject(
            new Error(
              event.reason ||
                `Realtime WebSocket closed during setup (${event.code})`,
            ),
          )
          return
        }
        if (!this.stopping && event.code !== 1000) {
          this.options.callbacks.onError(
            event.reason || `Realtime audio connection closed (${event.code})`,
          )
          void this.closeCapture()
          void this.player.close()
          this.options.callbacks.onStateChange('idle')
        }
      }
      socket.onopen = () => {
        void (async () => {
          try {
            this.sendEvent(
              createRealtimeSessionUpdate(this.options.instructions),
            )
            await this.openCapture()
            this.options.callbacks.onStateChange('listening')
            settled = true
            resolve()
          } catch (error) {
            settled = true
            reject(error)
          }
        })()
      }
    })
  }

  /** openCapture streams resampled microphone frames through input_audio_buffer.append. */
  private async openCapture(): Promise<void> {
    if (!this.mediaStream) {
      throw new Error('Microphone stream is unavailable')
    }

    const context = new AudioContext()
    this.captureContext = context
    if (context.state === 'suspended') {
      await context.resume()
    }

    const source = context.createMediaStreamSource(this.mediaStream)
    const processor = context.createScriptProcessor(4096, 1, 1)
    const silencer = context.createGain()
    silencer.gain.value = 0

    processor.onaudioprocess = (event) => {
      if (this.socket?.readyState !== WebSocket.OPEN) {
        return
      }
      const samples = event.inputBuffer.getChannelData(0)
      const pcm = resampleFloat32ToPCM16(samples, context.sampleRate)
      if (pcm.length === 0) {
        return
      }
      this.sendEvent({
        type: 'input_audio_buffer.append',
        audio: pcm16ToBase64(pcm),
      })
    }

    source.connect(processor)
    processor.connect(silencer)
    silencer.connect(context.destination)
    this.captureSource = source
    this.captureProcessor = processor
    this.captureSilencer = silencer
  }

  /** closeCapture releases every microphone and Web Audio capture resource. */
  private async closeCapture(): Promise<void> {
    if (this.captureProcessor) {
      this.captureProcessor.onaudioprocess = null
      this.captureProcessor.disconnect()
      this.captureProcessor = null
    }
    if (this.captureSource) {
      this.captureSource.disconnect()
      this.captureSource = null
    }
    if (this.captureSilencer) {
      this.captureSilencer.disconnect()
      this.captureSilencer = null
    }
    if (this.mediaStream) {
      this.mediaStream.getTracks().forEach((track) => track.stop())
      this.mediaStream = null
    }

    const context = this.captureContext
    this.captureContext = null
    if (context && context.state !== 'closed') {
      await context.close()
    }
  }

  /** sendEvent serializes one client event when the Realtime transport is open. */
  private sendEvent(event: Record<string, unknown>): void {
    if (this.socket?.readyState !== WebSocket.OPEN) {
      throw new Error('Realtime WebSocket is not open')
    }
    this.socket.send(JSON.stringify(event))
  }

  /** handleMessage applies one Realtime server event to UI and audio playback state. */
  private handleMessage(raw: unknown): void {
    if (typeof raw !== 'string') {
      return
    }

    let event: RealtimeServerEvent
    try {
      event = JSON.parse(raw) as RealtimeServerEvent
    } catch {
      this.options.callbacks.onError('Realtime server returned invalid JSON')
      return
    }

    switch (event.type) {
      case 'session.updated':
        this.options.callbacks.onStateChange('listening')
        break
      case 'input_audio_buffer.speech_started':
        this.interruptAssistant()
        this.options.callbacks.onStateChange('listening')
        break
      case 'input_audio_buffer.speech_stopped':
        this.options.callbacks.onStateChange('thinking')
        break
      case 'response.created':
        this.player.beginResponse()
        this.transcript = ''
        this.options.callbacks.onTranscriptChange('')
        this.options.callbacks.onStateChange('thinking')
        break
      case 'response.output_item.added':
        if (event.item?.role === 'assistant' && event.item.id) {
          this.assistantItemID = event.item.id
        }
        break
      case 'response.output_audio.delta':
      case 'response.audio.delta':
        this.handleAudioDelta(event)
        break
      case 'response.output_audio_transcript.delta':
      case 'response.audio_transcript.delta':
        if (event.delta) {
          this.transcript += event.delta
          this.options.callbacks.onTranscriptChange(this.transcript)
        }
        break
      case 'response.output_audio_transcript.done':
      case 'response.audio_transcript.done':
        if (event.transcript) {
          this.transcript = event.transcript
          this.options.callbacks.onTranscriptChange(this.transcript)
        }
        break
      case 'response.done':
        this.handleResponseDone(event)
        break
      case 'error':
        this.options.callbacks.onError(
          event.error?.message || 'Realtime API returned an error',
        )
        break
      default:
        break
    }
  }

  /** handleAudioDelta queues native model audio and records its conversation item. */
  private handleAudioDelta(event: RealtimeServerEvent): void {
    if (!event.delta) {
      return
    }
    if (event.item_id) {
      this.assistantItemID = event.item_id
    }
    if (typeof event.content_index === 'number') {
      this.assistantContentIndex = event.content_index
    }

    this.options.callbacks.onStateChange('speaking')
    this.playbackQueue = this.playbackQueue
      .then(() => this.player.append(event.delta || ''))
      .catch(() => {
        this.options.callbacks.onError('Failed to play Realtime audio')
      })
  }

  /** handleResponseDone reports model failures or returns the session to listening. */
  private handleResponseDone(event: RealtimeServerEvent): void {
    if (event.response?.status === 'failed') {
      this.options.callbacks.onError(
        event.response.status_details?.error?.message ||
          'Realtime response failed',
      )
      return
    }
    this.options.callbacks.onStateChange('listening')
  }

  /** interruptAssistant stops unheard audio and truncates it from model context. */
  private interruptAssistant(): void {
    const playedMilliseconds = this.player.interrupt()
    if (!this.assistantItemID || playedMilliseconds <= 0) {
      return
    }

    try {
      this.sendEvent({
        type: 'conversation.item.truncate',
        item_id: this.assistantItemID,
        content_index: this.assistantContentIndex,
        audio_end_ms: playedMilliseconds,
      })
    } catch {
      this.options.callbacks.onError(
        'Failed to reconcile interrupted Realtime audio',
      )
    }
  }
}
