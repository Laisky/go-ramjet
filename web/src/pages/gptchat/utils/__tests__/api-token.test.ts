import { describe, expect, it, vi } from 'vitest'

import {
  createFreeTierToken,
  createSyncKey,
  describeApiTokenKind,
  FREETIER_TOKEN_PREFIX,
  isGeneratedSyncKey,
  isSameApiEndpoint,
  maskSecretHint,
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
    // Pre-CSPRNG SPA keys: two Math.random base-36 runs.
    ['sync-4fzyo82mvl9k3j5tq0x7', true],
    ['sync-short', false],
    // Not a shape GPTChat ever generated: mixed case at a non-legacy length,
    // or longer than any SPA key.
    ['sync-ProviderToken12345', false],
    [`sync-${'a'.repeat(33)}`, false],
    [`sync-${'Ab3'.repeat(21)}`, false],
    ['sync-team/alpha_01.key', false],
    ['sync-previous-key', false],
    ['sk-k2x9q4w7p1z8m3', false],
    ['', false],
  ])('treats %s as generated=%s', (value, generated) => {
    expect(isGeneratedSyncKey(value)).toBe(generated)
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

describe('maskSecretHint', () => {
  it.each([
    ['sk-abcdefghijklmnop', 'sk-…mnop'],
    ['laisky-abcdefghijkl', 'laisky-…ijkl'],
    [SYNC_KEY, 'sync-…z8m3'],
    ['FREETIER-0123456789abcdef', 'FREETIER-…cdef'],
    ['custom-token-12345', '…2345'],
    ['short-key', '••••'],
    ['', ''],
    [undefined, ''],
  ])('masks %s as %s', (secret, hint) => {
    expect(maskSecretHint(secret)).toBe(hint)
  })

  it('never contains more than the last four characters of the secret body', () => {
    const secret = 'sk-' + 'x'.repeat(20) + 'TAIL'
    const hint = maskSecretHint(secret)
    expect(hint).not.toContain('xxxx')
    expect(hint.endsWith('TAIL')).toBe(true)
  })
})
