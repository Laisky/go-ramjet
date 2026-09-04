import { describe, expect, it } from 'vitest'

import {
  base64PCM16ToFloat32,
  pcm16ToBase64,
  resampleFloat32ToPCM16,
} from '../pcm-audio'

describe('resampleFloat32ToPCM16', () => {
  it('clamps and converts samples at the same rate', () => {
    const output = resampleFloat32ToPCM16(
      new Float32Array([0, 1, -1, 2, -2]),
      24_000,
    )

    expect(Array.from(output)).toEqual([0, 32767, -32768, 32767, -32768])
  })

  it('resamples 48 kHz input to 24 kHz', () => {
    const input = new Float32Array(480)
    const output = resampleFloat32ToPCM16(input, 48_000)

    expect(output).toHaveLength(240)
  })

  it('rejects invalid sample rates', () => {
    expect(() => resampleFloat32ToPCM16(new Float32Array([0]), 0)).toThrow(
      'Audio sample rates must be positive',
    )
  })
})

describe('PCM16 base64 codec', () => {
  it('round-trips signed little-endian samples', () => {
    const samples = new Int16Array([-32768, -1234, 0, 1234, 32767])
    const decoded = base64PCM16ToFloat32(pcm16ToBase64(samples))

    expect(decoded[0]).toBeCloseTo(-1, 5)
    expect(decoded[1]).toBeCloseTo(-1234 / 32768, 5)
    expect(decoded[2]).toBe(0)
    expect(decoded[3]).toBeCloseTo(1234 / 32767, 5)
    expect(decoded[4]).toBeCloseTo(1, 5)
  })

  it('rejects an odd number of PCM bytes', () => {
    expect(() => base64PCM16ToFloat32(btoa('x'))).toThrow(
      'PCM16 payload must contain an even number of bytes',
    )
  })
})
