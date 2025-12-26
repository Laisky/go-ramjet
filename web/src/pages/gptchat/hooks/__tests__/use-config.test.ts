import { renderHook, waitFor } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { useConfig } from '../use-config'
import { kvGet } from '@/utils/storage'
import { DefaultModel, ChatModelGPT4Turbo } from '../../models'

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
  },
}))

// Mock migration
vi.mock('@/pages/gptchat/utils/migration', () => ({
  migrateLegacyData: vi.fn().mockResolvedValue(undefined),
}))

// Mock API base
vi.mock('@/utils/api', () => ({
  API_BASE: 'http://localhost:24456',
}))

describe('useConfig', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('should migrate legacy selected_model to selected_chat_model', async () => {
    const legacyConfig = {
      selected_model: ChatModelGPT4Turbo,
      // selected_chat_model is missing
    }

    ;(kvGet as any).mockImplementation((key: string) => {
      if (key === 'config_selected_session') return Promise.resolve(1)
      if (key === 'chat_user_config_1') return Promise.resolve(legacyConfig)
      return Promise.resolve(null)
    })

    const { result } = renderHook(() => useConfig())

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false)
    }, { timeout: 3000 })

    expect(result.current.config.selected_model).toBe(ChatModelGPT4Turbo)
    expect(result.current.config.selected_chat_model).toBe(ChatModelGPT4Turbo)
  })

  it('should use DefaultModel if no saved config exists', async () => {
    ;(kvGet as any).mockResolvedValue(null)

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

    ;(kvGet as any).mockImplementation((key: string) => {
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
})
