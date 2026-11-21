import { renderHook } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import {
  installStore,
  kvSet,
  PROVIDER_A,
  sessionConfig,
  SYNC_KEY,
} from '../../__tests__/fixtures/credential-store'
import { migrateLegacyData } from '../../utils/migration'
import { useConfig } from '../use-config'

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

describe('useConfig after unmount', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('does not update state when the initial load finishes after unmount', async () => {
    installStore({
      config_selected_session: 1,
      chat_user_config_1: sessionConfig({ api_token: 'sk-stored' }),
    })
    let releaseLoad: () => void = () => undefined
    vi.mocked(migrateLegacyData).mockImplementationOnce(
      () =>
        new Promise<void>((resolve) => {
          releaseLoad = resolve
        }),
    )
    const rejections: string[] = []
    const onRejection = (reason: unknown) => rejections.push(String(reason))
    process.on('unhandledRejection', onRejection)

    try {
      const { unmount } = renderHook(() => useConfig())
      await vi.waitFor(() => expect(migrateLegacyData).toHaveBeenCalled())
      unmount()

      // A test file's jsdom environment is torn down while the load is still
      // pending, so React then finds no window when the load sets state.
      vi.stubGlobal('window', undefined)
      releaseLoad()
      // Every storage call is mocked, so the rest of the load runs in
      // microtasks that finish before this timer fires.
      await new Promise((resolve) => setTimeout(resolve, 50))
    } finally {
      vi.unstubAllGlobals()
      process.off('unhandledRejection', onRejection)
    }

    expect(rejections).toEqual([])
  })
  it('does not publish a captured link prompt after genuine unmount', async () => {
    window.history.replaceState(
      {},
      '',
      '/?api_base=https%3A%2F%2Fprovider-b.example',
    )
    const store = installStore({
      config_selected_session: 1,
      config_sync_key: SYNC_KEY,
      chat_user_config_1: sessionConfig({
        session_name: '',
        api_base: PROVIDER_A,
        api_token: 'sk-SYNTHETIC-ORDINARY-USER-KEY-0123456789',
      }),
    })
    let releaseWrite: () => void = () => undefined
    const writeGate = new Promise<void>((resolve) => {
      releaseWrite = resolve
    })
    let writeStarted = false
    kvSet.mockImplementation(async (key: string, value: unknown) => {
      if (key === 'chat_user_config_1') {
        writeStarted = true
        await writeGate
      }
      store[key] = value
    })
    const errors: string[] = []
    const errorSpy = vi
      .spyOn(console, 'error')
      .mockImplementation((...args) => {
        errors.push(args.map(String).join(' '))
      })
    const rejections: string[] = []
    const onRejection = (reason: unknown) => rejections.push(String(reason))
    process.on('unhandledRejection', onRejection)
    try {
      const { unmount } = renderHook(() => useConfig())
      await vi.waitFor(() => expect(writeStarted).toBe(true))
      // Hydration consumed the ordinary endpoint link and captured its prompt,
      // but its ordered configuration write has not completed.
      expect(window.location.search).toBe('')
      unmount()
      vi.stubGlobal('window', undefined)
      releaseWrite()
      await new Promise((resolve) => setTimeout(resolve, 50))
    } finally {
      releaseWrite()
      vi.unstubAllGlobals()
      window.history.replaceState({}, '', '/')
      process.off('unhandledRejection', onRejection)
      errorSpy.mockRestore()
    }
    expect(rejections).toEqual([])
    expect(errors).toEqual([])
  })
})
