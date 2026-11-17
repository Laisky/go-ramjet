/**
 * Constant-time comparison for secrets held in the browser.
 */

/**
 * constantTimeEqual reports whether the strings a and b are identical. It
 * always walks the longer input in full and folds every difference into one
 * accumulator, so its running time does not reveal how long a matching prefix
 * is. JavaScript engines give no hard timing guarantee; this is the
 * best-effort equivalent of Go's subtle.ConstantTimeCompare.
 * It returns true when both strings have the same UTF-16 code units.
 */
export function constantTimeEqual(a: string, b: string): boolean {
  const length = Math.max(a.length, b.length)
  let diff = a.length ^ b.length
  for (let i = 0; i < length; i += 1) {
    // charCodeAt past the end yields NaN, which bitwise operators read as 0.
    diff |= a.charCodeAt(i) ^ b.charCodeAt(i)
  }
  return diff === 0
}
