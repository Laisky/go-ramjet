import { afterEach, describe, expect, it, vi } from 'vitest'

import { constantTimeEqual } from '../constant-time'
import {
  LOWERCASE_ALPHANUMERIC,
  secureRandomString,
  secureRandomUUIDv4,
} from '../secure-random'

describe('secureRandomString', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('returns strings of the requested length from the alphabet', () => {
    for (const length of [0, 1, 8, 32, 100]) {
      const value = secureRandomString(length)
      expect(value).toHaveLength(length)
      expect(value).toMatch(/^[a-z0-9]*$/)
    }
    expect(secureRandomString(64, 'ab')).toMatch(/^[ab]{64}$/)
  })

  it('draws from crypto.getRandomValues, never Math.random', () => {
    const mathSpy = vi.spyOn(Math, 'random')
    const cryptoSpy = vi.spyOn(globalThis.crypto, 'getRandomValues')

    expect(secureRandomString(32)).not.toBe(secureRandomString(32))
    expect(cryptoSpy).toHaveBeenCalled()
    expect(mathSpy).not.toHaveBeenCalled()
  })

  it('rejects biased bytes instead of folding them into the alphabet', () => {
    // 36 symbols: bytes >= 252 must be discarded, so 255 never maps to 'd'.
    const queue = [Uint8Array.of(255, 0), Uint8Array.of(1, 1)]
    vi.spyOn(globalThis.crypto, 'getRandomValues').mockImplementation(
      <T extends ArrayBufferView | null>(array: T): T => {
        const next = queue.shift() ?? Uint8Array.of(2)
        const target = array as unknown as Uint8Array
        target.fill(255)
        target.set(next.subarray(0, target.length))
        return array
      },
    )

    expect(secureRandomString(2, LOWERCASE_ALPHANUMERIC)).toBe('ab')
  })

  it('refuses invalid lengths and alphabets', () => {
    expect(() => secureRandomString(-1)).toThrow(RangeError)
    expect(() => secureRandomString(1.5)).toThrow(RangeError)
    expect(() => secureRandomString(4, 'a')).toThrow(RangeError)
  })

  it('fails closed when Web Crypto is unavailable', () => {
    vi.stubGlobal('crypto', undefined)
    try {
      expect(() => secureRandomString(8)).toThrow(/getRandomValues/)
    } finally {
      vi.unstubAllGlobals()
    }
  })
})

describe('secureRandomUUIDv4', () => {
  it('returns distinct RFC 9562 version 4 UUIDs', () => {
    const uuid = secureRandomUUIDv4()
    expect(uuid).toMatch(
      /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/,
    )
    expect(secureRandomUUIDv4()).not.toBe(uuid)
  })
})

describe('constantTimeEqual', () => {
  it.each([
    ['', '', true],
    ['secret', 'secret', true],
    ['secret', 'secreT', false],
    ['secret', 'secret ', false],
    ['secret', 'secre', false],
    ['', 'x', false],
    ['ключ', 'ключ', true],
    ['ключ', 'ключи', false],
  ])('compares %j and %j as %s', (a, b, equal) => {
    expect(constantTimeEqual(a, b)).toBe(equal)
    expect(constantTimeEqual(b, a)).toBe(equal)
  })
})
