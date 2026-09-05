import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import {
  base64PCM16ToFloat32,
  pcm16ToBase64,
  PcmAudioPlayer,
  resampleFloat32ToPCM16,
} from '../pcm-audio'
import { FakeContext, installMedia } from './media-fixtures'

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

describe('PcmAudioPlayer output level', () => {
  let player: PcmAudioPlayer

  beforeEach(async () => {
    installMedia()
    player = new PcmAudioPlayer()
    await player.start()
  })
  afterEach(async () => {
    await player.close()
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('reads zero while nothing is scheduled', () => {
    FakeContext.outputAmplitude = 0.5
    // Silence between turns must settle the indicator rather than freeze it on
    // whatever the analyser last held.
    expect(player.getLevel()).toBe(0)
  })

  it('scales the measured amplitude into a usable display range', async () => {
    FakeContext.outputAmplitude = 0.2
    await player.append(pcm16ToBase64(new Int16Array(2400)))
    // A constant amplitude has that amplitude as its RMS, lifted by the display
    // gain so ordinary speech occupies most of the range.
    expect(player.getLevel()).toBeCloseTo(0.6, 5)
  })

  it('clips rather than overshooting on loud output', async () => {
    FakeContext.outputAmplitude = 0.9
    await player.append(pcm16ToBase64(new Int16Array(2400)))
    expect(player.getLevel()).toBe(1)
  })

  it('drops back to zero once playback is interrupted', async () => {
    FakeContext.outputAmplitude = 0.2
    await player.append(pcm16ToBase64(new Int16Array(2400)))
    player.interrupt()
    expect(player.getLevel()).toBe(0)
  })
})
