import { describe, expect, it } from 'vitest'

import { DefaultSessionConfig, type SessionConfig } from '../../types'
import { createSyncKey } from '../api-token'
import {
  findApiTokenCandidates,
  isApiTokenClobberedBySyncKey,
  isSyncKeyReusedAsApiToken,
  isUserSuppliedApiToken,
  repairStoredApiToken,
  restoringApiTokenNeedsRotation,
  validateCredentialUpdate,
} from '../credential-policy'

const SYNC_KEY = 'sync-k2x9q4w7p1z8m3'
const LEGACY_PAGE_SYNC_KEY = `sync-${'Ab3'.repeat(21)}x`
const PROVIDER_A = 'https://provider-a.example'
const PROVIDER_B = 'https://provider-b.example'

/**
 * config builds a session config sharing SYNC_KEY with overrides applied.
 */
function config(overrides: Partial<SessionConfig> = {}): SessionConfig {
  return { ...DefaultSessionConfig, sync_key: SYNC_KEY, ...overrides }
}

describe('isApiTokenClobberedBySyncKey', () => {
  it('flags a non-API-shaped token identical to the sync key', () => {
    expect(isApiTokenClobberedBySyncKey('my-sync', 'my-sync')).toBe(true)
    expect(isApiTokenClobberedBySyncKey(' my-sync ', 'my-sync')).toBe(true)
  })

  it('leaves an API-shaped token reused as the sync key to rotation', () => {
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

  it('keeps a provider token that merely starts with sync-', () => {
    expect(isApiTokenClobberedBySyncKey('sync-team/alpha_01.key', 'x')).toBe(
      false,
    )
    expect(isApiTokenClobberedBySyncKey('sync-ProviderToken12345', 'x')).toBe(
      false,
    )
  })

  it('ignores differing or empty values', () => {
    expect(isApiTokenClobberedBySyncKey('sk-abc', SYNC_KEY)).toBe(false)
    expect(isApiTokenClobberedBySyncKey('paid-token', 'paid-tokem')).toBe(false)
    expect(isApiTokenClobberedBySyncKey('', '')).toBe(false)
    expect(isApiTokenClobberedBySyncKey(undefined, SYNC_KEY)).toBe(false)
  })
})

describe('isSyncKeyReusedAsApiToken', () => {
  it('detects an API-shaped token that doubles as the sync key', () => {
    expect(isSyncKeyReusedAsApiToken('sk-abc', 'sk-abc')).toBe(true)
    expect(isSyncKeyReusedAsApiToken('FREETIER-abc', 'FREETIER-abc')).toBe(true)
  })

  it('does not count a clobbered token or distinct values', () => {
    expect(isSyncKeyReusedAsApiToken('my-sync', 'my-sync')).toBe(false)
    expect(isSyncKeyReusedAsApiToken(SYNC_KEY, SYNC_KEY)).toBe(false)
    expect(isSyncKeyReusedAsApiToken('sk-abc', SYNC_KEY)).toBe(false)
    expect(isSyncKeyReusedAsApiToken('', '')).toBe(false)
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

describe('validateCredentialUpdate', () => {
  it('drops an API token that copies the sync key, whatever its shape', () => {
    for (const syncKey of [SYNC_KEY, 'my-sync', 'sk-reused']) {
      const { accepted, rejections } = validateCredentialUpdate(
        config({ api_token: 'sk-current', sync_key: syncKey }),
        { api_token: syncKey, temperature: 0.3 },
      )
      expect(accepted).toEqual({ temperature: 0.3 })
      expect(rejections).toEqual([
        { field: 'api_token', reason: 'api-token-is-sync-key' },
      ])
    }
  })

  it('drops an API token holding an older generated sync key', () => {
    const { accepted, rejections } = validateCredentialUpdate(
      config({ api_token: 'sk-current' }),
      { api_token: 'sync-0123456789abcdef' },
    )
    expect(accepted.api_token).toBeUndefined()
    expect(rejections).toHaveLength(1)
  })

  it('drops a sync key that copies the API token, whatever its shape', () => {
    for (const apiToken of ['sk-live', 'laisky-live', 'FREETIER-live', 'own']) {
      const { accepted, rejections } = validateCredentialUpdate(
        config({ api_token: apiToken }),
        { sync_key: apiToken },
      )
      expect(accepted.sync_key).toBeUndefined()
      expect(rejections).toEqual([
        { field: 'sync_key', reason: 'sync-key-is-api-token' },
      ])
    }
  })

  it('checks a sync key against the API token written in the same update', () => {
    const { accepted } = validateCredentialUpdate(config(), {
      api_token: 'sk-new',
      sync_key: 'sk-new',
    })
    expect(accepted).toEqual({ api_token: 'sk-new' })
  })

  it('accepts unchanged values of a legacy pair that already match', () => {
    const legacy = config({ api_token: 'sk-reused', sync_key: 'sk-reused' })
    const { accepted, rejections } = validateCredentialUpdate(legacy, {
      api_token: 'sk-reused',
      sync_key: 'sk-reused',
    })
    expect(rejections).toEqual([])
    expect(accepted).toEqual({ api_token: 'sk-reused', sync_key: 'sk-reused' })
  })

  it('accepts ordinary API token and sync key changes', () => {
    const { accepted, rejections } = validateCredentialUpdate(config(), {
      api_token: 'sk-new',
      sync_key: 'sync-0000aaaa1111bbbb',
    })
    expect(rejections).toEqual([])
    expect(accepted.api_token).toBe('sk-new')
    expect(accepted.sync_key).toBe('sync-0000aaaa1111bbbb')
  })

  it('completes a pending reset when the user saves a key of their own', () => {
    const pending = config({
      api_token: 'FREETIER-x',
      api_token_reset_required: true,
    })
    expect(
      validateCredentialUpdate(pending, { api_token: 'sk-new' }).accepted,
    ).toEqual({ api_token: 'sk-new', api_token_reset_required: false })
    expect(
      validateCredentialUpdate(pending, { api_token: 'FREETIER-y' }).accepted,
    ).toEqual({ api_token: 'FREETIER-y' })
    expect(
      validateCredentialUpdate(pending, { api_token: SYNC_KEY }).accepted,
    ).toEqual({})
  })

  it('leaves the reset flag alone without a pending reset', () => {
    const { accepted } = validateCredentialUpdate(config(), {
      api_token: 'sk-new',
    })
    expect(accepted).toEqual({ api_token: 'sk-new' })
  })
})

describe('repairStoredApiToken', () => {
  it('returns a healthy config unchanged', () => {
    const healthy = config({ api_token: 'sk-ok' })
    const result = repairStoredApiToken(healthy, [SYNC_KEY])
    expect(result.outcome).toBe('healthy')
    expect(result.config).toBe(healthy)
  })

  it('keeps a legacy API-shaped token that doubles as the sync key', () => {
    const legacy = config({ api_token: 'sk-reused', sync_key: 'sk-reused' })
    expect(repairStoredApiToken(legacy, ['sk-reused']).outcome).toBe('healthy')
  })

  it('falls back to the free tier and flags a reset', () => {
    const result = repairStoredApiToken(config({ api_token: SYNC_KEY }), [
      SYNC_KEY,
    ])
    expect(result.outcome).toBe('reset-required')
    expect(result.config.api_token).toMatch(/^FREETIER-/)
    expect(result.config.api_token_reset_required).toBe(true)
  })

  it('checks the token against every sync key in play', () => {
    const result = repairStoredApiToken(config({ api_token: 'local-sync' }), [
      'cloud-sync',
      'local-sync',
    ])
    expect(result.outcome).toBe('reset-required')
  })

  it('restores the same session’s earlier key on the same endpoint', () => {
    const result = repairStoredApiToken(
      config({ api_base: PROVIDER_A, api_token: SYNC_KEY }),
      [SYNC_KEY],
      {
        previous: config({ api_base: `${PROVIDER_A}/`, api_token: 'sk-mine' }),
      },
    )
    expect(result.outcome).toBe('restored-previous')
    expect(result.config.api_token).toBe('sk-mine')
    expect(result.config.api_token_reset_required).toBe(false)
  })

  it('does not restore an earlier key from another endpoint or a bad one', () => {
    const clobbered = config({ api_base: PROVIDER_A, api_token: SYNC_KEY })
    for (const previous of [
      config({ api_base: PROVIDER_B, api_token: 'sk-mine' }),
      config({ api_base: PROVIDER_A, api_token: 'FREETIER-x' }),
      config({ api_base: PROVIDER_A, api_token: 'sync-0123456789abcd' }),
      null,
    ]) {
      expect(
        repairStoredApiToken(clobbered, [SYNC_KEY], { previous }).outcome,
      ).toBe('reset-required')
    }
  })
})

describe('findApiTokenCandidates', () => {
  const target = { apiBase: PROVIDER_A, syncKey: SYNC_KEY }

  it('groups sessions sharing a key and masks it', () => {
    const candidates = findApiTokenCandidates(
      [
        {
          sessionId: 2,
          config: config({
            api_base: PROVIDER_A,
            api_token: 'sk-account-one',
            session_name: 'Work',
          }),
        },
        {
          sessionId: 3,
          config: config({
            api_base: `${PROVIDER_A}/`,
            api_token: ' sk-account-one ',
          }),
        },
        {
          sessionId: 4,
          config: config({ api_base: PROVIDER_A, api_token: 'sk-account-two' }),
        },
      ],
      target,
    )
    expect(candidates).toEqual([
      {
        token: 'sk-account-one',
        hint: 'sk-…-one',
        sessions: [
          { id: 2, name: 'Work' },
          { id: 3, name: 'Chat Session 1' },
        ],
      },
      {
        token: 'sk-account-two',
        hint: 'sk-…-two',
        sessions: [{ id: 4, name: 'Chat Session 1' }],
      },
    ])
  })

  it('skips other endpoints and tokens that are not the user’s own', () => {
    const candidates = findApiTokenCandidates(
      [
        {
          sessionId: 2,
          config: config({ api_base: PROVIDER_B, api_token: 'sk-provider-b' }),
        },
        {
          sessionId: 3,
          config: config({ api_base: PROVIDER_A, api_token: SYNC_KEY }),
        },
        {
          sessionId: 4,
          config: config({ api_base: PROVIDER_A, api_token: 'FREETIER-x' }),
        },
        {
          sessionId: 5,
          config: config({
            api_base: PROVIDER_A,
            api_token: 'DEFAULT_PROXY_TOKEN',
          }),
        },
        { sessionId: 6, config: null },
      ],
      target,
    )
    expect(candidates).toEqual([])
  })

  it('matches sessions without an api_base against the default endpoint', () => {
    const candidates = findApiTokenCandidates(
      [{ sessionId: 2, config: { api_token: 'sk-default' } }],
      { apiBase: 'https://api.openai.com/', syncKey: SYNC_KEY },
    )
    expect(candidates.map((c) => c.token)).toEqual(['sk-default'])
  })

  it('caps the number of distinct candidates', () => {
    const entries = Array.from({ length: 8 }, (_, i) => ({
      sessionId: i + 2,
      config: config({
        api_base: PROVIDER_A,
        api_token: `sk-account-${i}-pad`,
      }),
    }))
    expect(findApiTokenCandidates(entries, target, 3)).toHaveLength(3)
  })
})

describe('ambiguous legacy pairs (custom API key equal to the sync key)', () => {
  const CUSTOM = 'my-custom-provider-token'

  it('sets the value aside instead of discarding it', () => {
    const { config: repaired, outcome } = repairStoredApiToken(
      config({ api_token: CUSTOM, sync_key: CUSTOM }),
      [CUSTOM],
    )

    expect(outcome).toBe('reset-required')
    expect(repaired.api_token).toMatch(/^FREETIER-/)
    expect(repaired.api_token_before_reset).toBe(CUSTOM)
  })

  it('never keeps a value with a generated sync-key shape', () => {
    const { config: repaired } = repairStoredApiToken(
      config({ api_token: SYNC_KEY }),
      [SYNC_KEY],
    )

    expect(repaired.api_token_reset_required).toBe(true)
    expect(repaired.api_token_before_reset).toBeUndefined()
  })

  it('needs a rotation to restore only while the value is still the sync key', () => {
    expect(
      restoringApiTokenNeedsRotation({
        api_token_before_reset: CUSTOM,
        sync_key: CUSTOM,
      }),
    ).toBe(true)
    expect(
      restoringApiTokenNeedsRotation({
        api_token_before_reset: CUSTOM,
        sync_key: SYNC_KEY,
      }),
    ).toBe(false)
    expect(restoringApiTokenNeedsRotation({ sync_key: SYNC_KEY })).toBe(false)
  })

  it('forgets the set-aside value whenever the reset ends', () => {
    const pending = config({
      api_token: 'FREETIER-x',
      api_token_reset_required: true,
      api_token_before_reset: CUSTOM,
    })

    for (const updates of [
      { api_token: 'sk-new' },
      { api_token_reset_required: false },
    ]) {
      const { accepted } = validateCredentialUpdate(pending, updates)
      expect(accepted.api_token_reset_required).toBe(false)
      expect('api_token_before_reset' in accepted).toBe(true)
      expect(accepted.api_token_before_reset).toBeUndefined()
    }
    expect(
      'api_token_before_reset' in
        validateCredentialUpdate(pending, { system_prompt: 'x' }).accepted,
    ).toBe(false)
  })
})

describe('validateCredentialUpdate across sessions', () => {
  it('refuses a sync key that is the API key of another session', () => {
    const { accepted, rejections } = validateCredentialUpdate(
      config({ api_token: 'sk-active' }),
      { sync_key: 'sk-other-session' },
      { otherApiTokens: ['FREETIER-a', 'sk-other-session'] },
    )

    expect(accepted.sync_key).toBeUndefined()
    expect(rejections).toEqual([
      { field: 'sync_key', reason: 'sync-key-is-api-token' },
    ])
  })

  it('accepts a sync key no session uses, and an unchanged one', () => {
    const others = { otherApiTokens: ['sk-other-session'] }
    expect(
      validateCredentialUpdate(
        config(),
        { sync_key: 'sync-fresh123456' },
        others,
      ).rejections,
    ).toEqual([])
    expect(
      validateCredentialUpdate(
        config({ sync_key: 'sk-other-session' }),
        { sync_key: 'sk-other-session' },
        others,
      ).rejections,
    ).toEqual([])
  })
})
