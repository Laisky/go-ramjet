/**
 * CV editing authorization helpers.
 *
 * CV accepts Laisky SSO sessions only after the backend has verified them with
 * SSO and confirmed the account is the configured CV owner. Holding a token is
 * never enough to offer editing.
 */

export const CV_AUTH_TOKEN_STORAGE_KEY = 'cv_sso_token'
const SSO_TOKEN_PARAM = 'sso_token'

/**
 * CvAuthStatus is the editor's view of the current session.
 * - anonymous: no session is stored.
 * - checking: the session is being verified.
 * - owner: the backend verified the session and the account may edit.
 * - invalid: SSO rejected the session (expired, revoked, tampered).
 * - forbidden: a valid session for an account that may not edit the CV.
 * - unavailable: SSO could not answer; the session may still be valid.
 */
export type CvAuthStatus =
  'anonymous' | 'checking' | 'owner' | 'invalid' | 'forbidden' | 'unavailable'

/**
 * readStoredAuthToken returns the persisted SSO token, or null.
 */
export function readStoredAuthToken(): string | null {
  try {
    const token = window.localStorage.getItem(CV_AUTH_TOKEN_STORAGE_KEY)?.trim()
    return token ? token : null
  } catch {
    return null
  }
}

/**
 * clearAuthToken removes the persisted SSO token.
 */
export function clearAuthToken() {
  try {
    window.localStorage.removeItem(CV_AUTH_TOKEN_STORAGE_KEY)
  } catch {
    // localStorage may be unavailable; nothing is persisted then.
  }
}

/**
 * consumeAuthTokenFromLocation handles the SSO callback synchronously: it moves
 * an sso_token from the URL into storage and scrubs it from the address bar
 * before the page issues any request, so the token never appears in history,
 * referrers or request logs. It returns the effective session token.
 */
export function consumeAuthTokenFromLocation(): string | null {
  const params = new URLSearchParams(window.location.search)
  const token = params.get(SSO_TOKEN_PARAM)?.trim()
  if (params.has(SSO_TOKEN_PARAM)) {
    params.delete(SSO_TOKEN_PARAM)
    const rest = params.toString()
    window.history.replaceState(
      window.history.state,
      document.title,
      `${window.location.pathname}${rest ? `?${rest}` : ''}${window.location.hash}`,
    )
  }
  if (token) {
    try {
      window.localStorage.setItem(CV_AUTH_TOKEN_STORAGE_KEY, token)
    } catch {
      // Without storage the token still authorizes this page view.
    }
    return token
  }
  return readStoredAuthToken()
}

/**
 * authStatusFromResponse maps a protected-route HTTP status to the session
 * state it proves. It returns null for statuses that say nothing about the
 * session (success is handled by the caller, other errors are generic).
 */
export function authStatusFromResponse(status: number): CvAuthStatus | null {
  switch (status) {
    case 401:
      return 'invalid'
    case 403:
      return 'forbidden'
    case 503:
      return 'unavailable'
    default:
      return null
  }
}

/**
 * describeAuthStatus returns the user-facing explanation for a session state,
 * or null when nothing needs explaining.
 */
export function describeAuthStatus(status: CvAuthStatus): string | null {
  switch (status) {
    case 'invalid':
      return 'Your SSO session is invalid or has expired. Please sign in again.'
    case 'forbidden':
      return 'This account is not allowed to edit the CV. Sign in with the owner account.'
    case 'unavailable':
      return 'SSO is temporarily unavailable. Please try again shortly.'
    default:
      return null
  }
}

/**
 * shouldDiscardToken reports whether a session state proves the stored token is
 * useless here, so the visitor can sign in with another account.
 */
export function shouldDiscardToken(status: CvAuthStatus): boolean {
  return status === 'invalid' || status === 'forbidden'
}

/**
 * verifyCvSession asks the backend whether token belongs to the CV owner.
 * Network failures are reported as unavailable, never as an invalid session.
 */
export async function verifyCvSession(
  token: string,
  signal: AbortSignal,
): Promise<CvAuthStatus> {
  try {
    const response = await fetch('/cv/auth/session', {
      signal,
      cache: 'no-store',
      headers: { Authorization: `Bearer ${token}` },
    })
    if (response.ok) return 'owner'
    return authStatusFromResponse(response.status) ?? 'unavailable'
  } catch (error) {
    if (signal.aborted) throw error
    return 'unavailable'
  }
}
