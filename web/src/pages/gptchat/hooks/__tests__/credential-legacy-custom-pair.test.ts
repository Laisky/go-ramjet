import { act, renderHook, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import {
  installStore,
  PROVIDER_A,
  sessionConfig,
  storedConfig,
  SYNC_KEY,
} from '../../__tests__/fixtures/credential-store'
import { api } from '../../utils/api'
import { repairStoredSessionCredentials } from '../session-credentials'
import { useApiTokenRecovery } from '../use-api-token-recovery'
import { useConfig } from '../use-config'
import { useSyncKeyRotation } from '../use-sync-key-rotation'

vi.mock(
  '@/utils/storage',
  () => import('../../__tests__/fixtures/storage-mock'),
)

vi.mock('@/pages/gptchat/utils/migration', () => ({
  migrateLegacyData: vi.fn().mockResolvedValue(undefined),
}))

vi.mock('@/utils/api', () => ({
  getApiBase: () => 'http://localhost:24456',
  getSHA1: vi.fn(),
}))

vi.mock('../../utils/api', () => ({
  api: {
    downloadUserData: vi.fn(),
    uploadUserData: vi.fn(),
    deleteUserData: vi.fn(),
  },
}))

/**
 * CUSTOM_PAIR is a legacy value without an API-key prefix that a session
 * used as both its API key and the sync key, on purpose or through a
 * password manager: nothing in the data tells which.
 */
const CUSTOM_PAIR = 'my-custom-provider-token'

/**
 * useRecoveryHarness wires useConfig, sync key rotation and API key
 * recovery the way the GPTChat page does.
 */
function useRecoveryHarness() {
  const cfg = useConfig()
  const rotateSyncKey = useSyncKeyRotation({
    exportData: cfg.exportAllData,
    importData: cfg.importAllData,
    updateConfig: cfg.updateConfig,
  })
  const recovery = useApiTokenRecovery({
    sessionId: cfg.sessionId,
    config: cfg.config,
    updateConfig: cfg.updateConfig,
    rotateSyncKey,
  })
  return { cfg, recovery }
}

/**
 * renderRecoveryHarness mounts useRecoveryHarness and waits for hydration.
 */
async function renderRecoveryHarness() {
  const hook = renderHook(() => useRecoveryHarness())
  await waitFor(() => {
    expect(hook.result.current.cfg.isLoading).toBe(false)
  })
  return hook
}

describe('legacy API key that is also a custom sync key', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(api.downloadUserData).mockResolvedValue({})
    vi.mocked(api.uploadUserData).mockResolvedValue(undefined)
    vi.mocked(api.deleteUserData).mockResolvedValue(undefined)
  })

  afterEach(() => {
    window.history.replaceState({}, '', '/')
  })

  it('is never sent, but is kept for the user to decide', async () => {
    const store = installStore({
      config_selected_session: 1,
      config_sync_key: CUSTOM_PAIR,
      chat_user_config_1: sessionConfig({
        api_token: CUSTOM_PAIR,
        sync_key: CUSTOM_PAIR,
      }),
    })

    const { result } = await renderRecoveryHarness()

    const config = result.current.cfg.config
    expect(config.api_token).toMatch(/^FREETIER-/)
    expect(config.api_token_reset_required).toBe(true)
    expect(config.api_token_before_reset).toBe(CUSTOM_PAIR)
    expect(storedConfig(store, 1).api_token_before_reset).toBe(CUSTOM_PAIR)
    expect(result.current.recovery.needsRotation).toBe(true)
  })

  it('holds a link endpoint until the user decides, so a restore stays on its endpoint', async () => {
    window.history.replaceState({}, '', '/?api_base=https://evil.example')
    const store = installStore({
      config_selected_session: 1,
      config_sync_key: CUSTOM_PAIR,
      chat_user_config_1: sessionConfig({
        api_base: PROVIDER_A,
        api_token: CUSTOM_PAIR,
        sync_key: CUSTOM_PAIR,
      }),
    })

    const { result } = await renderRecoveryHarness()

    const config = result.current.cfg.config
    expect(config.api_token_before_reset).toBe(CUSTOM_PAIR)
    expect(config.api_base).toBe(PROVIDER_A)
    expect(storedConfig(store, 1).api_base).toBe(PROVIDER_A)
    expect(result.current.cfg.pendingLinkChange).toEqual({
      sessionId: 1,
      apiBase: 'https://evil.example',
    })
  })

  it('does not keep a generated sync key, which is never an API key', async () => {
    installStore({
      config_selected_session: 1,
      config_sync_key: SYNC_KEY,
      chat_user_config_1: sessionConfig({ api_token: SYNC_KEY }),
    })

    const { result } = await renderRecoveryHarness()

    expect(result.current.cfg.config.api_token_reset_required).toBe(true)
    expect(result.current.cfg.config.api_token_before_reset).toBeUndefined()
  })

  it('is set aside in inactive sessions too', async () => {
    const store = installStore({
      chat_user_config_2: sessionConfig({
        api_token: CUSTOM_PAIR,
        sync_key: CUSTOM_PAIR,
      }),
    })

    expect(await repairStoredSessionCredentials(CUSTOM_PAIR, 1)).toEqual([2])
    expect(storedConfig(store, 2).api_token_before_reset).toBe(CUSTOM_PAIR)
  })

  it('is restored after the sync key is rotated, when the user says it is theirs', async () => {
    const store = installStore({
      config_selected_session: 1,
      config_sync_key: CUSTOM_PAIR,
      chat_user_config_1: sessionConfig({
        api_token: CUSTOM_PAIR,
        sync_key: CUSTOM_PAIR,
      }),
      chat_user_config_2: sessionConfig({
        api_token: 'sk-session-2',
        sync_key: CUSTOM_PAIR,
      }),
    })
    const { result } = await renderRecoveryHarness()

    await act(async () => {
      expect(await result.current.recovery.restorePrevious()).toBe(true)
    })

    const newKey = store.config_sync_key as string
    expect(newKey).toMatch(/^sync-/)
    expect(newKey).not.toBe(CUSTOM_PAIR)
    expect(api.uploadUserData).toHaveBeenCalledWith(
      newKey,
      expect.objectContaining({ config_sync_key: newKey }),
    )
    expect(api.deleteUserData).toHaveBeenCalledWith(CUSTOM_PAIR)
    expect(store.config_pending_backup_deletions).toEqual([])

    const session1 = storedConfig(store, 1)
    expect(session1.api_token).toBe(CUSTOM_PAIR)
    expect(session1.sync_key).toBe(newKey)
    expect(session1.api_token_reset_required).toBe(false)
    expect(session1.api_token_before_reset).toBeUndefined()
    expect(storedConfig(store, 2).sync_key).toBe(newKey)
    expect(result.current.cfg.config.api_token).toBe(CUSTOM_PAIR)
  })

  it('is restored without a rotation once the sync key has changed', async () => {
    const store = installStore({
      config_selected_session: 1,
      config_sync_key: SYNC_KEY,
      chat_user_config_1: sessionConfig({
        api_token: 'FREETIER-placeholder',
        api_token_reset_required: true,
        api_token_before_reset: CUSTOM_PAIR,
      }),
    })
    const { result } = await renderRecoveryHarness()
    expect(result.current.recovery.needsRotation).toBe(false)

    await act(async () => {
      await result.current.recovery.restorePrevious()
    })

    expect(api.uploadUserData).not.toHaveBeenCalled()
    expect(store.config_sync_key).toBe(SYNC_KEY)
    expect(storedConfig(store, 1).api_token).toBe(CUSTOM_PAIR)
    expect(storedConfig(store, 1).api_token_reset_required).toBe(false)
  })

  it('changes nothing when the rotation fails', async () => {
    vi.mocked(api.uploadUserData).mockRejectedValue(new Error('offline'))
    const store = installStore({
      config_selected_session: 1,
      config_sync_key: CUSTOM_PAIR,
      chat_user_config_1: sessionConfig({
        api_token: CUSTOM_PAIR,
        sync_key: CUSTOM_PAIR,
      }),
    })
    const { result } = await renderRecoveryHarness()

    let failure: unknown
    await act(async () => {
      failure = await result.current.recovery
        .restorePrevious()
        .catch((error: unknown) => error)
    })

    expect(String(failure)).toContain('offline')
    expect(store.config_sync_key).toBe(CUSTOM_PAIR)
    expect(storedConfig(store, 1).api_token).toMatch(/^FREETIER-/)
    expect(storedConfig(store, 1).api_token_reset_required).toBe(true)
    expect(storedConfig(store, 1).api_token_before_reset).toBe(CUSTOM_PAIR)
  })

  it('is forgotten once the user ends the reset another way', async () => {
    const store = installStore({
      config_selected_session: 1,
      config_sync_key: CUSTOM_PAIR,
      chat_user_config_1: sessionConfig({
        api_token: CUSTOM_PAIR,
        sync_key: CUSTOM_PAIR,
      }),
    })
    const { result } = await renderRecoveryHarness()

    await act(async () => {
      await result.current.cfg.updateConfig({ api_token_reset_required: false })
    })

    expect(storedConfig(store, 1).api_token_reset_required).toBe(false)
    expect(storedConfig(store, 1).api_token_before_reset).toBeUndefined()
  })
})
