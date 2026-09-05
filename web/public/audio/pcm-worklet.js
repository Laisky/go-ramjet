// Microphone capture worklet for GPTChat voice calls.
//
// This file is served verbatim from the public directory and is NEVER processed
// by the bundler. That is deliberate. AudioWorkletGlobalScope cannot execute
// `import` statements, and Vite's worker transform injects one of its own in dev
// (its client env module) on top of any the source already has. When the module
// body throws, addModule() still resolves, registerProcessor() never runs, and
// constructing the node fails with "The node name 'gptchat-microphone' is not
// defined in AudioWorkletGlobalScope". Keep this file free of imports and
// exports, and keep it out of src/.
//
// StreamingPcmResampler below mirrors src/pages/gptchat/audio/pcm-resampler.ts.
// pcm-worklet.test.ts evaluates this file and asserts the two stay identical.

/** StreamingPcmResampler converts a continuous input stream to 24 kHz PCM16 without chunk drift. */
class StreamingPcmResampler {
  /** constructor validates the source sample rate and initializes the conversion ratio. */
  constructor(inputRate, outputRate = 24000) {
    if (
      !Number.isFinite(inputRate) ||
      !Number.isFinite(outputRate) ||
      inputRate <= 0 ||
      outputRate <= 0
    ) {
      throw new Error('Audio sample rates must be positive and finite')
    }
    this.ratio = inputRate / outputRate
    this.weight = 0
    this.sum = 0
  }

  /** append emits complete output samples, retaining fractional coverage for the next chunk. */
  append(input) {
    const output = []
    for (const value of input) {
      let remaining = 1
      while (remaining > 1e-9) {
        const take = Math.min(remaining, this.ratio - this.weight)
        this.sum += value * take
        this.weight += take
        remaining -= take
        if (this.weight >= this.ratio - 1e-9) {
          const sample = Math.max(-1, Math.min(1, this.sum / this.ratio))
          output.push(Math.round(sample * (sample < 0 ? 32768 : 32767)))
          this.sum = 0
          this.weight = 0
        }
      }
    }
    return new Int16Array(output)
  }
}

/** MicrophoneProcessor sends 20 ms PCM16 frames off the audio rendering thread. */
class MicrophoneProcessor extends AudioWorkletProcessor {
  constructor() {
    super()
    this.resampler = new StreamingPcmResampler(sampleRate)
    this.frame = new Int16Array(480)
    this.offset = 0
  }

  /** process converts mono capture blocks without ever monitoring the microphone through speakers. */
  process(inputs) {
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
