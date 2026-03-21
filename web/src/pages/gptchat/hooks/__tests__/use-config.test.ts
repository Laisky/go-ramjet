import { kvGet, kvSet } from '@/utils/storage'
import { act, renderHook, waitFor } from '@testing-library/react'
import { type Mock, beforeEach, describe, expect, it, vi } from 'vitest'
import { ChatModelGPT5Mini, DefaultModel } from '../../models'
import { DefaultSessionConfig } from '../../types'
import { useConfig } from '../use-config'

interface Deferred<T> {
  promise: Promise<T>
  resolve: (value: T) => void
}

/**
 * createDeferred builds a promise whose resolution is controlled by the test.
 */
function createDeferred<T>(): Deferred<T> {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((res) => {
    resolve = res
  })

  return { promise, resolve }
}

// Mock storage
vi.mock('@/utils/storage', () => ({
  kvGet: vi.fn(),
  kvSet: vi.fn(),
  kvDel: vi.fn(),
  kvList: vi.fn().mockResolvedValue([]),
  StorageKeys: {
    SESSION_CONFIG_PREFIX: 'chat_user_config_',
    SELECTED_SESSION: 'config_selected_session',
    VERSION_DATE: 'config_version_date',
    SESSION_HISTORY_PREFIX: 'chat_user_session_',
    SESSION_ORDER: 'config_session_order',
    SYNC_KEY: 'config_sync_key',
  },
}))

// Mock migration
vi.mock('@/pages/gptchat/utils/migration', () => ({
  migrateLegacyData: vi.fn().mockResolvedValue(undefined),
}))

// Mock API base
vi.mock('@/utils/api', () => ({
  getApiBase: () => 'http://localhost:24456',
}))

describe('useConfig', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('should migrate legacy selected_model to selected_chat_model', async () => {
    const legacyConfig = {
      selected_model: ChatModelGPT5Mini,
      // selected_chat_model is missing
    }

    ;(kvGet as Mock).mockImplementation((key: string) => {
      if (key === 'config_selected_session') return Promise.resolve(1)
      if (key === 'chat_user_config_1') return Promise.resolve(legacyConfig)
      return Promise.resolve(null)
    })

    const { result } = renderHook(() => useConfig())

    await waitFor(
      () => {
        expect(result.current.isLoading).toBe(false)
      },
      { timeout: 3000 },
    )

    expect(result.current.config.selected_model).toBe(ChatModelGPT5Mini)
    expect(result.current.config.selected_chat_model).toBe(ChatModelGPT5Mini)
  })

  it('should use DefaultModel if no saved config exists', async () => {
    ;(kvGet as Mock).mockResolvedValue(null)

    const { result } = renderHook(() => useConfig())

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false)
    })

    expect(result.current.config.selected_model).toBe(DefaultModel)
    expect(result.current.config.selected_chat_model).toBe(DefaultModel)
  })

  it('should migrate legacy image model to selected_draw_model', async () => {
    const legacyConfig = {
      selected_model: 'dall-e-3',
    }

    ;(kvGet as Mock).mockImplementation((key: string) => {
      if (key === 'config_selected_session') return Promise.resolve(1)
      if (key === 'chat_user_config_1') return Promise.resolve(legacyConfig)
      return Promise.resolve(null)
    })

    const { result } = renderHook(() => useConfig())

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false)
    })

    expect(result.current.config.selected_model).toBe('dall-e-3')
    expect(result.current.config.selected_chat_model).toBe(DefaultModel)
    expect(result.current.config.selected_draw_model).toBe('dall-e-3')
  })

  it('should update config state before async persistence completes', async () => {
    const syncKeyDeferred = createDeferred<string | null>()

    ;(kvGet as Mock).mockImplementation((key: string) => {
      if (key === 'config_selected_session') return Promise.resolve(1)
      if (key === 'chat_user_config_1') {
        return Promise.resolve({
          ...DefaultSessionConfig,
          sync_key: 'sync-existing',
        })
      }
      if (key === 'config_sync_key') return syncKeyDeferred.promise
      return Promise.resolve(null)
    })

    const { result } = renderHook(() => useConfig())

    syncKeyDeferred.resolve('sync-existing')

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false)
    })

    const persistDeferred = createDeferred<string | null>()
    ;(kvGet as Mock).mockImplementation((key: string) => {
      if (key === 'config_sync_key') return persistDeferred.promise
      if (key === 'config_selected_session') return Promise.resolve(1)
      if (key === 'chat_user_config_1')
        return Promise.resolve(result.current.config)
      return Promise.resolve(null)
    })

    await act(async () => {
      void result.current.updateConfig({ system_prompt: 'updated prompt' })
    })

    expect(result.current.config.system_prompt).toBe('updated prompt')

    persistDeferred.resolve('sync-existing')

    await waitFor(() => {
      expect(result.current.config.system_prompt).toBe('updated prompt')
    })
  })

  it('should persist an explicitly updated sync key without blocking state updates', async () => {
    ;(kvGet as Mock).mockImplementation((key: string) => {
      if (key === 'config_selected_session') return Promise.resolve(1)
      if (key === 'chat_user_config_1') return Promise.resolve(null)
      if (key === 'config_sync_key') return Promise.resolve('sync-initial')
      return Promise.resolve(null)
    })

    const { result } = renderHook(() => useConfig())

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false)
    })

    await act(async () => {
      void result.current.updateConfig({ sync_key: 'sync-updated' })
    })

    expect(result.current.config.sync_key).toBe('sync-updated')
  })

  it('should preserve the current token when purging all sessions', async () => {
    const store: Record<string, unknown> = {
      config_selected_session: 1,
      config_sync_key: 'sync-existing',
      chat_user_config_1: {
        ...DefaultSessionConfig,
        api_token: 'paid-token-12345',
        api_base: 'https://proxy.example.com',
        sync_key: 'sync-existing',
      },
    }

    ;(kvGet as Mock).mockImplementation((key: string) => {
      return Promise.resolve(store[key] ?? null)
    })
    ;(kvSet as Mock).mockImplementation(async (key: string, value: unknown) => {
      store[key] = value
    })

    const { result } = renderHook(() => useConfig())

    await waitFor(() => {
      expect(result.current?.isLoading).toBe(false)
    })

    await act(async () => {
      await result.current.purgeAllSessions()
    })

    expect(result.current.config.api_token).toBe('paid-token-12345')
    expect(result.current.config.api_base).toBe('https://proxy.example.com')
    expect(
      (store.chat_user_config_1 as typeof DefaultSessionConfig).api_token,
    ).toBe('paid-token-12345')
  })

  it('should preserve the current token when switching to a new session', async () => {
    const store: Record<string, unknown> = {
      config_selected_session: 1,
      config_sync_key: 'sync-existing',
      chat_user_config_1: {
        ...DefaultSessionConfig,
        api_token: 'paid-token-12345',
        sync_key: 'sync-existing',
      },
    }

    ;(kvGet as Mock).mockImplementation((key: string) => {
      return Promise.resolve(store[key] ?? null)
    })
    ;(kvSet as Mock).mockImplementation(async (key: string, value: unknown) => {
      store[key] = value
    })

    const { result } = renderHook(() => useConfig())

    await waitFor(() => {
      expect(result.current?.isLoading).toBe(false)
    })

    await act(async () => {
      await result.current.switchSession(2)
    })

    await waitFor(() => {
      expect(result.current.config.api_token).toBe('paid-token-12345')
    })

    expect(result.current.sessionId).toBe(2)
    expect(
      (store.chat_user_config_2 as typeof DefaultSessionConfig).api_token,
    ).toBe('paid-token-12345')
  })
})
