import { renderHook } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import {
  installStore,
  sessionConfig,
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
})
