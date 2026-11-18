import { act } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import {
  installStore,
  kvGet,
  kvSet,
  PROVIDER_A,
  renderLoadedConfig,
  sessionConfig,
  storedConfig,
  SYNC_KEY,
} from '../../__tests__/fixtures/credential-store'
import { exportAllData } from '../../utils/data-sync'

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

/**
 * deferred returns a promise together with its resolve function, so a test
 * can hold an asynchronous step open and release it at a chosen moment.
 */
function deferred(): { promise: Promise<void>; resolve: () => void } {
  let resolve = () => {}
  const promise = new Promise<void>((r) => {
    resolve = r
  })
  return { promise, resolve }
}

describe('credential integrity: recovery never switches accounts silently', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('does not hand another session’s key to a clobbered session', async () => {
    const store = installStore({
      config_selected_session: 1,
      config_sync_key: SYNC_KEY,
      chat_user_config_1: sessionConfig({
        api_base: PROVIDER_A,
        api_token: SYNC_KEY,
      }),
      chat_user_config_2: sessionConfig({
        api_base: PROVIDER_A,
        api_token: 'sk-account-b',
      }),
    })

    const { result } = await renderLoadedConfig()

    expect(result.current.config.api_token).not.toBe('sk-account-b')
    expect(result.current.config.api_token).toMatch(/^FREETIER-/)
    expect(result.current.config.api_token_reset_required).toBe(true)
    expect(storedConfig(store, 1).api_token).not.toBe('sk-account-b')
  })
})

describe('credential integrity: inactive sessions are repaired before export', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('never exports a sync key as an inactive session’s API token', async () => {
    const store = installStore({
      config_selected_session: 1,
      config_sync_key: SYNC_KEY,
      chat_user_config_1: sessionConfig({ api_token: 'sk-healthy' }),
      chat_user_config_2: sessionConfig({ api_token: SYNC_KEY }),
    })

    await renderLoadedConfig()
    const exported = await exportAllData()

    const exportedSession2 = exported.chat_user_config_2 as {
      api_token: string
    }
    expect(exportedSession2.api_token).not.toBe(SYNC_KEY)
    expect(storedConfig(store, 2).api_token).not.toBe(SYNC_KEY)
    expect(storedConfig(store, 2).api_token_reset_required).toBe(true)
  })
})

describe('credential integrity: the sync key and API token stay separate', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('rejects a sync key that copies an API-shaped token', async () => {
    const store = installStore({
      config_selected_session: 1,
      config_sync_key: SYNC_KEY,
      chat_user_config_1: sessionConfig({ api_token: 'sk-live-key' }),
    })
    const { result } = await renderLoadedConfig()

    await act(async () => {
      await result.current.updateConfig({ sync_key: 'sk-live-key' })
    })

    expect(result.current.config.sync_key).toBe(SYNC_KEY)
    expect(store.config_sync_key).toBe(SYNC_KEY)
  })

  it('rejects an API token that copies the sync key, whatever its shape', async () => {
    const store = installStore({
      config_selected_session: 1,
      config_sync_key: 'sk-reused-key',
      chat_user_config_1: sessionConfig({
        api_token: 'sk-other',
        sync_key: 'sk-reused-key',
      }),
    })
    const { result } = await renderLoadedConfig()

    await act(async () => {
      await result.current.updateConfig({ api_token: 'sk-reused-key' })
    })

    expect(result.current.config.api_token).toBe('sk-other')
    expect(storedConfig(store, 1).api_token).toBe('sk-other')
  })

  it('keeps a legacy pair that already shares one API-shaped value', async () => {
    const store = installStore({
      config_selected_session: 1,
      config_sync_key: 'sk-reused-key',
      chat_user_config_1: sessionConfig({
        api_token: 'sk-reused-key',
        sync_key: 'sk-reused-key',
      }),
    })

    const { result } = await renderLoadedConfig()

    expect(result.current.config.api_token).toBe('sk-reused-key')
    expect(result.current.config.sync_key).toBe('sk-reused-key')
    expect(store.config_sync_key).toBe('sk-reused-key')
  })
})

describe('credential integrity: URL overrides cannot redirect sync', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    window.history.replaceState({}, '', '/')
  })

  it('ignores a sync_key URL parameter and strips it from the address bar', async () => {
    installStore({
      config_selected_session: 1,
      config_sync_key: SYNC_KEY,
      chat_user_config_1: sessionConfig({ api_token: 'sk-user' }),
    })
    window.history.replaceState({}, '', '/?sync_key=sync-attackerattacker1')

    const { result } = await renderLoadedConfig()

    expect(result.current.config.sync_key).toBe(SYNC_KEY)
    expect(window.location.search).not.toContain('sync_key')
  })
})

describe('credential integrity: config persistence is ordered', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('never lets a slower, older write overwrite a newer one', async () => {
    const store = installStore({
      config_selected_session: 1,
      config_sync_key: SYNC_KEY,
      chat_user_config_1: sessionConfig({ api_token: 'sk-user' }),
    })
    const { result } = await renderLoadedConfig()

    // The first config write stalls (an IndexedDB retry sleeps 300 ms), the
    // second one goes straight through.
    const stall = deferred()
    let configWrites = 0
    kvSet.mockImplementation(async (key: string, value: unknown) => {
      if (key === 'chat_user_config_1') {
        configWrites += 1
        if (configWrites === 1) await stall.promise
      }
      store[key] = value
    })

    let first: Promise<void> = Promise.resolve()
    let second: Promise<void> = Promise.resolve()
    act(() => {
      first = result.current.updateConfig({ temperature: 0.2 })
      second = result.current.updateConfig({ temperature: 0.7 })
    })
    await act(async () => {
      await Promise.resolve()
      stall.resolve()
      await Promise.all([first, second])
    })

    expect(storedConfig(store, 1).temperature).toBe(0.7)
  })

  it('writes an update to the session it was made in, even after a switch', async () => {
    const store = installStore({
      config_selected_session: 1,
      config_sync_key: SYNC_KEY,
      chat_user_config_1: sessionConfig({
        api_token: 'sk-one',
        session_name: 'one',
      }),
      chat_user_config_2: sessionConfig({
        api_token: 'sk-two',
        session_name: 'two',
      }),
    })
    const { result } = await renderLoadedConfig()

    const stall = deferred()
    kvSet.mockImplementation(async (key: string, value: unknown) => {
      if (key === 'config_sync_key') await stall.promise
      store[key] = value
    })

    let update: Promise<void> = Promise.resolve()
    act(() => {
      update = result.current.updateConfig({
        sync_key: 'sync-rotatedrotated1',
      })
    })
    await act(async () => {
      const switching = result.current.switchSession(2)
      await Promise.resolve()
      stall.resolve()
      await Promise.all([update, switching])
    })

    expect(storedConfig(store, 2).session_name).toBe('two')
    expect(storedConfig(store, 2).api_token).toBe('sk-two')
    expect(storedConfig(store, 1).sync_key).toBe('sync-rotatedrotated1')
    expect(kvGet).toHaveBeenCalled()
  })
})
