import { describe, expect, it } from 'vitest'

import { compareUuidV7, isUuidV7, uuidv7 } from '../uuidv7'

function hexToBytes(uuid: string): Uint8Array {
  const hex = uuid.replaceAll('-', '').toLowerCase()
  const out = new Uint8Array(hex.length / 2)
  for (let i = 0; i < out.length; i++) {
    out[i] = parseInt(hex.slice(i * 2, i * 2 + 2), 16)
  }
  return out
}

describe('uuidv7', () => {
  it('generates valid UUIDv7 strings', () => {
    const id = uuidv7()
    expect(isUuidV7(id)).toBe(true)

    const bytes = hexToBytes(id)
    // version nibble is 7
    expect((bytes[6] & 0xf0) >>> 4).toBe(0x7)
    // variant is RFC4122 (10xxxxxx)
    expect((bytes[8] & 0xc0) >>> 6).toBe(0x2)
  })

  it('is monotonic within same millisecond', () => {
    const ts = 1700000000000
    const a = uuidv7(ts)
    const b = uuidv7(ts)

    expect(isUuidV7(a)).toBe(true)
    expect(isUuidV7(b)).toBe(true)

    expect(compareUuidV7(a, b)).toBe(-1)
  })
})
