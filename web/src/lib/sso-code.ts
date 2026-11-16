/** Registered SSO code handoff shared by CV and GPTChat. */
export type SsoApp = 'cv' | 'chat'
const issuer = 'https://sso.laisky.com'
const pendingKey = 'ramjet_sso_code_pending'
const keys = { cv: 'cv_sso_token', chat: 'gptchat_ownership_sso_token' }
const volatileSessions: Partial<Record<SsoApp, string>> = {}

/** readCodeSession preserves existing sessions and supports storage-disabled page views. */
export function readCodeSession(app: SsoApp): string | null {
  try {
    return volatileSessions[app] || localStorage.getItem(keys[app]) || null
  } catch {
    return volatileSessions[app] || null
  }
}

/** clearCodeSession signs out only the specified application. */
export function clearCodeSession(app: SsoApp): void {
  delete volatileSessions[app]
  try {
    localStorage.removeItem(keys[app])
  } catch {
    /* Storage may be disabled. */
  }
}

type Pending = {
  app: SsoApp
  state: string
  verifier: string
  callback: string
  returnTo: string
  created: number
}

/** base64url encodes random bytes in the issuer's unpadded format. */
function base64url(bytes: Uint8Array): string {
  return btoa(String.fromCharCode(...bytes))
    .replaceAll('+', '-')
    .replaceAll('/', '_')
    .replace(/=+$/, '')
}

/** equalState compares equal-length nonces without early character exits. */
function equalState(a: string, b: string): boolean {
  if (a.length !== 43 || b.length !== 43) return false
  let difference = 0
  for (let i = 0; i < a.length; i++)
    difference |= a.charCodeAt(i) ^ b.charCodeAt(i)
  return difference === 0
}

/** registryClient binds issuer registration to a hostname, independently of the app route. */
function registryClient(hostname: string, app: SsoApp): string {
  const hosts: Record<string, string> = {
    'cv.laisky.com': 'ramjet-cv',
    'chat.laisky.com': 'ramjet-chat',
    'chat2.laisky.com': 'ramjet-chat2',
  }
  return Object.hasOwn(hosts, hostname) ? hosts[hostname] : 'ramjet-' + app
}

/** returnApp validates a stored route against the actual SPA routes and root landing app. */
function returnApp(returnTo: unknown, landing: SsoApp | null): SsoApp | null {
  if (
    typeof returnTo !== 'string' ||
    !returnTo.startsWith('/') ||
    returnTo.startsWith('//') ||
    returnTo.includes('\\') ||
    /[\p{Cc}]/u.test(returnTo)
  )
    return null
  const parsed = new URL(returnTo, window.location.origin)
  if (parsed.origin !== window.location.origin) return null
  const path = parsed.pathname.toLowerCase().replace(/\/+$/, '') || '/'
  if (path === '/cv') return 'cv'
  if (path === '/gptchat') return 'chat'
  return path === '/' ? landing : null
}

/** beginCodeSignIn stores tab-local PKCE state and builds a registered callback request. */
export async function beginCodeSignIn(app: SsoApp): Promise<string> {
  const current = new URL(window.location.href)
  // Upgrade before persisting tab state: storage belongs to an origin.
  if (
    current.protocol === 'http:' &&
    current.hostname.endsWith('.laisky.com')
  ) {
    current.protocol = 'https:'
    return current.href
  }
  const state = base64url(crypto.getRandomValues(new Uint8Array(32)))
  const verifier = base64url(crypto.getRandomValues(new Uint8Array(32)))
  const challenge = base64url(
    new Uint8Array(
      await crypto.subtle.digest('SHA-256', new TextEncoder().encode(verifier)),
    ),
  )
  for (const key of ['sso_code', 'sso_state', 'sso_token', 'state'])
    current.searchParams.delete(key)
  const callback = new URL('/', current.origin)
  const pending: Pending = {
    app,
    state,
    verifier,
    callback: callback.href,
    returnTo: current.pathname + current.search + current.hash,
    created: Date.now(),
  }
  sessionStorage.setItem(pendingKey, JSON.stringify(pending))
  callback.search = new URLSearchParams({
    sso_flow: 'code',
    sso_client_id: registryClient(current.hostname, app),
    sso_state: state,
    sso_challenge: challenge,
    sso_challenge_method: 'S256',
  }).toString()
  const login = new URL(issuer)
  login.searchParams.set('redirect_to', callback.href)
  return login.href
}

