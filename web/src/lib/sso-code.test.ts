import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { webcrypto } from 'node:crypto'
import {
  beginCodeSignIn,
  completeCodeSignIn,
  clearCodeSession,
  readCodeSession,
} from './sso-code'

describe('registered SSO migration', () => {
  beforeEach(() => {
    clearCodeSession('cv')
    clearCodeSession('chat')
    localStorage.clear()
    sessionStorage.clear()
    history.replaceState(null, '', '/cv?tab=edit#details')
    vi.stubGlobal('crypto', webcrypto)
  })
  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })
  it('uses registered code flow and never requests a bearer redirect', async () => {
    const login = new URL(await beginCodeSignIn('cv'))
    const callback = new URL(login.searchParams.get('redirect_to')!)
    expect(callback.pathname).toBe('/')
    expect(callback.searchParams.get('sso_client_id')).toBe('ramjet-cv')
    expect(callback.searchParams.get('sso_flow')).toBe('code')
    expect(callback.searchParams.get('sso_challenge_method')).toBe('S256')
    expect(callback.searchParams.get('sso_challenge')).toHaveLength(43)
    expect(callback.searchParams.has('sso_token')).toBe(false)
  })
  it('scrubs before exchange, binds verifier, restores route and preserves distinct sessions', async () => {
    const request = new URL(
      new URL(await beginCodeSignIn('cv')).searchParams.get('redirect_to')!,
    )
    const state = request.searchParams.get('sso_state')!
    localStorage.setItem('gptchat_ownership_sso_token', 'chat-old')
    history.replaceState(null, '', '/?sso_code=single-use&sso_state=' + state)
    const fetcher = vi.fn(async (_url, init) => {
      expect(location.search).not.toContain('sso_code')
      const body = JSON.parse(init.body)
      expect(body.client_id).toBe('ramjet-cv')
      expect(body.state).toBe(state)
      expect(body.code_verifier).toHaveLength(43)
      return {
        ok: true,
        json: async () => ({ access_token: 'cv-new', token_type: 'Bearer' }),
      }
    })
    vi.stubGlobal('fetch', fetcher)
    await completeCodeSignIn('cv')
    expect(localStorage.getItem('cv_sso_token')).toBe('cv-new')
    expect(localStorage.getItem('gptchat_ownership_sso_token')).toBe('chat-old')
    expect(location.pathname + location.search + location.hash).toBe(
      '/cv?tab=edit#details',
    )
    await completeCodeSignIn('cv')
    expect(fetcher).toHaveBeenCalledTimes(1)
  })
  it.each([
    'sso_code=x&sso_state=wrong',
    'sso_token=attacker',
    'sso_code=x&sso_code=y&sso_state=z',
  ])('rejects unbound callbacks: %s', async (query) => {
    localStorage.setItem('cv_sso_token', 'existing')
    const fetcher = vi.fn()
    vi.stubGlobal('fetch', fetcher)
    history.replaceState(null, '', '/?' + query)
    await completeCodeSignIn('cv')
    expect(fetcher).not.toHaveBeenCalled()
    expect(location.search).toBe('')
    expect(localStorage.getItem('cv_sso_token')).toBe('existing')
  })
  it.each([
    'wrong-app',
    'root-landing-mismatch',
    'unknown-route',
    'backslash-return',
    'control-return',
    'expired',
    'future',
    'duplicate-state',
    'wrong-route',
    'external-return',
    'wrong-callback',
    'malformed-verifier',
  ])('rejects bound but invalid context: %s', async (problem) => {
    await beginCodeSignIn('cv')
    const pending = JSON.parse(
      sessionStorage.getItem('ramjet_sso_code_pending')!,
    )
    if (problem === 'wrong-app') pending.app = 'chat'
    if (problem === 'root-landing-mismatch') pending.returnTo = '/'
    if (problem === 'unknown-route') pending.returnTo = '/other'
    if (problem === 'backslash-return') pending.returnTo = '/\\other-host/'
    if (problem === 'control-return') pending.returnTo = '/\t/other-host/'
    if (problem === 'expired') pending.created -= 600001
    if (problem === 'future') pending.created += 60000
    if (problem === 'external-return') pending.returnTo = '//attacker.example/'
    if (problem === 'wrong-callback')
      pending.callback = 'https://attacker.example/'
    if (problem === 'malformed-verifier') pending.verifier = 'short'
    sessionStorage.setItem('ramjet_sso_code_pending', JSON.stringify(pending))
    history.replaceState(
      null,
      '',
      (problem === 'wrong-route' ? '/cv' : '/') +
        '?sso_code=x&sso_state=' +
        pending.state +
        (problem === 'duplicate-state' ? '&sso_state=' + pending.state : ''),
    )
    vi.stubGlobal('fetch', vi.fn())
    await completeCodeSignIn(
      problem === 'wrong-app' || problem === 'root-landing-mismatch'
        ? 'chat'
        : 'cv',
    )
    expect(fetch).not.toHaveBeenCalled()
    expect(readCodeSession('cv')).toBeNull()
    expect(location.search).toBe('')
  })

  it.each([
    'network',
    'http',
    'malformed',
    'invalid-json',
    'null-body',
    'timeout',
  ])(
    'preserves established sessions after %s failure and never retries a consumed callback',
    async (failure) => {
      await beginCodeSignIn('cv')
      const pending = JSON.parse(
        sessionStorage.getItem('ramjet_sso_code_pending')!,
      )
      localStorage.setItem('cv_sso_token', 'existing')
      history.replaceState(null, '', '/?sso_code=x&sso_state=' + pending.state)
      vi.stubGlobal(
        'fetch',
        vi.fn(async () => {
          if (failure === 'network') throw new Error('Network unavailable')
          if (failure === 'timeout')
            throw new DOMException('Timed out', 'TimeoutError')
          if (failure === 'invalid-json') return new Response('not-json')
          if (failure === 'null-body') return new Response('null')
          return new Response(
            JSON.stringify({ token_type: 'Bearer', access_token: 'bad token' }),
            { status: failure === 'http' ? 400 : 200 },
          )
        }),
      )
      await completeCodeSignIn('cv')
      await completeCodeSignIn('cv')
      expect(readCodeSession('cv')).toBe('existing')
      expect(fetch).toHaveBeenCalledTimes(1)
      expect(sessionStorage.getItem('ramjet_sso_code_pending')).toBeNull()
      expect(location.pathname + location.search + location.hash).toBe(
        '/cv?tab=edit#details',
      )
    },
  )

  it.each([true, false])(
    'does not overwrite newer navigation when exchange success=%s',
    async (success) => {
      await beginCodeSignIn('cv')
      const pending = JSON.parse(
        sessionStorage.getItem('ramjet_sso_code_pending')!,
      )
      history.replaceState(null, '', '/?sso_code=x&sso_state=' + pending.state)
      let finish!: (response: Response) => void
      vi.stubGlobal(
        'fetch',
        vi.fn(
          () =>
            new Promise<Response>((resolve) => {
              finish = resolve
            }),
        ),
      )
      const completion = completeCodeSignIn('cv')
      expect(location.pathname + location.search + location.hash).toBe('/')
      history.replaceState(null, '', '/gptchat?tab=new#navigation')
      finish(
        new Response(
          JSON.stringify({ token_type: 'Bearer', access_token: 'new-token' }),
          { status: success ? 200 : 400 },
        ),
      )
      await completion
      expect(location.pathname + location.search + location.hash).toBe(
        '/gptchat?tab=new#navigation',
      )
    },
  )

  it('retains a successfully exchanged session in memory when persistence is unavailable', async () => {
    history.replaceState(null, '', '/gptchat')
    await beginCodeSignIn('chat')
    const pending = JSON.parse(
      sessionStorage.getItem('ramjet_sso_code_pending')!,
    )
    history.replaceState(null, '', '/?sso_code=x&sso_state=' + pending.state)
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('Unavailable')
    })
    vi.stubGlobal(
      'fetch',
      vi.fn(
        async () =>
          new Response(
            JSON.stringify({ token_type: 'Bearer', access_token: 'new-chat' }),
          ),
      ),
    )
    await completeCodeSignIn('chat')
    expect(readCodeSession('chat')).toBe('new-chat')
    expect(readCodeSession('cv')).toBeNull()
    clearCodeSession('chat')
    expect(readCodeSession('chat')).toBeNull()
  })
})
