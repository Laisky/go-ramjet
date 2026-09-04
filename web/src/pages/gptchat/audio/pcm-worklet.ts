import { StreamingPcmResampler } from './pcm-resampler'

declare const sampleRate: number
/** AudioWorkletProcessor describes the browser-provided audio worklet base class. */
declare class AudioWorkletProcessor {
  readonly port: MessagePort
}
declare function registerProcessor(
  name: string,
  processor: typeof AudioWorkletProcessor,
): void

/** MicrophoneProcessor sends 20 ms PCM16 frames off the audio rendering thread. */
class MicrophoneProcessor extends AudioWorkletProcessor {
  private readonly resampler = new StreamingPcmResampler(sampleRate)
  private frame = new Int16Array(480)
  private offset = 0

  /** process converts mono capture blocks without ever monitoring the microphone through speakers. */
  process(inputs: Float32Array[][]): boolean {
    const input = inputs[0]?.[0]
    if (input) {
      for (const sample of this.resampler.append(input)) {
        this.frame[this.offset++] = sample
        if (this.offset === this.frame.length) {
          this.port.postMessage(this.frame.buffer, [this.frame.buffer])
          this.frame = new Int16Array(480)
          this.offset = 0
        }
      }
    }
    return true
  }
}

registerProcessor('gptchat-microphone', MicrophoneProcessor)