/** completeCodeSignIn scrubs callback values before network or app loading and exchanges once. */
export async function completeCodeSignIn(app: SsoApp | null): Promise<void> {
  const url = new URL(window.location.href)
  const codes = url.searchParams.getAll('sso_code')
  const states = url.searchParams.getAll('sso_state')
  const hasCallback = ['sso_code', 'sso_state', 'sso_token'].some((key) =>
    url.searchParams.has(key),
  )
  if (!hasCallback) return
  for (const key of ['sso_code', 'sso_state', 'sso_token', 'state'])
    url.searchParams.delete(key)
  history.replaceState(
    history.state,
    document.title,
    url.pathname + url.search + url.hash,
  )
  let pending: Pending
  try {
    const raw = sessionStorage.getItem(pendingKey)
    sessionStorage.removeItem(pendingKey)
    pending = JSON.parse(raw || 'null')
  } catch {
    return
  }
  // App identity comes from the bound return route, not the callback root's host theme.
  // This preserves /cv and /gptchat on all supported hosts without mixing sessions.
  if (!pending || (pending.app !== 'cv' && pending.app !== 'chat')) return
  const targetApp = returnApp(pending?.returnTo, app || pending?.app)
  if (!targetApp || pending?.app !== targetApp) return
  app = targetApp
  if (
    !pending ||
    !app ||
    pending.app !== app ||
    codes.length !== 1 ||
    states.length !== 1 ||
    typeof pending.state !== 'string' ||
    !equalState(pending.state, states[0]) ||
    typeof pending.verifier !== 'string' ||
    !/^[A-Za-z0-9_-]{43}$/.test(pending.verifier) ||
    pending.callback !== new URL('/', url.origin).href ||
    url.pathname !== '/' ||
    !Number.isFinite(pending.created) ||
    Date.now() - pending.created > 10 * 60_000 ||
    Date.now() < pending.created ||
    typeof pending.returnTo !== 'string' ||
    !pending.returnTo.startsWith('/') ||
    pending.returnTo.startsWith('//') ||
    pending.returnTo.includes('\\')
  )
    return
  // Remember the scrubbed callback so completion cannot overwrite newer navigation.
  const callbackLocation = window.location.href
  try {
    const response = await fetch(issuer + '/sso/token', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      credentials: 'omit',
      cache: 'no-store',
      redirect: 'error',
      referrerPolicy: 'no-referrer',
      signal: AbortSignal.timeout(15_000),
      body: JSON.stringify({
        client_id: registryClient(url.hostname, app),
        redirect_uri: pending.callback,
        code: codes[0],
        state: states[0],
        code_verifier: pending.verifier,
      }),
    })
    if (!response.ok) return
    const body = await response.json()
    if (
      body.token_type !== 'Bearer' ||
      typeof body.access_token !== 'string' ||
      !body.access_token ||
      /[\s\p{Cc}]/u.test(body.access_token)
    )
      return
    try {
      localStorage.setItem(keys[app], body.access_token)
      delete volatileSessions[app]
    } catch {
      volatileSessions[app] = body.access_token
    }
  } catch {
    // Preserve established sessions; a lost exchange requires a fresh sign-in.
  } finally {
    // Restore validated context on every exchange outcome, before app bootstrap.
    if (window.location.href === callbackLocation)
      history.replaceState(history.state, document.title, pending.returnTo)
  }
}
