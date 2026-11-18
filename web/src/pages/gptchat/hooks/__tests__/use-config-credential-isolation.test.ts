import { act } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import {
  installStore,
  PROVIDER_A,
  PROVIDER_B,
  renderLoadedConfig,
  sessionConfig,
  storedConfig,
  SYNC_KEY,
} from '../../__tests__/fixtures/credential-store'
import { importAllData } from '../../utils/data-sync'
import {
  listApiTokenChoices,
  readAdoptableApiToken,
} from '../session-credentials'

vi.mock(
  '@/utils/storage',
  () => import('../../__tests__/fixtures/storage-mock'),
)

vi.mock('@/pages/gptchat/utils/migration', () => ({
  migrateLegacyData: vi.fn().mockResolvedValue(undefined),
}))

vi.mock('@/utils/api', () => ({
  getApiBase: () => 'http://localhost:24456',
}))

describe('useConfig credential isolation during API token recovery', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    window.history.replaceState({}, '', '/')
  })

  it('never recovers a token that belongs to a different API endpoint', async () => {
    const store: Record<string, unknown> = {
      config_selected_session: 1,
      config_sync_key: SYNC_KEY,
      chat_user_config_1: sessionConfig({
        api_base: PROVIDER_A,
        api_token: SYNC_KEY,
      }),
      chat_user_config_2: sessionConfig({
        api_base: PROVIDER_B,
        api_token: 'sk-provider-b',
        updated_at: 999,
      }),
    }
    installStore(store)

    const { result } = await renderLoadedConfig()

    expect(result.current.config.api_base).toBe(PROVIDER_A)
    expect(result.current.config.api_token).not.toBe('sk-provider-b')
    expect(result.current.config.api_token).toMatch(/^FREETIER-/)
    expect(result.current.config.api_token_reset_required).toBe(true)
    expect(storedConfig(store, 1).api_token).not.toBe('sk-provider-b')
  })

  it('offers, but never applies, the keys of siblings on the same endpoint', async () => {
    const store: Record<string, unknown> = {
      config_selected_session: 1,
      config_sync_key: SYNC_KEY,
      chat_user_config_1: sessionConfig({
        api_base: PROVIDER_A,
        api_token: SYNC_KEY,
      }),
      chat_user_config_2: sessionConfig({
        api_base: PROVIDER_B,
        api_token: 'sk-provider-b',
        updated_at: 999,
      }),
      chat_user_config_3: sessionConfig({
        api_base: `${PROVIDER_A.toUpperCase()}/`,
        api_token: 'sk-provider-a',
        updated_at: 1,
      }),
    }
    installStore(store)

    const { result } = await renderLoadedConfig()

    expect(result.current.config.api_token).toMatch(/^FREETIER-/)
    expect(result.current.config.api_token_reset_required).toBe(true)

    const target = { apiBase: PROVIDER_A, syncKey: SYNC_KEY }
    const choices = await listApiTokenChoices(1, target)
    expect(choices).toEqual([
      {
        sourceSessionId: 3,
        hint: 'sk-…er-a',
        sessionNames: ['Chat Session 1'],
      },
    ])
    expect(JSON.stringify(choices)).not.toContain('sk-provider-a')
    expect(await readAdoptableApiToken(3, target)).toBe('sk-provider-a')
    expect(await readAdoptableApiToken(2, target)).toBe('')
  })

  it('asks the user instead of guessing between distinct same-endpoint tokens', async () => {
    const store: Record<string, unknown> = {
      config_selected_session: 1,
      config_sync_key: SYNC_KEY,
      chat_user_config_1: sessionConfig({
        api_base: PROVIDER_A,
        api_token: SYNC_KEY,
      }),
      chat_user_config_2: sessionConfig({
        api_base: PROVIDER_A,
        api_token: 'sk-account-one',
        updated_at: 100,
      }),
      chat_user_config_3: sessionConfig({
        api_base: PROVIDER_A,
        api_token: 'sk-account-two',
        updated_at: 200,
      }),
    }
    installStore(store)

    const { result } = await renderLoadedConfig()

    expect(result.current.config.api_token).toMatch(/^FREETIER-/)
    expect(result.current.config.api_token_reset_required).toBe(true)
  })

  it('offers several sessions sharing one key as a single choice', async () => {
    const store: Record<string, unknown> = {
      config_selected_session: 1,
      config_sync_key: SYNC_KEY,
      chat_user_config_1: sessionConfig({ api_token: SYNC_KEY }),
      chat_user_config_2: sessionConfig({ api_token: 'sk-shared' }),
      chat_user_config_3: sessionConfig({ api_token: ' sk-shared ' }),
    }
    installStore(store)

    const { result } = await renderLoadedConfig()

    expect(result.current.config.api_token).toMatch(/^FREETIER-/)
    const choices = await listApiTokenChoices(1, {
      apiBase: result.current.config.api_base,
      syncKey: SYNC_KEY,
    })
    expect(choices).toHaveLength(1)
    expect(choices[0].sourceSessionId).toBe(2)
    expect(choices[0].sessionNames).toHaveLength(2)
  })

  it('does not hand the previous session token to a clobbered session on another endpoint', async () => {
    const store: Record<string, unknown> = {
      config_selected_session: 1,
      config_sync_key: SYNC_KEY,
      chat_user_config_1: sessionConfig({
        api_base: PROVIDER_A,
        api_token: 'sk-provider-a',
      }),
      chat_user_config_2: sessionConfig({
        api_base: PROVIDER_B,
        api_token: SYNC_KEY,
      }),
    }
    installStore(store)

    const { result } = await renderLoadedConfig()
    expect(result.current.config.api_token).toBe('sk-provider-a')

    await act(async () => {
      await result.current.switchSession(2)
    })

    expect(result.current.config.api_base).toBe(PROVIDER_B)
    expect(result.current.config.api_token).not.toBe('sk-provider-a')
    expect(result.current.config.api_token).toMatch(/^FREETIER-/)
    expect(result.current.config.api_token_reset_required).toBe(true)
  })

  it('does not copy the previous session token into a tokenless session on another endpoint', async () => {
    const store: Record<string, unknown> = {
      config_selected_session: 1,
      config_sync_key: SYNC_KEY,
      chat_user_config_1: sessionConfig({
        api_base: PROVIDER_A,
        api_token: 'sk-provider-a',
      }),
      chat_user_config_2: sessionConfig({
        api_base: PROVIDER_B,
        api_token: '',
      }),
    }
    installStore(store)

    const { result } = await renderLoadedConfig()
    await act(async () => {
      await result.current.switchSession(2)
    })

    expect(result.current.config.api_token).not.toBe('sk-provider-a')
    expect(result.current.config.api_token).toMatch(/^FREETIER-/)
  })

  it('still carries the token into a brand-new session on the same endpoint', async () => {
    const store: Record<string, unknown> = {
      config_selected_session: 1,
      config_sync_key: SYNC_KEY,
      chat_user_config_1: sessionConfig({
        api_base: PROVIDER_A,
        api_token: 'sk-provider-a',
      }),
    }
    installStore(store)

    const { result } = await renderLoadedConfig()
    await act(async () => {
      await result.current.switchSession(7)
    })

    expect(result.current.config.api_base).toBe(PROVIDER_A)
    expect(result.current.config.api_token).toBe('sk-provider-a')
  })

  it('keeps the session’s own key when a download carries a clobbered copy', async () => {
    const store: Record<string, unknown> = {
      config_selected_session: 1,
      config_sync_key: SYNC_KEY,
      chat_user_config_1: sessionConfig({ api_token: 'sk-local' }),
    }
    installStore(store)

    await importAllData(
      {
        config_sync_key: SYNC_KEY,
        chat_user_config_1: sessionConfig({
          api_token: SYNC_KEY,
          updated_at: 10,
        }),
        chat_user_config_2: sessionConfig({
          api_token: 'sk-from-cloud',
          updated_at: 5,
        }),
      },
      1,
      'download',
    )
    // The local copy of the same session is the only key with known
    // provenance; session 2's key may belong to another account.
    expect(storedConfig(store, 1).api_token).toBe('sk-local')

    const { result } = await renderLoadedConfig()

    expect(result.current.config.api_token).toBe('sk-local')
    expect(result.current.config.api_token_reset_required).toBeFalsy()
  })

  it('flags a downloaded clobbered session that has no local copy', async () => {
    const store = installStore({
      config_selected_session: 1,
      config_sync_key: SYNC_KEY,
      chat_user_config_1: sessionConfig({ api_token: 'sk-local' }),
    })

    await importAllData(
      {
        config_sync_key: SYNC_KEY,
        chat_user_config_2: sessionConfig({ api_token: SYNC_KEY }),
      },
      1,
      'download',
    )

    expect(storedConfig(store, 2).api_token).toMatch(/^FREETIER-/)
    expect(storedConfig(store, 2).api_token_reset_required).toBe(true)
  })
})

