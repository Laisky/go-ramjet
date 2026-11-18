import { act } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import {
  installStore,
  kvSet,
  renderLoadedConfig,
  sessionConfig,
  storedConfig,
  SYNC_KEY,
} from '../../__tests__/fixtures/credential-store'
import { exportAllData, importAllData } from '../../utils/data-sync'

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

const OLD_SYNC_KEY = 'sync-01d01d01d01d01d0'

/**
 * deferWritesOf makes kvSet hold every write of key until the returned
 * release function is called; other keys are stored at once.
 */
function deferWritesOf(store: Record<string, unknown>, key: string) {
  let release: () => void = () => undefined
  const gate = new Promise<void>((resolve) => {
    release = resolve
  })
  kvSet.mockImplementation(async (k: string, value: unknown) => {
    if (k === key) await gate
    store[k] = value
  })
  return release
}

describe('useConfig persistence contract', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    window.history.replaceState({}, '', '/')
  })

  it('rejects updateConfig when the write fails and keeps the stored config', async () => {
    const store = installStore({
      config_selected_session: 1,
      config_sync_key: SYNC_KEY,
      chat_user_config_1: sessionConfig({ api_token: 'sk-original' }),
    })
    const { result } = await renderLoadedConfig()
    kvSet.mockImplementation(async (key: string, value: unknown) => {
      if (key === 'chat_user_config_1') throw new Error('QuotaExceededError')
      store[key] = value
    })

    let failure: unknown
    await act(async () => {
      failure = await result.current
        .updateConfig({ api_token: 'sk-replacement' })
        .then(() => undefined)
        .catch((error: unknown) => error)
    })

    expect(String(failure)).toContain('QuotaExceededError')
    expect(storedConfig(store, 1).api_token).toBe('sk-original')
    // The in-memory config does not claim a value storage never received.
    expect(result.current.config.api_token).toBe('sk-original')
  })

  it('exports only after queued config writes have landed', async () => {
    const store = installStore({
      config_selected_session: 1,
      config_sync_key: SYNC_KEY,
      chat_user_config_1: sessionConfig({ system_prompt: 'old prompt' }),
    })
    const { result } = await renderLoadedConfig()
    const release = deferWritesOf(store, 'chat_user_config_1')

    let update: Promise<unknown> = Promise.resolve()
    let exported: Promise<Record<string, unknown>> = Promise.resolve({})
    act(() => {
      update = result.current.updateConfig({ system_prompt: 'new prompt' })
      exported = result.current.exportAllData()
    })
    // Give the export every chance to read storage before the write lands.
    await new Promise((resolve) => setTimeout(resolve, 20))
    release()
    await act(async () => {
      await update
    })

    const snapshot = (await exported).chat_user_config_1 as {
      system_prompt: string
    }
    expect(snapshot.system_prompt).toBe('new prompt')
  })

  it('persists the browser-wide sync key into a stale session copy on load', async () => {
    const store = installStore({
      config_selected_session: 1,
      config_sync_key: SYNC_KEY,
      chat_user_config_1: sessionConfig({
        api_token: 'sk-session-1',
        sync_key: OLD_SYNC_KEY,
      }),
    })

    await renderLoadedConfig()

    expect(storedConfig(store, 1).sync_key).toBe(SYNC_KEY)
  })

  it('exports every session config bound to the browser-wide sync key', async () => {
    installStore({
      config_sync_key: SYNC_KEY,
      chat_user_config_1: sessionConfig({ api_token: 'sk-1' }),
      chat_user_config_2: sessionConfig({
        api_token: 'sk-2',
        sync_key: OLD_SYNC_KEY,
      }),
    })

    const data = await exportAllData()

    expect(data.config_sync_key).toBe(SYNC_KEY)
    expect((data.chat_user_config_2 as { sync_key: string }).sync_key).toBe(
      SYNC_KEY,
    )
  })

  it('rebinds every stored session copy when the sync key changes', async () => {
    const store = installStore({
      config_selected_session: 1,
      config_sync_key: SYNC_KEY,
      chat_user_config_1: sessionConfig({ api_token: 'sk-1' }),
      chat_user_config_2: sessionConfig({ api_token: 'sk-2' }),
    })
    const { result } = await renderLoadedConfig()
    const nextKey = 'sync-n3wn3wn3wn3wn3w'

    await act(async () => {
      await result.current.updateConfig({ sync_key: nextKey })
    })

    expect(store.config_sync_key).toBe(nextKey)
    expect(storedConfig(store, 1).sync_key).toBe(nextKey)
    expect(storedConfig(store, 2).sync_key).toBe(nextKey)
  })

  it('refuses a sync key that is the API key of an inactive session', async () => {
    const store = installStore({
      config_selected_session: 1,
      config_sync_key: SYNC_KEY,
      chat_user_config_1: sessionConfig({ api_token: 'sk-1' }),
      chat_user_config_2: sessionConfig({ api_token: 'sk-inactive-session' }),
    })
    const { result } = await renderLoadedConfig()

    let outcome: unknown
    await act(async () => {
      outcome = await result.current.updateConfig({
        sync_key: 'sk-inactive-session',
      })
    })

    expect(store.config_sync_key).toBe(SYNC_KEY)
    expect(outcome).toEqual({
      rejections: [{ field: 'sync_key', reason: 'sync-key-is-api-token' }],
    })
  })

  it('never lets a cloud backup replace the sync key this device downloads with', async () => {
    const store = installStore({
      config_selected_session: 1,
      config_sync_key: SYNC_KEY,
      chat_user_config_1: sessionConfig({ api_token: 'sk-local-1' }),
      chat_user_config_2: sessionConfig({ api_token: 'sk-local-2' }),
    })

    await importAllData(
      {
        config_sync_key: 'sk-local-2',
        chat_user_config_3: sessionConfig({
          api_token: 'sk-cloud-3',
          sync_key: 'sk-local-2',
        }),
      },
      1,
      'download',
    )

    expect(store.config_sync_key).toBe(SYNC_KEY)
    expect(storedConfig(store, 3).sync_key).toBe(SYNC_KEY)
    expect(storedConfig(store, 3).api_token).toBe('sk-cloud-3')
  })

  it('keeps the list of old backups to delete on this device only', async () => {
    const store = installStore({
      config_sync_key: SYNC_KEY,
      config_pending_backup_deletions: [OLD_SYNC_KEY],
      chat_user_config_1: sessionConfig({ api_token: 'sk-1' }),
    })

    const data = await exportAllData()
    expect(data).not.toHaveProperty('config_pending_backup_deletions')

    await importAllData(
      { config_pending_backup_deletions: ['sync-planted0000000'] },
      1,
      'download',
    )
    expect(store.config_pending_backup_deletions).toEqual([OLD_SYNC_KEY])
  })

  it('reloads the active session after an import, so a later update keeps it', async () => {
    const store = installStore({
      config_selected_session: 1,
      config_sync_key: SYNC_KEY,
      chat_user_config_1: sessionConfig({
        api_token: 'sk-1',
        system_prompt: 'local prompt',
        updated_at: 1,
      }),
    })
    const { result } = await renderLoadedConfig()

    await act(async () => {
      await result.current.importAllData(
        {
          chat_user_config_1: sessionConfig({
            api_token: 'sk-1',
            system_prompt: 'cloud prompt',
            updated_at: 2,
          }),
        },
        'merge',
      )
    })
    expect(result.current.config.system_prompt).toBe('cloud prompt')

    await act(async () => {
      await result.current.updateConfig({ temperature: 0.5 })
    })
    expect(storedConfig(store, 1).system_prompt).toBe('cloud prompt')
    expect(storedConfig(store, 1).temperature).toBe(0.5)
  })
})
