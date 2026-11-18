import { act } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import {
  installStore,
  renderLoadedConfig,
  sessionConfig,
  SYNC_KEY,
} from '../../__tests__/fixtures/credential-store'
import { DefaultSessionConfig, type SessionConfig } from '../../types'

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

describe('useConfig API token clobbered by sync key', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('never adopts a sibling key on its own, even when every sibling agrees', async () => {
    const store: Record<string, unknown> = {
      config_selected_session: 1,
      config_sync_key: SYNC_KEY,
      chat_user_config_1: sessionConfig({ api_token: SYNC_KEY }),
      chat_user_config_2: sessionConfig({
        api_token: 'sk-user-key',
        updated_at: 100,
      }),
      chat_user_config_3: sessionConfig({
        api_token: 'sk-user-key',
        updated_at: 200,
      }),
    }
    installStore(store)

    const { result } = await renderLoadedConfig()

    // The siblings may belong to another account on the same endpoint; the
    // user picks one from the alert instead.
    expect(result.current.config.api_token).toMatch(/^FREETIER-/)
    expect(result.current.config.api_token_reset_required).toBe(true)
    const persisted = store.chat_user_config_1 as SessionConfig
    expect(persisted.api_token).toMatch(/^FREETIER-/)
    expect(persisted.api_token_reset_required).toBe(true)
    expect(result.current.config.sync_key).toBe(SYNC_KEY)
  })

  it('detects a token holding an older generated sync key', async () => {
    const store: Record<string, unknown> = {
      config_selected_session: 1,
      config_sync_key: SYNC_KEY,
      chat_user_config_1: sessionConfig({ api_token: 'sync-0ld3r5ync4ey99' }),
      chat_user_config_2: sessionConfig({ api_token: 'sk-sibling' }),
    }
    installStore(store)

    const { result } = await renderLoadedConfig()

    expect(result.current.config.api_token).toMatch(/^FREETIER-/)
    expect(result.current.config.api_token_reset_required).toBe(true)
  })

  it('falls back to a free-tier token when no sibling holds a usable token either', async () => {
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

  it('keeps a flagged session on the free tier until the user acts', async () => {
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

    expect(result.current.config.api_token).toBe('FREETIER-placeholder')
    expect(result.current.config.api_token_reset_required).toBe(true)
    expect(
      (store.chat_user_config_1 as SessionConfig).api_token_reset_required,
    ).toBe(true)
  })

  it('clears the alert in the same write that saves a key of the user’s own', async () => {
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
    const persisted = store.chat_user_config_1 as SessionConfig
    expect(persisted.api_token).toBe('sk-re-entered')
    expect(persisted.api_token_reset_required).toBe(false)
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

  it('rejects an API token update carrying an older generated sync key', async () => {
    const store: Record<string, unknown> = {
      config_selected_session: 1,
      config_sync_key: SYNC_KEY,
      chat_user_config_1: sessionConfig({ api_token: 'sk-original' }),
    }
    installStore(store)

    const { result } = await renderLoadedConfig()

    await act(async () => {
      await result.current.updateConfig({ api_token: 'sync-0ld3r5ync4ey99' })
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

describe('useConfig sync key writes honor the API token invariant', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('rejects a sync key that copies a custom API token', async () => {
    const store: Record<string, unknown> = {
      config_selected_session: 1,
      config_sync_key: SYNC_KEY,
      chat_user_config_1: sessionConfig({ api_token: 'paid-token-12345' }),
    }
    installStore(store)

    const { result } = await renderLoadedConfig()

    await act(async () => {
      await result.current.updateConfig({
        sync_key: 'paid-token-12345',
        system_prompt: 'still saved',
      })
    })

    expect(result.current.config.sync_key).toBe(SYNC_KEY)
    expect(result.current.config.system_prompt).toBe('still saved')
    expect(store.config_sync_key).toBe(SYNC_KEY)
    expect((store.chat_user_config_1 as SessionConfig).api_token).toBe(
      'paid-token-12345',
    )
  })

  it('refuses an API-shaped key as the sync key', async () => {
    const store: Record<string, unknown> = {
      config_selected_session: 1,
      config_sync_key: SYNC_KEY,
      chat_user_config_1: sessionConfig({ api_token: 'sk-shared-secret' }),
    }
    installStore(store)

    const { result } = await renderLoadedConfig()

    await act(async () => {
      await result.current.updateConfig({ sync_key: 'sk-shared-secret' })
    })

    // Whoever holds the API key (its provider included) could otherwise
    // download and decrypt the cloud backup.
    expect(store.config_sync_key).toBe(SYNC_KEY)
    expect(result.current.config.sync_key).toBe(SYNC_KEY)
    expect(result.current.config.api_token).toBe('sk-shared-secret')
  })
})
