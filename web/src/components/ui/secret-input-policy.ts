/**
 * Browser-facing policy for SecretInput: markup that keeps password managers
 * away from app secrets, and the interaction check that gates value changes.
 *
 * Password managers key off `type="password"`, and no autocomplete token makes
 * them skip such a field: Chrome, Firefox, and Safari ignore
 * `autocomplete="off"` there, and `autocomplete="new-password"` invites
 * password generation and save prompts instead. Chromium and Firefox also
 * keep treating a field as a password field once it has *ever* been
 * `type="password"`, so a reveal toggle that flips `type` does not escape.
 * App secrets therefore render as `type="text"` from the first render and are
 * masked with CSS.
 */

/**
 * SECRET_INPUT_MASK_CLASS masks the characters of a text input the way a
 * password field does. `-webkit-text-security` is supported by Chromium,
 * WebKit, and Gecko (Firefox 114+).
 */
export const SECRET_INPUT_MASK_CLASS = '[-webkit-text-security:disc]'

/**
 * SECRET_INPUT_OPT_OUT_ATTRIBUTES marks a field as an app secret rather than a
 * login credential. `autoComplete="off"` is the WHATWG token for sensitive
 * data that must not be remembered; the data attributes are the documented
 * opt-outs of 1Password, LastPass, Bitwarden, Dashlane, and Proton Pass. They
 * must be present on the first render, because some extensions classify a
 * field once and cache the result.
 */
export const SECRET_INPUT_OPT_OUT_ATTRIBUTES = {
  autoComplete: 'off',
  'data-1p-ignore': 'true',
  'data-lpignore': 'true',
  'data-bwignore': 'true',
  'data-form-type': 'other',
  'data-protonpass-ignore': 'true',
} as const

/**
 * isUserEditing reports whether a value change on input comes from the user
 * editing that field. Typing, pasting, dictation, and picking a suggestion all
 * happen while the field has focus; browser autofill of a field that merely
 * appeared on screen dispatches synthetic focus/input events without moving
 * focus into it. It returns true when input is the focused element.
 */
export function isUserEditing(input: HTMLInputElement): boolean {
  return input.ownerDocument.activeElement === input
}
