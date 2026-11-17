import { describe, expect, it, vi } from 'vitest'

import {
  apiTokenCollidesWithSyncKey,
  canCompleteApiTokenReset,
  createFreeTierToken,
  createSyncKey,
  describeApiTokenKind,
  findRecoverableApiToken,
  FREETIER_TOKEN_PREFIX,
  isApiTokenClobberedBySyncKey,
  isGeneratedSyncKey,
  isSameApiEndpoint,
  isUserSuppliedApiToken,
  normalizeApiBase,
  SYNC_KEY_PREFIX,
} from '../api-token'

const SYNC_KEY = 'sync-k2x9q4w7p1z8m3'
const LEGACY_PAGE_SYNC_KEY = `sync-${'Ab3'.repeat(21)}x`
const PROVIDER_A = 'https://provider-a.example'
const PROVIDER_B = 'https://provider-b.example'

describe('describeApiTokenKind', () => {
  it.each([
    [undefined, 'missing'],
    ['   ', 'missing'],
    ['DEFAULT_PROXY_TOKEN', 'legacy-default'],
    ['FREETIER-abc', 'freetier'],
    ['sk-abc', 'byok'],
    ['laisky-abc', 'byok'],
    ['sync-abc', 'custom'],
  ])('classifies %s as %s', (token, kind) => {
    expect(describeApiTokenKind(token)).toBe(kind)
  })

  it('creates free-tier tokens that classify as freetier', () => {
    expect(describeApiTokenKind(createFreeTierToken())).toBe('freetier')
  })
})

describe('credential generation', () => {
  it('creates distinct prefixed sync keys of the generated shape', () => {
    const a = createSyncKey()
    expect(a.startsWith(SYNC_KEY_PREFIX)).toBe(true)
    expect(a).toMatch(/^sync-[a-z0-9]{32}$/)
    expect(isGeneratedSyncKey(a)).toBe(true)
    expect(createSyncKey()).not.toBe(a)
  })

  it('creates distinct free-tier tokens', () => {
    const a = createFreeTierToken()
    expect(a).toMatch(/^FREETIER-[a-z0-9]{32}$/)
    expect(a.startsWith(FREETIER_TOKEN_PREFIX)).toBe(true)
    expect(createFreeTierToken()).not.toBe(a)
  })

  it('never derives credentials from Math.random', () => {
    const spy = vi.spyOn(Math, 'random').mockReturnValue(0.123456789)
    try {
      expect(createSyncKey()).not.toBe(createSyncKey())
      expect(createFreeTierToken()).not.toBe(createFreeTierToken())
      expect(spy).not.toHaveBeenCalled()
    } finally {
      spy.mockRestore()
    }
  })
})

describe('isGeneratedSyncKey', () => {
  it.each([
    [SYNC_KEY, true],
    [LEGACY_PAGE_SYNC_KEY, true],
    ['sync-0123456789ab', true],
    ['sync-short', false],
    ['sync-team/alpha_01.key', false],
    ['sync-previous-key', false],
    ['sk-k2x9q4w7p1z8m3', false],
    ['', false],
  ])('treats %s as generated=%s', (value, generated) => {
    expect(isGeneratedSyncKey(value)).toBe(generated)
  })
})

describe('isApiTokenClobberedBySyncKey', () => {
  it('flags a non-API-shaped token identical to the sync key', () => {
    expect(isApiTokenClobberedBySyncKey('my-sync', 'my-sync')).toBe(true)
    expect(isApiTokenClobberedBySyncKey(' my-sync ', 'my-sync')).toBe(true)
  })

  it('keeps API-shaped tokens even when reused as the sync key', () => {
    expect(isApiTokenClobberedBySyncKey('sk-abc', 'sk-abc')).toBe(false)
    expect(isApiTokenClobberedBySyncKey('laisky-abc', 'laisky-abc')).toBe(false)
  })

  it('flags a generated sync key even when it is not the current one', () => {
    expect(isApiTokenClobberedBySyncKey(SYNC_KEY, 'sync-current')).toBe(true)
    expect(isApiTokenClobberedBySyncKey(LEGACY_PAGE_SYNC_KEY, undefined)).toBe(
      true,
    )
    expect(isApiTokenClobberedBySyncKey(createSyncKey(), 'custom-key')).toBe(
      true,
    )
  })

  it('keeps a custom token that merely starts with sync-', () => {
    expect(
      isApiTokenClobberedBySyncKey('sync-team/alpha_01.key', 'custom-sync'),
    ).toBe(false)
  })

  it('ignores differing or empty values', () => {
    expect(isApiTokenClobberedBySyncKey('sk-abc', SYNC_KEY)).toBe(false)
    expect(isApiTokenClobberedBySyncKey('paid-token', 'other-key')).toBe(false)
    expect(isApiTokenClobberedBySyncKey('paid-token', 'paid-tokem')).toBe(false)
    expect(isApiTokenClobberedBySyncKey('', '')).toBe(false)
    expect(isApiTokenClobberedBySyncKey(undefined, SYNC_KEY)).toBe(false)
  })
})

describe('apiTokenCollidesWithSyncKey', () => {
  it('collides only for a custom token equal to the sync key', () => {
    expect(apiTokenCollidesWithSyncKey('my-secret', 'my-secret')).toBe(true)
    expect(apiTokenCollidesWithSyncKey('sk-secret', 'sk-secret')).toBe(false)
    expect(apiTokenCollidesWithSyncKey('my-secret', 'my-secret2')).toBe(false)
    expect(apiTokenCollidesWithSyncKey('my-secret', '')).toBe(false)
    expect(apiTokenCollidesWithSyncKey('', '')).toBe(false)
  })
})

