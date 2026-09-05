/** StreamingPcmResampler converts a continuous input stream to 24 kHz PCM16 without chunk drift. */
export class StreamingPcmResampler {
  private readonly ratio: number
  private weight = 0
  private sum = 0

  /** constructor validates the source sample rate and initializes the conversion ratio. */
  constructor(inputRate: number, outputRate = 24_000) {
    if (
      !Number.isFinite(inputRate) ||
      !Number.isFinite(outputRate) ||
      inputRate <= 0 ||
      outputRate <= 0
    ) {
      throw new Error('Audio sample rates must be positive and finite')
    }
    this.ratio = inputRate / outputRate
  }

  /** append emits complete output samples, retaining fractional coverage for the next chunk. */
  append(input: Float32Array): Int16Array<ArrayBuffer> {
    const output: number[] = []
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
