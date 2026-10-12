/** GPTChat ownership sessions are independent of model keys and CV sessions. */
export const GPTCHAT_SSO_STORAGE_KEY = 'gptchat_ownership_sso_token'
export const GPTCHAT_SSO_STATE_KEY = 'gptchat_ownership_sso_pending_state'
let unpersistedSession: string | null = null

/** readOwnershipSession returns the stored original SSO session without interpreting claims. */
export function readOwnershipSession(): string | null {
  try {
    return (
      window.localStorage.getItem(GPTCHAT_SSO_STORAGE_KEY) || unpersistedSession
    )
  } catch {
    return unpersistedSession
  }
}

/** clearOwnershipSession removes only the GPTChat ownership session. */
export function clearOwnershipSession(): void {
  unpersistedSession = null
  try {
    window.localStorage.removeItem(GPTCHAT_SSO_STORAGE_KEY)
  } catch {
    // Storage may be unavailable; the component still clears its local session.
  }
}

/** matchingState compares a valid nonce without stopping at the first differing character. */
function matchingState(
  expected: string | null,
  actual: string | null,
): boolean {
  if (!expected || !actual || expected.length !== 64 || actual.length !== 64)
    return false
  let difference = 0
  for (let i = 0; i < 64; i += 1)
    difference |= expected.charCodeAt(i) ^ actual.charCodeAt(i)
  return difference === 0
}

/** consumeOwnershipSession scrubs callback credentials and accepts only this tab's pending sign-in. */
export function consumeOwnershipSession(): string | null {
  const url = new URL(window.location.href)
  if (!url.searchParams.has('sso_token') && !url.searchParams.has('state'))
    return readOwnershipSession()
  const tokens = url.searchParams.getAll('sso_token')
  const states = url.searchParams.getAll('state')
  url.searchParams.delete('sso_token')
  url.searchParams.delete('state')
  window.history.replaceState(
    window.history.state,
    document.title,
    url.pathname + url.search + url.hash,
  )
  let pending: string | null = null
  try {
    pending = window.sessionStorage.getItem(GPTCHAT_SSO_STATE_KEY)
    window.sessionStorage.removeItem(GPTCHAT_SSO_STATE_KEY)
  } catch {
    // A callback without its tab-local nonce cannot establish a session.
  }
  const token = tokens[0]
  if (
    tokens.length !== 1 ||
    states.length !== 1 ||
    !matchingState(pending, states[0]) ||
    !token ||
    /[\s\p{Cc}]/u.test(token)
  ) {
    // An untrusted callback cannot replace an already established session.
    return readOwnershipSession()
  }
  try {
    window.localStorage.setItem(GPTCHAT_SSO_STORAGE_KEY, token)
    unpersistedSession = null
  } catch {
    unpersistedSession = token
    // The current page can retain the session when persistence is unavailable.
  }
  return token
}

/** ownershipSignInURL creates a tab-local sign-in nonce and the established SSO redirect. */
export function ownershipSignInURL(): string {
  const destination = new URL(window.location.href)
  destination.searchParams.delete('sso_token')
  destination.searchParams.delete('state')
  const bytes = window.crypto.getRandomValues(new Uint8Array(32))
  const state = Array.from(bytes, (byte) =>
    byte.toString(16).padStart(2, '0'),
  ).join('')
  window.sessionStorage.setItem(GPTCHAT_SSO_STATE_KEY, state)
  destination.searchParams.set('state', state)
  const sso = new URL('https://sso.laisky.com/')
  sso.searchParams.set('redirect_to', destination.href)
  sso.searchParams.set('state', state)
  return sso.href
}

/** ownershipSessionHeader validates the separate session and returns its exact Bearer header. */
export function ownershipSessionHeader(token?: string): Record<string, string> {
  if (!token || /[\s\p{Cc}]/u.test(token)) {
    throw new Error('Sign in with Laisky SSO to manage private datasets.')
  }
  return { 'X-Laisky-SSO-Token': 'Bearer ' + token }
}
