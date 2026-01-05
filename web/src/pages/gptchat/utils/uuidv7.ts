/**
 * UUIDv7 generator.
 *
 * uuidv7 generates RFC 9562 UUID version 7 identifiers that are time-ordered.
 * The implementation also applies a simple monotonic random increment when
 * multiple UUIDs are generated within the same millisecond.
 */

const UUID_V7_VERSION = 0x70
const UUID_VARIANT_RFC4122 = 0x80

const RAND_A_BITS = 12n
const RAND_B_BITS = 62n
const RAND_TOTAL_BITS = RAND_A_BITS + RAND_B_BITS

const RAND_MASK = (1n << RAND_TOTAL_BITS) - 1n
const RAND_B_MASK = (1n << RAND_B_BITS) - 1n

let lastTimestampMs = 0
let lastRand = 0n

function getCrypto(): Crypto {
  if (typeof globalThis.crypto !== 'undefined') {
    return globalThis.crypto
  }

  throw new Error('crypto.getRandomValues is required to generate UUIDv7')
}

function randomBigInt(bits: number): bigint {
  const byteLen = Math.ceil(bits / 8)
  const buf = new Uint8Array(byteLen)
  getCrypto().getRandomValues(buf)

  let v = 0n
  for (const b of buf) {
    v = (v << 8n) | BigInt(b)
  }

  const extraBits = BigInt(byteLen * 8 - bits)
  if (extraBits > 0n) {
    v = v & ((1n << BigInt(bits)) - 1n)
  }

  return v
}

function toHexByte(b: number): string {
  return b.toString(16).padStart(2, '0')
}

function formatUuid(bytes: Uint8Array): string {
  const hex = Array.from(bytes, toHexByte).join('')
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`
}

/**
 * uuidv7 generates a time-ordered UUIDv7 string.
 */
export function uuidv7(timestampMs: number = Date.now()): string {
  const ts = Math.max(0, Math.floor(timestampMs))

  let rand74: bigint
  if (ts === lastTimestampMs) {
    rand74 = (lastRand + 1n) & RAND_MASK
  } else {
    rand74 = randomBigInt(Number(RAND_TOTAL_BITS)) & RAND_MASK
    lastTimestampMs = ts
  }
  lastRand = rand74

  const randA = Number((rand74 >> RAND_B_BITS) & ((1n << RAND_A_BITS) - 1n))
  const randB = rand74 & RAND_B_MASK

  const bytes = new Uint8Array(16)

  // 48-bit big-endian timestamp
  bytes[0] = (ts >>> 40) & 0xff
  bytes[1] = (ts >>> 32) & 0xff
  bytes[2] = (ts >>> 24) & 0xff
  bytes[3] = (ts >>> 16) & 0xff
  bytes[4] = (ts >>> 8) & 0xff
  bytes[5] = ts & 0xff

  // version 7 + 12-bit randA
  bytes[6] = UUID_V7_VERSION | ((randA >>> 8) & 0x0f)
  bytes[7] = randA & 0xff

  // variant RFC4122 (10xxxxxx) + top 6 bits of randB
  bytes[8] = UUID_VARIANT_RFC4122 | Number((randB >> 56n) & 0x3fn)

  // remaining 56 bits of randB
  bytes[9] = Number((randB >> 48n) & 0xffn)
  bytes[10] = Number((randB >> 40n) & 0xffn)
  bytes[11] = Number((randB >> 32n) & 0xffn)
  bytes[12] = Number((randB >> 24n) & 0xffn)
  bytes[13] = Number((randB >> 16n) & 0xffn)
  bytes[14] = Number((randB >> 8n) & 0xffn)
  bytes[15] = Number(randB & 0xffn)

  return formatUuid(bytes)
}

/**
 * isUuidV7 returns true if the string looks like a UUIDv7.
 */
export function isUuidV7(value: string | undefined | null): boolean {
  if (!value) return false
  return /^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(
    value,
  )
}

/**
 * compareUuidV7 compares two UUIDv7 strings by their canonical byte order.
 *
 * Returns -1 if a < b, 1 if a > b, 0 if equal or either is not UUIDv7.
 */
export function compareUuidV7(a?: string | null, b?: string | null): number {
  if (!isUuidV7(a) || !isUuidV7(b)) return 0
  const na = a!.replaceAll('-', '').toLowerCase()
  const nb = b!.replaceAll('-', '').toLowerCase()
  if (na === nb) return 0
  return na < nb ? -1 : 1
}
