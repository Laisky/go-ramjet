/**
 * Cryptographically secure random helpers for anything that acts as a
 * credential or must not be guessable (sync keys, tokens, passwords, session
 * IDs). Never fall back to Math.random: its output is predictable.
 *
 * Only crypto.getRandomValues is used, because unlike crypto.randomUUID and
 * crypto.subtle it is also available on insecure (plain HTTP) origins.
 */

/**
 * LOWERCASE_ALPHANUMERIC is the default alphabet: URL-, header-, and
 * copy-paste-safe, and unambiguous when users type it by hand.
 */
export const LOWERCASE_ALPHANUMERIC = 'abcdefghijklmnopqrstuvwxyz0123456789'

/**
 * getSecureCrypto returns the Web Crypto implementation.
 * It throws when none exists, so callers never silently degrade to weak
 * randomness.
 */
function getSecureCrypto(): Crypto {
  const impl = globalThis.crypto
  if (!impl || typeof impl.getRandomValues !== 'function') {
    throw new Error('crypto.getRandomValues is required for secure randomness')
  }
  return impl
}

/**
 * secureRandomString returns a string of length characters drawn uniformly
 * from alphabet using a CSPRNG. Rejection sampling discards bytes above the
 * largest multiple of the alphabet size, so no character is favored.
 * alphabet must hold between 2 and 256 characters. It returns the random
 * string.
 */
export function secureRandomString(
  length: number,
  alphabet: string = LOWERCASE_ALPHANUMERIC,
): string {
  if (!Number.isInteger(length) || length < 0) {
    throw new RangeError(`invalid random string length: ${length}`)
  }
  if (alphabet.length < 2 || alphabet.length > 256) {
    throw new RangeError(`invalid alphabet size: ${alphabet.length}`)
  }

  const cryptoImpl = getSecureCrypto()
  const limit = 256 - (256 % alphabet.length)
  let result = ''
  while (result.length < length) {
    const bytes = cryptoImpl.getRandomValues(
      new Uint8Array(Math.max(16, (length - result.length) * 2)),
    )
    for (const byte of bytes) {
      if (byte >= limit) continue
      result += alphabet[byte % alphabet.length]
      if (result.length === length) break
    }
  }
  return result
}

/**
 * secureRandomUUIDv4 returns an RFC 9562 version 4 UUID built from a CSPRNG.
 * It works on insecure origins, where crypto.randomUUID is unavailable.
 */
export function secureRandomUUIDv4(): string {
  const bytes = getSecureCrypto().getRandomValues(new Uint8Array(16))
  bytes[6] = (bytes[6] & 0x0f) | 0x40
  bytes[8] = (bytes[8] & 0x3f) | 0x80
  const hex = Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('')
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`
}
