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
      input[sourceIndex] +
      (input[nextIndex] - input[sourceIndex]) * fraction
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
export function base64PCM16ToFloat32(encoded: string): Float32Array {
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

/** PcmAudioPlayer schedules streamed PCM chunks and tracks how much audio was heard. */
export class PcmAudioPlayer {
  private context: AudioContext | null = null
  private nextStartTime = 0
  private responseStartTime: number | null = null
  private responseEndTime: number | null = null
  private readonly activeSources = new Set<AudioBufferSourceNode>()

  /** start prepares the browser audio context from a user gesture. */
  async start(): Promise<void> {
    if (!this.context) {
      this.context = new AudioContext()
    }
    if (this.context.state === 'suspended') {
      await this.context.resume()
    }
    this.nextStartTime = Math.max(this.nextStartTime, this.context.currentTime)
  }

  /** beginResponse resets playback accounting for a new assistant response. */
  beginResponse(): void {
    this.responseStartTime = null
    this.responseEndTime = null
  }

  /** append schedules one base64-encoded PCM16 delta for gap-free playback. */
  async append(encoded: string): Promise<void> {
    await this.start()
    if (!this.context) {
      throw new Error('Audio context is unavailable')
    }

    const samples = base64PCM16ToFloat32(encoded)
    if (samples.length === 0) {
      return
    }

    const buffer = this.context.createBuffer(
      1,
      samples.length,
      REALTIME_PCM_SAMPLE_RATE,
    )
    buffer.copyToChannel(samples, 0)

    const source = this.context.createBufferSource()
    source.buffer = buffer
    source.connect(this.context.destination)

    const startTime = Math.max(this.context.currentTime, this.nextStartTime)
    if (this.responseStartTime === null) {
      this.responseStartTime = startTime
    }
    this.responseEndTime = startTime + buffer.duration
    this.nextStartTime = this.responseEndTime
    this.activeSources.add(source)
    source.onended = () => {
      this.activeSources.delete(source)
    }
    source.start(startTime)
  }

  /** interrupt stops queued output and returns milliseconds already played. */
  interrupt(): number {
    if (!this.context) {
      return 0
    }

    let playedMilliseconds = 0
    if (this.responseStartTime !== null && this.responseEndTime !== null) {
      const playedSeconds = Math.max(
        0,
        Math.min(
          this.context.currentTime - this.responseStartTime,
          this.responseEndTime - this.responseStartTime,
        ),
      )
      playedMilliseconds = Math.round(playedSeconds * 1000)
    }

    this.activeSources.forEach((source) => {
      try {
        source.stop()
      } catch {
        // The source may already have ended between iteration and stop().
      }
    })
    this.activeSources.clear()
    this.nextStartTime = this.context.currentTime
    this.responseStartTime = null
    this.responseEndTime = null
    return playedMilliseconds
  }

  /** close releases every playback resource owned by the player. */
  async close(): Promise<void> {
    this.interrupt()
    const context = this.context
    this.context = null
    this.nextStartTime = 0
    if (context && context.state !== 'closed') {
      await context.close()
    }
  }
}
