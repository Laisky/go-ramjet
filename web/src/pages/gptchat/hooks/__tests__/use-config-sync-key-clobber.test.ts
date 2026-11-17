import { kvGet, kvList, kvSet } from '@/utils/storage'
import { act, renderHook, waitFor } from '@testing-library/react'
import { type Mock, beforeEach, describe, expect, it, vi } from 'vitest'

import { DefaultSessionConfig, type SessionConfig } from '../../types'
import { useConfig } from '../use-config'

vi.mock('@/utils/storage', () => ({
  kvGet: vi.fn(),
  kvSet: vi.fn(),
  kvDel: vi.fn(),
  kvList: vi.fn(),
  StorageKeys: {
    SESSION_CONFIG_PREFIX: 'chat_user_config_',
    SELECTED_SESSION: 'config_selected_session',
    VERSION_DATE: 'config_version_date',
    SESSION_HISTORY_PREFIX: 'chat_user_session_',
    SESSION_ORDER: 'config_session_order',
    SYNC_KEY: 'config_sync_key',
  },
}))

vi.mock('@/pages/gptchat/utils/migration', () => ({
  migrateLegacyData: vi.fn().mockResolvedValue(undefined),
}))

vi.mock('@/utils/api', () => ({
  getApiBase: () => 'http://localhost:24456',
}))

const SYNC_KEY = 'sync-k2x9q4w7p1z8m3'

/**
 * installStore backs the mocked KV layer with an in-memory record so tests can
 * observe exactly what the hook reads and persists.
 */
function installStore(store: Record<string, unknown>): void {
  ;(kvGet as Mock).mockImplementation((key: string) =>
    Promise.resolve(store[key] ?? null),
  )
  ;(kvSet as Mock).mockImplementation(async (key: string, value: unknown) => {
    store[key] = value
  })
  ;(kvList as Mock).mockImplementation(async () => Object.keys(store))
}

/**
 * sessionConfig builds a stored session config with the given overrides.
 */
function sessionConfig(overrides: Partial<SessionConfig>): SessionConfig {
  return {
    ...DefaultSessionConfig,
    sync_key: SYNC_KEY,
    ...overrides,
  }
}

/**
 * renderLoadedConfig mounts useConfig and waits until hydration has finished.
 */
async function renderLoadedConfig() {
  const hook = renderHook(() => useConfig())
  await waitFor(() => {
    expect(hook.result.current.isLoading).toBe(false)
  })
  return hook
}

