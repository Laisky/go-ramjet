import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  clearOwnershipSession,
  consumeOwnershipSession,
  GPTCHAT_SSO_STORAGE_KEY,
  GPTCHAT_SSO_STATE_KEY,
  ownershipSessionHeader,
  ownershipSignInURL,
  readOwnershipSession,
} from '../ownership-session'

const state = 'a'.repeat(64)

describe('GPTChat ownership session', () => {
  beforeEach(() => {
    clearOwnershipSession()
    localStorage.clear()
    sessionStorage.clear()
    history.replaceState(null, '', '/gptchat')
  })
  afterEach(() => vi.restoreAllMocks())

  it('does not reuse CV or model keys as ownership sessions', () => {
    localStorage.setItem('cv_sso_token', 'cv-session')
    localStorage.setItem('api_token', 'model-key')
    expect(consumeOwnershipSession()).toBeNull()
  })

  it.each([
    'sso_token=attacker-session',
    'sso_token=attacker-session&state=' + 'b'.repeat(64),
    'sso_token=attacker-session&state=' + state + '&state=' + state,
    'sso_token=attacker-session&sso_token=other&state=' + state,
    'state=' + state,
  ])('scrubs and rejects an unsolicited or ambiguous callback: %s', (query) => {
    sessionStorage.setItem(GPTCHAT_SSO_STATE_KEY, state)
    history.replaceState(null, '', '/gptchat?' + query)
    expect(consumeOwnershipSession()).toBeNull()
    expect(location.search).toBe('')
    expect(readOwnershipSession()).toBeNull()
    expect(sessionStorage.getItem(GPTCHAT_SSO_STATE_KEY)).toBeNull()
  })

  it.each(['', 'session with space', 'session\n'])(
    'rejects a malformed callback without replacing an established session',
    (token) => {
      localStorage.setItem(GPTCHAT_SSO_STORAGE_KEY, 'previous-session')
      sessionStorage.setItem(GPTCHAT_SSO_STATE_KEY, state)
      history.replaceState(
        null,
        '',
        '/gptchat?sso_token=' + encodeURIComponent(token) + '&state=' + state,
      )
      expect(consumeOwnershipSession()).toBe('previous-session')
      expect(location.search).toBe('')
      expect(readOwnershipSession()).toBe('previous-session')
    },
  )

  it('cannot consume a matching-looking callback without tab-local pending state', () => {
    history.replaceState(
      null,
      '',
      '/gptchat?sso_token=attacker-session&state=' + state,
    )
    expect(consumeOwnershipSession()).toBeNull()
    expect(location.search).toBe('')
  })

  it('signs out only GPTChat and keeps CV storage intact', () => {
    localStorage.setItem(GPTCHAT_SSO_STORAGE_KEY, 'gptchat-session')
    localStorage.setItem('cv_sso_token', 'cv-session')
    clearOwnershipSession()
    expect(readOwnershipSession()).toBeNull()
    expect(localStorage.getItem('cv_sso_token')).toBe('cv-session')
  })

  it('fails closed if pending sign-in state cannot be persisted', async () => {
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('Unavailable')
    })
    await expect(ownershipSignInURL()).rejects.toThrow()
  })

  it('preserves the original session in the separate header', () => {
    expect(ownershipSessionHeader('original.session.signature')).toEqual({
      'X-Laisky-SSO-Token': 'Bearer original.session.signature',
    })
  })

  it.each([undefined, '', 'Bearer token', 'token\n'])(
    'rejects missing or malformed header values without echoing credentials',
    (token) => {
      expect(() => ownershipSessionHeader(token)).toThrow(
        'Sign in with Laisky SSO to manage private datasets.',
      )
    },
  )
})
