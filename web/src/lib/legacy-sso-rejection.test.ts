import { expect, it } from 'vitest'
import { consumeAuthTokenFromLocation } from '../pages/cv/cv-auth'
import { consumeOwnershipSession } from '../pages/gptchat/utils/ownership-session'

it('does not replace existing CV authorization with a URL bearer', () => {
  localStorage.setItem('cv_sso_token', 'existing')
  history.replaceState(null, '', '/cv?sso_token=forged')
  expect(consumeAuthTokenFromLocation()).toBe('existing')
  expect(location.search).toBe('')
})
it('does not admit legacy chat bearers even with legacy pending state', () => {
  localStorage.setItem('gptchat_ownership_sso_token', 'existing')
  sessionStorage.setItem('gptchat_ownership_sso_pending_state', 'a'.repeat(64))
  history.replaceState(
    null,
    '',
    '/gptchat?sso_token=forged&state=' + 'a'.repeat(64),
  )
  expect(consumeOwnershipSession()).toBe('existing')
  expect(location.search).toBe('')
})
