import {
  beginCodeSignIn,
  clearCodeSession,
  readCodeSession,
} from '@/lib/sso-code'
/** GPTChat ownership sessions are independent of model keys and CV sessions. */
export const GPTCHAT_SSO_STORAGE_KEY = 'gptchat_ownership_sso_token'
export const GPTCHAT_SSO_STATE_KEY = 'gptchat_ownership_sso_pending_state'
// Kept for scrubbing old tabs during the rollout; never used to admit URL bearers.
/** readOwnershipSession reads only the established GPTChat session. */
export function readOwnershipSession(): string | null {
  return readCodeSession('chat')
}
/** clearOwnershipSession signs out GPTChat without touching CV sessions. */
export function clearOwnershipSession(): void {
  clearCodeSession('chat')
}
/** consumeOwnershipSession rejects legacy callbacks and scrubs their credentials. */
export function consumeOwnershipSession(): string | null {
  const url = new URL(window.location.href)
  if (url.searchParams.has('sso_token') || url.searchParams.has('state')) {
    url.searchParams.delete('sso_token')
    url.searchParams.delete('state')
    history.replaceState(
      history.state,
      document.title,
      url.pathname + url.search + url.hash,
    )
    try {
      sessionStorage.removeItem(GPTCHAT_SSO_STATE_KEY)
    } catch {
      /* Storage disabled. */
    }
  }
  return readOwnershipSession()
}

/** ownershipSignInURL creates a tab-local sign-in nonce and the established SSO redirect. */
export function ownershipSignInURL(): Promise<string> {
  return beginCodeSignIn('chat')
}

/** ownershipSessionHeader validates the separate session and returns its exact Bearer header. */
export function ownershipSessionHeader(token?: string): Record<string, string> {
  if (!token || /[\s\p{Cc}]/u.test(token)) {
    throw new Error('Sign in with Laisky SSO to manage private datasets.')
  }
  return { 'X-Laisky-SSO-Token': 'Bearer ' + token }
}

/** bootstrapOwnershipSession admits callbacks only on routes that actually mount GPTChat. */
export function bootstrapOwnershipSession(siteId: string): void {
  const path = window.location.pathname.toLowerCase().replace(/\/+$/, '') || '/'
  if (path === '/gptchat' || (path === '/' && siteId === 'chat')) {
    consumeOwnershipSession()
  }
}
