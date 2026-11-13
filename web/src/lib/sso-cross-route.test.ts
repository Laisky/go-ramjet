// @vitest-environment-options {"url":"https://cv.laisky.com"}
import { webcrypto } from 'node:crypto'
import { afterEach, expect, it, vi } from 'vitest'
import {
  beginCodeSignIn,
  completeCodeSignIn,
  readCodeSession,
} from './sso-code'

afterEach(() => {
  vi.unstubAllGlobals()
  localStorage.clear()
  sessionStorage.clear()
})
it('keeps the CV host client binding while completing Chat on its supported cross-route', async () => {
  vi.stubGlobal('crypto', webcrypto)
  history.replaceState(null, '', '/gptchat?theme=dark#draft')
  localStorage.setItem('cv_sso_token', 'existing-cv')
  const login = new URL(await beginCodeSignIn('chat'))
  const callback = new URL(login.searchParams.get('redirect_to')!)
  expect(callback.searchParams.get('sso_client_id')).toBe('ramjet-cv')
  const pending = JSON.parse(sessionStorage.getItem('ramjet_sso_code_pending')!)
  history.replaceState(
    null,
    '',
    '/?sso_code=single-use&sso_state=' + pending.state,
  )
  const fetcher = vi.fn(async (_url, init) => {
    expect(JSON.parse(init.body).client_id).toBe('ramjet-cv')
    return new Response(
      JSON.stringify({ token_type: 'Bearer', access_token: 'new-chat' }),
    )
  })
  vi.stubGlobal('fetch', fetcher)
  await completeCodeSignIn('cv')
  expect(fetcher).toHaveBeenCalledTimes(1)
  expect(readCodeSession('chat')).toBe('new-chat')
  expect(readCodeSession('cv')).toBe('existing-cv')
  expect(location.pathname + location.search + location.hash).toBe(
    '/gptchat?theme=dark#draft',
  )
})

it.each(['network', 'http', 'invalid-json', 'invalid-token'])(
  'restores a validated cross-route and preserves both sessions after %s failure',
  async (failure) => {
    vi.stubGlobal('crypto', webcrypto)
    const returnTo = '/gptchat?tab=data#draft'
    history.replaceState(null, '', returnTo)
    localStorage.setItem('cv_sso_token', 'existing-cv')
    localStorage.setItem('gptchat_ownership_sso_token', 'existing-chat')
    await beginCodeSignIn('chat')
    const pending = JSON.parse(
      sessionStorage.getItem('ramjet_sso_code_pending')!,
    )
    history.replaceState(
      null,
      '',
      '/?sso_code=single-use&sso_state=' + pending.state,
    )
    const fetcher = vi.fn(async () => {
      if (failure === 'network') throw new Error('Offline')
      if (failure === 'invalid-json') return new Response('not-json')
      return new Response(
        JSON.stringify({ token_type: 'Bearer', access_token: 'bad token' }),
        { status: failure === 'http' ? 400 : 200 },
      )
    })
    vi.stubGlobal('fetch', fetcher)
    await completeCodeSignIn('cv')
    expect(location.pathname + location.search + location.hash).toBe(returnTo)
    expect(readCodeSession('chat')).toBe('existing-chat')
    expect(readCodeSession('cv')).toBe('existing-cv')
    await completeCodeSignIn('cv')
    expect(fetcher).toHaveBeenCalledTimes(1)
  },
)