describe('useConfig URL overrides honor the API token invariant', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    window.history.replaceState({}, '', '/')
  })

  it('ignores an api_key URL parameter that carries the sync key', async () => {
    window.history.replaceState({}, '', `/?api_key=${SYNC_KEY}`)
    const store: Record<string, unknown> = {
      config_selected_session: 1,
      config_sync_key: SYNC_KEY,
      chat_user_config_1: sessionConfig({ api_token: 'sk-original' }),
    }
    installStore(store)

    const { result } = await renderLoadedConfig()

    expect(result.current.config.api_token).toBe('sk-original')
    expect(storedConfig(store, 1).api_token).toBe('sk-original')
  })

  it('accepts a real API key from the URL and clears a pending reset', async () => {
    window.history.replaceState({}, '', '/?api_key=sk-from-url')
    const store: Record<string, unknown> = {
      config_selected_session: 1,
      config_sync_key: SYNC_KEY,
      chat_user_config_1: sessionConfig({
        api_token: 'FREETIER-placeholder',
        api_token_reset_required: true,
      }),
    }
    installStore(store)

    const { result } = await renderLoadedConfig()

    expect(result.current.config.api_token).toBe('sk-from-url')
    expect(result.current.config.api_token_reset_required).toBe(false)
  })

  it('scopes key choices to the endpoint chosen by the URL', async () => {
    window.history.replaceState({}, '', `/?api_base=${PROVIDER_B}`)
    const store: Record<string, unknown> = {
      config_selected_session: 1,
      config_sync_key: SYNC_KEY,
      chat_user_config_1: sessionConfig({
        api_base: PROVIDER_A,
        api_token: SYNC_KEY,
      }),
      chat_user_config_2: sessionConfig({
        api_base: PROVIDER_A,
        api_token: 'sk-provider-a',
      }),
      chat_user_config_3: sessionConfig({
        api_base: PROVIDER_B,
        api_token: 'sk-provider-b',
      }),
    }
    installStore(store)

    const { result } = await renderLoadedConfig()

    expect(result.current.config.api_base).toBe(PROVIDER_B)
    expect(result.current.config.api_token).toMatch(/^FREETIER-/)
    const choices = await listApiTokenChoices(1, {
      apiBase: result.current.config.api_base,
      syncKey: SYNC_KEY,
    })
    expect(choices.map((c) => c.sourceSessionId)).toEqual([3])
  })
})

