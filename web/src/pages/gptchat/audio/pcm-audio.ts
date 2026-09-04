export const REALTIME_PCM_SAMPLE_RATE = 24_000

/** resampleFloat32ToPCM16 converts browser audio samples to signed 24 kHz PCM16. */
export function resampleFloat32ToPCM16(
  input: Float32Array,
  inputSampleRate: number,
  outputSampleRate = REALTIME_PCM_SAMPLE_RATE,
): Int16Array {
  if (inputSampleRate <= 0 || outputSampleRate <= 0) {
    throw new Error('Audio sample rates must be positive')
  }
  if (input.length === 0) {
    return new Int16Array()
  }

  const outputLength = Math.max(
    1,
    Math.floor((input.length * outputSampleRate) / inputSampleRate),
  )
  const output = new Int16Array(outputLength)
  const sourceStep = inputSampleRate / outputSampleRate

  for (let index = 0; index < outputLength; index += 1) {
    const sourcePosition = index * sourceStep
    const sourceIndex = Math.floor(sourcePosition)
    const nextIndex = Math.min(sourceIndex + 1, input.length - 1)
    const fraction = sourcePosition - sourceIndex
    const sample =
      input[sourceIndex] + (input[nextIndex] - input[sourceIndex]) * fraction
    const clamped = Math.max(-1, Math.min(1, sample))
    output[index] =
      clamped < 0 ? Math.round(clamped * 0x8000) : Math.round(clamped * 0x7fff)
  }

  return output
}

/** pcm16ToBase64 encodes signed little-endian PCM16 for Realtime JSON events. */
export function pcm16ToBase64(samples: Int16Array): string {
  const bytes = new Uint8Array(samples.length * 2)
  const view = new DataView(bytes.buffer)
  samples.forEach((sample, index) => {
    view.setInt16(index * 2, sample, true)
  })

  let binary = ''
  const chunkSize = 0x8000
  for (let offset = 0; offset < bytes.length; offset += chunkSize) {
    binary += String.fromCharCode(...bytes.subarray(offset, offset + chunkSize))
  }
  return btoa(binary)
}

/** base64PCM16ToFloat32 decodes Realtime little-endian PCM16 output for playback. */
export function base64PCM16ToFloat32(
  encoded: string,
): Float32Array<ArrayBuffer> {
  const binary = atob(encoded)
  if (binary.length % 2 !== 0) {
    throw new Error('PCM16 payload must contain an even number of bytes')
  }

  const bytes = new Uint8Array(binary.length)
  for (let index = 0; index < binary.length; index += 1) {
    bytes[index] = binary.charCodeAt(index)
  }

  const view = new DataView(bytes.buffer)
  const output = new Float32Array(bytes.length / 2)
  for (let index = 0; index < output.length; index += 1) {
    const sample = view.getInt16(index * 2, true)
    output[index] = sample < 0 ? sample / 0x8000 : sample / 0x7fff
  }
  return output
}

/** PcmAudioPlayer schedules audio, measures heard samples, and cannot reopen after disposal. */
export class PcmAudioPlayer {
  private context: AudioContext | null = null
  private closed = false
  private nextStart = 0
  private segments: { start: number; duration: number }[] = []
  private readonly sources = new Set<AudioBufferSourceNode>()
  private readonly drainWaiters = new Set<() => void>()

  /** start unlocks output during the original user gesture, before microphone permission awaits. */
  async start(): Promise<void> {
    if (this.closed) throw new DOMException('Audio player closed', 'AbortError')
    const context =
      this.context ?? new AudioContext({ sampleRate: REALTIME_PCM_SAMPLE_RATE })
    this.context = context
    if (context.state === 'suspended') await context.resume()
    if (this.closed || this.context !== context)
      throw new DOMException('Audio player closed', 'AbortError')
  }

  /** getContext returns the unlocked context shared by capture and playback. */
  getContext(): AudioContext {
    if (!this.context || this.closed)
      throw new Error('Audio context is unavailable')
    return this.context
  }

  /** beginResponse resets played-audio accounting after previous output has been stopped. */
  beginResponse(): void {
    this.segments = []
  }

  /** append schedules a native audio delta without an asynchronous restart race. */
  async append(encoded: string): Promise<void> {
    const context = this.getContext()
    const samples = base64PCM16ToFloat32(encoded)
    if (!samples.length) return
    const buffer = context.createBuffer(
      1,
      samples.length,
      REALTIME_PCM_SAMPLE_RATE,
    )
    buffer.copyToChannel(samples, 0)
    const source = context.createBufferSource()
    source.buffer = buffer
    source.connect(context.destination)
    const start = Math.max(context.currentTime, this.nextStart)
    this.nextStart = start + buffer.duration
    this.segments.push({ start, duration: buffer.duration })
    this.sources.add(source)
    source.onended = () => {
      source.disconnect()
      this.sources.delete(source)
      this.notifyDrain()
    }
    source.start(start)
  }

  /** whenDrained resolves after every scheduled audio node has actually ended. */
  whenDrained(): Promise<void> {
    if (!this.sources.size) return Promise.resolve()
    return new Promise((resolve) => this.drainWaiters.add(resolve))
  }

  /** notifyDrain settles listeners only after playback has fully drained or been stopped. */
  private notifyDrain(): void {
    if (this.sources.size) return
    this.drainWaiters.forEach((resolve) => resolve())
    this.drainWaiters.clear()
  }

  /** interrupt stops output and returns heard milliseconds, excluding network gaps. */
  interrupt(): number {
    const now = this.context?.currentTime ?? 0
    const heard = this.segments.reduce(
      (sum, part) =>
        sum + Math.max(0, Math.min(now - part.start, part.duration)),
      0,
    )
    this.sources.forEach((source) => {
      source.onended = null
      source.stop()
      source.disconnect()
    })
    this.sources.clear()
    this.segments = []
    this.nextStart = now
    this.notifyDrain()
    return Math.floor(heard * 1000)
  }

  /** close synchronously silences audio and permanently disposes this call's context. */
  async close(): Promise<void> {
    if (this.closed) return
    this.closed = true
    this.interrupt()
    const context = this.context
    this.context = null
    if (context && context.state !== 'closed') await context.close()
  }
}
