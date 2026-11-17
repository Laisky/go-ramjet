import { describe, expect, it } from 'vitest'

import {
  createFreeTierToken,
  createSyncKey,
  describeApiTokenKind,
  findRecoverableApiToken,
  hasUsableApiToken,
  isApiTokenClobberedBySyncKey,
  SYNC_KEY_PREFIX,
} from '../api-token'

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

describe('isApiTokenClobberedBySyncKey', () => {
  it('flags a non-API-shaped token identical to the sync key', () => {
    expect(isApiTokenClobberedBySyncKey('sync-abc', 'sync-abc')).toBe(true)
    expect(isApiTokenClobberedBySyncKey(' sync-abc ', 'sync-abc')).toBe(true)
  })

  it('keeps API-shaped tokens even when reused as the sync key', () => {
    expect(isApiTokenClobberedBySyncKey('sk-abc', 'sk-abc')).toBe(false)
    expect(isApiTokenClobberedBySyncKey('laisky-abc', 'laisky-abc')).toBe(false)
  })

  it('flags any token carrying the sync key prefix, even an older sync key', () => {
    expect(isApiTokenClobberedBySyncKey('sync-old', 'sync-current')).toBe(true)
    expect(isApiTokenClobberedBySyncKey('sync-abc', undefined)).toBe(true)
    expect(isApiTokenClobberedBySyncKey(createSyncKey(), 'custom-key')).toBe(
      true,
    )
  })

  it('ignores differing or empty values', () => {
    expect(isApiTokenClobberedBySyncKey('sk-abc', 'sync-abc')).toBe(false)
    expect(isApiTokenClobberedBySyncKey('paid-token', 'other-key')).toBe(false)
    expect(isApiTokenClobberedBySyncKey('', '')).toBe(false)
    expect(isApiTokenClobberedBySyncKey(undefined, 'sync-abc')).toBe(false)
  })
})

describe('createSyncKey', () => {
  it('creates distinct prefixed keys', () => {
    const a = createSyncKey()
    expect(a.startsWith(SYNC_KEY_PREFIX)).toBe(true)
    expect(a.length).toBeGreaterThan(SYNC_KEY_PREFIX.length + 10)
    expect(createSyncKey()).not.toBe(a)
  })
})

describe('hasUsableApiToken', () => {
  it.each([
    ['sk-abc', true],
    ['laisky-abc', true],
    ['paid-token', true],
    ['FREETIER-abc', false],
    ['DEFAULT_PROXY_TOKEN', false],
    ['', false],
    ['sync-abc', false],
  ])('treats %s as usable=%s', (token, usable) => {
    expect(hasUsableApiToken(token, 'custom-sync')).toBe(usable)
  })

  it('rejects a custom token that copies the sync key', () => {
    expect(hasUsableApiToken('my-secret', 'my-secret')).toBe(false)
  })
})

describe('findRecoverableApiToken', () => {
  it('picks the most recently updated usable token', () => {
    expect(
      findRecoverableApiToken(
        [
          { api_token: 'sk-old', updated_at: 1 },
          { api_token: 'paid-token', updated_at: 3 },
          { api_token: 'sk-mid', updated_at: 2 },
        ],
        'sync-abc',
      ),
    ).toBe('paid-token')
  })

  it('skips clobbered, free-tier, legacy, and missing tokens', () => {
    expect(
      findRecoverableApiToken(
        [
          { api_token: 'sync-abc', updated_at: 9 },
          { api_token: 'FREETIER-x', updated_at: 8 },
          { api_token: 'DEFAULT_PROXY_TOKEN', updated_at: 7 },
          { api_token: '', updated_at: 6 },
          null,
          undefined,
        ],
        'sync-abc',
      ),
    ).toBe('')
  })

  it('treats configs without updated_at as oldest', () => {
    expect(
      findRecoverableApiToken(
        [{ api_token: 'sk-undated' }, { api_token: 'sk-dated', updated_at: 1 }],
        'sync-abc',
      ),
    ).toBe('sk-dated')
  })
})
