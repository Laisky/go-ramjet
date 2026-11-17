import { act, renderHook } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { ChatModels } from '../../models'
import { DefaultSessionConfig } from '../../types'
import { useConfigHandlers } from '../use-config-handlers'

vi.mock('@/utils/api', () => ({
  getApiBase: () => 'http://localhost:24456',
  getSHA1: vi.fn(),
}))

/**
 * renderHandlers mounts useConfigHandlers around updateConfig.
 */
function renderHandlers(
  updateConfig: ReturnType<typeof vi.fn>,
  allowedModels?: string[],
) {
  return renderHook(() =>
    useConfigHandlers({
      config: DefaultSessionConfig,
      updateConfig,
      exportAllData: vi.fn().mockResolvedValue({}),
      importAllData: vi.fn().mockResolvedValue(undefined),
      allowedModels,
    }),
  )
}

describe('useConfigHandlers', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('hands the settings sidebar a handler that waits for the write and reports failure', async () => {
    const failure = new Error('QuotaExceededError')
    const updateConfig = vi.fn().mockRejectedValue(failure)
    const { result } = renderHandlers(updateConfig)

    await expect(
      result.current.handleConfigChange({ api_token: 'sk-new' }),
    ).rejects.toBe(failure)

    updateConfig.mockResolvedValue({ rejections: [] })
    await expect(
      result.current.handleConfigChange({ api_token: 'sk-new' }),
    ).resolves.toEqual({ rejections: [] })
  })

  it('logs, rather than leaks, a failed background update', async () => {
    const updateConfig = vi.fn().mockRejectedValue(new Error('offline'))
    const logged = vi.spyOn(console, 'error').mockImplementation(() => {})
    const { result } = renderHandlers(updateConfig)

    await act(async () => {
      result.current.handleChatModelChange('gpt-x')
      await Promise.resolve()
    })

    expect(updateConfig).toHaveBeenCalledWith({
      selected_model: 'gpt-x',
      selected_chat_model: 'gpt-x',
    })
    expect(logged).toHaveBeenCalledWith(expect.stringContaining('offline'))
  })

  it('moves the selection into the allowed models', async () => {
    const updateConfig = vi.fn().mockResolvedValue({ rejections: [] })
    const allowed = ChatModels.find(
      (model) => model !== DefaultSessionConfig.selected_chat_model,
    ) as string

    await act(async () => {
      renderHandlers(updateConfig, [allowed])
    })

    expect(updateConfig).toHaveBeenCalledTimes(1)
    expect(updateConfig.mock.calls[0][0]).toMatchObject({
      selected_chat_model: allowed,
      selected_model: allowed,
    })
  })
})