describe('useConfig API token clobbered by sync key', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('restores the token from a sibling session and persists the repair', async () => {
    const store: Record<string, unknown> = {
      config_selected_session: 1,
      config_sync_key: SYNC_KEY,
      chat_user_config_1: sessionConfig({ api_token: SYNC_KEY }),
      chat_user_config_2: sessionConfig({
        api_token: 'sk-older',
        updated_at: 100,
      }),
      chat_user_config_3: sessionConfig({
        api_token: 'sk-newest',
        updated_at: 200,
      }),
    }
    installStore(store)

    const { result } = await renderLoadedConfig()

    expect(result.current.config.api_token).toBe('sk-newest')
    expect((store.chat_user_config_1 as SessionConfig).api_token).toBe(
      'sk-newest',
    )
    expect(result.current.config.sync_key).toBe(SYNC_KEY)
    expect(result.current.config.api_token_reset_required).toBeFalsy()
  })

  it('detects a token holding an older sync key by its prefix', async () => {
    const store: Record<string, unknown> = {
      config_selected_session: 1,
      config_sync_key: SYNC_KEY,
      chat_user_config_1: sessionConfig({ api_token: 'sync-previous-key' }),
      chat_user_config_2: sessionConfig({ api_token: 'sk-sibling' }),
    }
    installStore(store)

    const { result } = await renderLoadedConfig()

    expect(result.current.config.api_token).toBe('sk-sibling')
    expect(result.current.config.api_token_reset_required).toBeFalsy()
  })

  it('falls back to a free-tier token when no sibling holds a usable token', async () => {
    const store: Record<string, unknown> = {
      config_selected_session: 1,
      config_sync_key: SYNC_KEY,
      chat_user_config_1: sessionConfig({ api_token: SYNC_KEY }),
      chat_user_config_2: sessionConfig({ api_token: SYNC_KEY }),
    }
    installStore(store)

    const { result } = await renderLoadedConfig()

    expect(result.current.config.api_token).toMatch(/^FREETIER-/)
    expect(result.current.config.api_token_reset_required).toBe(true)
    const persisted = store.chat_user_config_1 as SessionConfig
    expect(persisted.api_token).not.toBe(SYNC_KEY)
    expect(persisted.api_token_reset_required).toBe(true)
  })

  it('keeps alerting across reloads until a usable token exists', async () => {
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

    expect(result.current.config.api_token).toBe('FREETIER-placeholder')
    expect(result.current.config.api_token_reset_required).toBe(true)
  })

  it('retries recovery for a flagged session once a sibling has a token', async () => {
    const store: Record<string, unknown> = {
      config_selected_session: 1,
      config_sync_key: SYNC_KEY,
      chat_user_config_1: sessionConfig({
        api_token: 'FREETIER-placeholder',
        api_token_reset_required: true,
      }),
      chat_user_config_2: sessionConfig({ api_token: 'sk-entered-later' }),
    }
    installStore(store)

    const { result } = await renderLoadedConfig()

    expect(result.current.config.api_token).toBe('sk-entered-later')
    expect(result.current.config.api_token_reset_required).toBe(false)
    expect(
      (store.chat_user_config_1 as SessionConfig).api_token_reset_required,
    ).toBe(false)
  })

  it('clears the alert when the user enters a new API key', async () => {
    const store: Record<string, unknown> = {
      config_selected_session: 1,
      config_sync_key: SYNC_KEY,
      chat_user_config_1: sessionConfig({ api_token: SYNC_KEY }),
    }
    installStore(store)

    const { result } = await renderLoadedConfig()
    expect(result.current.config.api_token_reset_required).toBe(true)

    await act(async () => {
      await result.current.updateConfig({ api_token: 'sk-re-entered' })
    })

    expect(result.current.config.api_token).toBe('sk-re-entered')
    expect(result.current.config.api_token_reset_required).toBe(false)
    expect(
      (store.chat_user_config_1 as SessionConfig).api_token_reset_required,
    ).toBe(false)
  })

  it('stops alerting when the user chooses to keep the free tier', async () => {
    const store: Record<string, unknown> = {
      config_selected_session: 1,
      config_sync_key: SYNC_KEY,
      chat_user_config_1: sessionConfig({ api_token: SYNC_KEY }),
      chat_user_config_2: sessionConfig({ api_token: SYNC_KEY }),
    }
    installStore(store)

    const first = await renderLoadedConfig()
    await act(async () => {
      await first.result.current.updateConfig({
        api_token_reset_required: false,
      })
    })
    first.unmount()

    const { result } = await renderLoadedConfig()
    expect(result.current.config.api_token).toMatch(/^FREETIER-/)
    expect(result.current.config.api_token_reset_required).toBe(false)
  })

  it('leaves configs written before the flag existed unchanged', async () => {
    const store: Record<string, unknown> = {
      config_selected_session: 1,
      config_sync_key: SYNC_KEY,
      chat_user_config_1: sessionConfig({
        api_token: 'sk-legacy',
        selected_chat_model: DefaultSessionConfig.selected_chat_model,
        selected_draw_model: DefaultSessionConfig.selected_draw_model,
        session_name: 'Chat Session 1',
        updated_at: 42,
      }),
    }
    installStore(store)

    const { result } = await renderLoadedConfig()

    expect(result.current.config.api_token).toBe('sk-legacy')
    expect(result.current.config.api_token_reset_required).toBeUndefined()
    expect(
      (store.chat_user_config_1 as SessionConfig).api_token_reset_required,
    ).toBeUndefined()
  })

  it('keeps an API-shaped token the user deliberately reused as the sync key', async () => {
    const sharedSecret = 'sk-shared-secret'
    const store: Record<string, unknown> = {
      config_selected_session: 1,
      config_sync_key: sharedSecret,
      chat_user_config_1: sessionConfig({
        api_token: sharedSecret,
        sync_key: sharedSecret,
      }),
    }
    installStore(store)

    const { result } = await renderLoadedConfig()

    expect(result.current.config.api_token).toBe(sharedSecret)
  })

  it('leaves a healthy custom token untouched', async () => {
    const store: Record<string, unknown> = {
      config_selected_session: 1,
      config_sync_key: SYNC_KEY,
      chat_user_config_1: sessionConfig({ api_token: 'paid-token-12345' }),
    }
    installStore(store)

    const { result } = await renderLoadedConfig()

    expect(result.current.config.api_token).toBe('paid-token-12345')
  })

  it('rejects an API token update that copies the sync key', async () => {
    const store: Record<string, unknown> = {
      config_selected_session: 1,
      config_sync_key: SYNC_KEY,
      chat_user_config_1: sessionConfig({ api_token: 'sk-original' }),
    }
    installStore(store)

    const { result } = await renderLoadedConfig()

    await act(async () => {
      await result.current.updateConfig({ api_token: SYNC_KEY })
    })

    expect(result.current.config.api_token).toBe('sk-original')
    expect((store.chat_user_config_1 as SessionConfig).api_token).toBe(
      'sk-original',
    )
  })

  it('rejects an API token update carrying the sync key prefix', async () => {
    const store: Record<string, unknown> = {
      config_selected_session: 1,
      config_sync_key: SYNC_KEY,
      chat_user_config_1: sessionConfig({ api_token: 'sk-original' }),
    }
    installStore(store)

    const { result } = await renderLoadedConfig()

    await act(async () => {
      await result.current.updateConfig({ api_token: 'sync-older-key' })
    })

    expect(result.current.config.api_token).toBe('sk-original')
  })

  it('still accepts ordinary API token edits', async () => {
    const store: Record<string, unknown> = {
      config_selected_session: 1,
      config_sync_key: SYNC_KEY,
      chat_user_config_1: sessionConfig({ api_token: 'sk-original' }),
    }
    installStore(store)

    const { result } = await renderLoadedConfig()

    await act(async () => {
      await result.current.updateConfig({ api_token: 'sk-replacement' })
    })

    expect(result.current.config.api_token).toBe('sk-replacement')
    expect((store.chat_user_config_1 as SessionConfig).api_token).toBe(
      'sk-replacement',
    )
  })
})
