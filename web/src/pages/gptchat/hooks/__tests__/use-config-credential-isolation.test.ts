import { kvDel, kvGet, kvList, kvSet } from '@/utils/storage'
import { act, renderHook, waitFor } from '@testing-library/react'
import {
  type Mock,
  afterEach,
  beforeEach,
  describe,
  expect,
  it,
  vi,
} from 'vitest'

import { DefaultSessionConfig, type SessionConfig } from '../../types'
import { importAllData } from '../../utils/data-sync'
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
    CHAT_DATA_PREFIX: 'chat_data_',
    DELETED_CHAT_IDS: 'chat_deleted_chat_ids',
  },
}))

vi.mock('@/pages/gptchat/utils/migration', () => ({
  migrateLegacyData: vi.fn().mockResolvedValue(undefined),
}))

vi.mock('@/utils/api', () => ({
  getApiBase: () => 'http://localhost:24456',
}))

const SYNC_KEY = 'sync-k2x9q4w7p1z8m3'
const PROVIDER_A = 'https://provider-a.example'
const PROVIDER_B = 'https://provider-b.example'

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
  ;(kvDel as Mock).mockImplementation(async (key: string) => {
    delete store[key]
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
 * storedConfig reads the persisted config of the given session.
 */
function storedConfig(
  store: Record<string, unknown>,
  sessionId: number,
): SessionConfig {
  return store[`chat_user_config_${sessionId}`] as SessionConfig
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

  it('recovers the token of a sibling on the same endpoint', async () => {
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

    expect(result.current.config.api_token).toBe('sk-provider-a')
    expect(result.current.config.api_token_reset_required).toBeFalsy()
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

  it('treats several sessions sharing one token as a single candidate', async () => {
    const store: Record<string, unknown> = {
      config_selected_session: 1,
      config_sync_key: SYNC_KEY,
      chat_user_config_1: sessionConfig({ api_token: SYNC_KEY }),
      chat_user_config_2: sessionConfig({ api_token: 'sk-shared' }),
      chat_user_config_3: sessionConfig({ api_token: ' sk-shared ' }),
    }
    installStore(store)

    const { result } = await renderLoadedConfig()

    expect(result.current.config.api_token).toBe('sk-shared')
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

  it('repairs a clobbered config that arrives through a sync download', async () => {
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
    expect(storedConfig(store, 1).api_token).toBe(SYNC_KEY)

    const { result } = await renderLoadedConfig()

    expect(result.current.config.api_token).toBe('sk-from-cloud')
    expect(storedConfig(store, 1).api_token).toBe('sk-from-cloud')
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

  it('recovers against the endpoint chosen by the URL', async () => {
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
    expect(result.current.config.api_token).toBe('sk-provider-b')
  })
})

describe('useConfig API token reset state machine', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('keeps the reset pending while the user is still typing a key', async () => {
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
    await act(async () => {
      await first.result.current.updateConfig({ api_token: 's' })
    })

    expect(first.result.current.config.api_token).toBe('s')
    expect(first.result.current.config.api_token_reset_required).toBe(true)
    first.unmount()

    const { result } = await renderLoadedConfig()
    expect(result.current.config.api_token).toBe('s')
    expect(result.current.config.api_token_reset_required).toBe(true)
  })

  it('does not overwrite a key the user started typing with a sibling token', async () => {
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
