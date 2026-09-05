import { describe, expect, it } from 'vitest'
import { StreamingPcmResampler } from '../pcm-resampler'

describe('Continuous PCM resampling', () => {
  it.each([24_000, 44_100, 48_000])(
    'preserves duration at %i Hz regardless of worklet block boundaries',
    (rate) => {
      const input = Float32Array.from({ length: rate }, (_, i) =>
        Math.sin(i / 50),
      )
      const whole = new StreamingPcmResampler(rate).append(input)
      const streaming = new StreamingPcmResampler(rate)
      const chunks: number[] = []
      for (let offset = 0; offset < input.length; offset += 128)
        chunks.push(...streaming.append(input.subarray(offset, offset + 128)))
      expect(whole).toHaveLength(24_000)
      expect(chunks).toEqual(Array.from(whole))
    },
  )

  it('clips and preserves little-endian PCM16 signed endpoints', () => {
    const converter = new StreamingPcmResampler(24_000)
    expect(
      Array.from(converter.append(new Float32Array([-2, -1, 0, 1, 2]))),
    ).toEqual([-32768, -32768, 0, 32767, 32767])
  })

  it.each([0, -1, NaN, Infinity])('rejects invalid sample rate %s', (rate) => {
    expect(() => new StreamingPcmResampler(rate)).toThrow('positive and finite')
  })
})