describe('isUserSuppliedApiToken', () => {
  it.each([
    ['sk-abc', true],
    ['laisky-abc', true],
    ['paid-token', true],
    ['sync-team/alpha_01.key', true],
    ['FREETIER-abc', false],
    ['DEFAULT_PROXY_TOKEN', false],
    ['', false],
    [SYNC_KEY, false],
  ])('treats %s as user-supplied=%s', (token, supplied) => {
    expect(isUserSuppliedApiToken(token, 'custom-sync')).toBe(supplied)
  })

  it('rejects a custom token that copies the sync key', () => {
    expect(isUserSuppliedApiToken('my-secret', 'my-secret')).toBe(false)
  })
})

describe('normalizeApiBase / isSameApiEndpoint', () => {
  it('treats an empty base as the default endpoint', () => {
    expect(isSameApiEndpoint('', 'https://api.openai.com/')).toBe(true)
    expect(isSameApiEndpoint(undefined, 'https://API.openai.com')).toBe(true)
  })

  it('ignores scheme/host case, default ports, and trailing slashes', () => {
    expect(normalizeApiBase('HTTPS://Provider-A.example:443/v1//')).toBe(
      'https://provider-a.example/v1',
    )
    expect(isSameApiEndpoint(`${PROVIDER_A}/`, PROVIDER_A.toUpperCase())).toBe(
      true,
    )
  })

  it('distinguishes hosts, ports, paths, and schemes', () => {
    expect(isSameApiEndpoint(PROVIDER_A, PROVIDER_B)).toBe(false)
    expect(isSameApiEndpoint(PROVIDER_A, `${PROVIDER_A}:8443`)).toBe(false)
    expect(isSameApiEndpoint(PROVIDER_A, `${PROVIDER_A}/v2`)).toBe(false)
    expect(isSameApiEndpoint(PROVIDER_A, 'http://provider-a.example')).toBe(
      false,
    )
  })

  it('compares non-URL bases verbatim apart from trailing slashes', () => {
    expect(isSameApiEndpoint('/relative/', '/relative')).toBe(true)
    expect(isSameApiEndpoint('/relative', '/other')).toBe(false)
  })
})

describe('findRecoverableApiToken', () => {
  const target = { apiBase: PROVIDER_A, syncKey: SYNC_KEY }

  it('recovers the single token used on the same endpoint', () => {
    expect(
      findRecoverableApiToken(
        [
          { api_base: PROVIDER_A, api_token: 'sk-a', updated_at: 1 },
          { api_base: PROVIDER_A, api_token: ' sk-a ', updated_at: 5 },
          { api_base: PROVIDER_B, api_token: 'sk-b', updated_at: 9 },
        ],
        target,
      ),
    ).toEqual({ token: 'sk-a', outcome: 'recovered', candidates: 1 })
  })

  it('never recovers a token from another endpoint, however recent', () => {
    expect(
      findRecoverableApiToken(
        [{ api_base: PROVIDER_B, api_token: 'sk-b', updated_at: 999 }],
        target,
      ),
    ).toEqual({ token: '', outcome: 'none', candidates: 0 })
  })

  it('refuses to choose between distinct same-endpoint tokens', () => {
    expect(
      findRecoverableApiToken(
        [
          { api_base: PROVIDER_A, api_token: 'sk-one', updated_at: 1 },
          { api_base: PROVIDER_A, api_token: 'sk-two', updated_at: 2 },
        ],
        target,
      ),
    ).toEqual({ token: '', outcome: 'ambiguous', candidates: 2 })
  })

  it('skips clobbered, free-tier, legacy, and missing tokens', () => {
    expect(
      findRecoverableApiToken(
        [
          { api_base: PROVIDER_A, api_token: SYNC_KEY },
          { api_base: PROVIDER_A, api_token: LEGACY_PAGE_SYNC_KEY },
          { api_base: PROVIDER_A, api_token: 'FREETIER-x' },
          { api_base: PROVIDER_A, api_token: 'DEFAULT_PROXY_TOKEN' },
          { api_base: PROVIDER_A, api_token: '' },
          null,
          undefined,
        ],
        target,
      ).outcome,
    ).toBe('none')
  })

  it('matches sessions without an api_base against the default endpoint', () => {
    expect(
      findRecoverableApiToken([{ api_token: 'sk-default' }], {
        apiBase: 'https://api.openai.com/',
        syncKey: SYNC_KEY,
      }).token,
    ).toBe('sk-default')
  })
})

describe('canCompleteApiTokenReset', () => {
  it('completes only a pending reset that now has a user key', () => {
    const base = { sync_key: SYNC_KEY }
    expect(
      canCompleteApiTokenReset({
        ...base,
        api_token: 'sk-new',
        api_token_reset_required: true,
      }),
    ).toBe(true)
    expect(
      canCompleteApiTokenReset({
        ...base,
        api_token: 'FREETIER-x',
        api_token_reset_required: true,
      }),
    ).toBe(false)
    expect(
      canCompleteApiTokenReset({
        ...base,
        api_token: SYNC_KEY,
        api_token_reset_required: true,
      }),
    ).toBe(false)
    expect(
      canCompleteApiTokenReset({
        ...base,
        api_token: 'sk-new',
        api_token_reset_required: false,
      }),
    ).toBe(false)
  })
})
