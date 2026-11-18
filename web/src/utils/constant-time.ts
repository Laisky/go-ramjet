/**
 * Constant-time comparison for secrets held in the browser.
 *
 * Threat model: this only removes the obvious early-exit timing signal.
 * JavaScript engines can still optimize, so it is not a guarantee. Secrets
 * compared in the browser already live in the same profile as the code
 * comparing them, so isolation (what may be stored and sent where) is what
 * protects them, not this function.
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
