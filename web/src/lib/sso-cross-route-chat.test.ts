// @vitest-environment-options {"url":"https://chat.laisky.com"}
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
it('keeps the chat.laisky.com binding while completing CV on its supported cross-route', async () => {
  vi.stubGlobal('crypto', webcrypto)
  history.replaceState(null, '', '/cv?theme=dark#draft')
  localStorage.setItem('gptchat_ownership_sso_token', 'existing-chat')
  const login = new URL(await beginCodeSignIn('cv'))
  const callback = new URL(login.searchParams.get('redirect_to')!)
  expect(callback.searchParams.get('sso_client_id')).toBe('ramjet-chat')
  const pending = JSON.parse(sessionStorage.getItem('ramjet_sso_code_pending')!)
  history.replaceState(
    null,
    '',
    '/?sso_code=single-use&sso_state=' + pending.state,
  )
  const fetcher = vi.fn(async (_url, init) => {
    expect(JSON.parse(init.body).client_id).toBe('ramjet-chat')
    return new Response(
      JSON.stringify({ token_type: 'Bearer', access_token: 'new-cv' }),
    )
  })
  vi.stubGlobal('fetch', fetcher)
  await completeCodeSignIn('chat')
  expect(fetcher).toHaveBeenCalledTimes(1)
  expect(readCodeSession('cv')).toBe('new-cv')
  expect(readCodeSession('chat')).toBe('existing-chat')
  expect(location.pathname + location.search + location.hash).toBe(
    '/cv?theme=dark#draft',
  )
})