describe('useConfig API token reset state machine', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('keeps the reset pending when a write stores no key of the user’s own', async () => {
    const store: Record<string, unknown> = {
      config_selected_session: 1,
      config_sync_key: SYNC_KEY,
      chat_user_config_1: sessionConfig({
        api_token: 'FREETIER-placeholder',
        api_token_reset_required: true,
      }),
    }
    installStore(store)

    const first = await renderLoadedConfig()
    // "Reset settings" writes an empty token; the next load fills a
    // free-tier one. Neither is the user's own key.
    await act(async () => {
      await first.result.current.updateConfig({ api_token: '' })
    })

    expect(first.result.current.config.api_token_reset_required).toBe(true)
    first.unmount()

    const { result } = await renderLoadedConfig()
    expect(result.current.config.api_token).toMatch(/^FREETIER-/)
    expect(result.current.config.api_token_reset_required).toBe(true)
  })

  it('never replaces a stored key with a sibling token', async () => {
    const store: Record<string, unknown> = {
      config_selected_session: 1,
      config_sync_key: SYNC_KEY,
      chat_user_config_1: sessionConfig({
        api_token: 'sk-half-typed',
        api_token_reset_required: true,
      }),
      chat_user_config_2: sessionConfig({ api_token: 'sk-sibling' }),
    }
    installStore(store)

    const { result } = await renderLoadedConfig()

    expect(result.current.config.api_token).toBe('sk-half-typed')
    expect(result.current.config.api_token_reset_required).toBe(true)
  })
})
