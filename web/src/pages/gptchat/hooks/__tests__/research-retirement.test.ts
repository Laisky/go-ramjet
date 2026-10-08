import { act, renderHook } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  AllModels,
  DefaultModel,
  ModelCategories,
  ImageModelFluxDev,
} from '../../models'
import { DefaultSessionConfig } from '../../types'
import {
  applyUrlOverridesToConfig,
  normalizeConfigNumericFields,
} from '../../utils/config-helpers'
import { useChat } from '../use-chat'

const mocks = vi.hoisted(() => ({
  stream: vi.fn().mockResolvedValue(undefined),
  streamingConfig: vi.fn(),
  save: vi.fn().mockResolvedValue(undefined),
  image: vi.fn().mockResolvedValue(undefined),
}))

vi.mock('@/utils/storage', () => ({
  kvDel: vi.fn().mockResolvedValue(undefined),
  StorageKeys: {
    CHAT_DATA_PREFIX: 'chat_data_',
    SESSION_HISTORY_PREFIX: 'chat_user_session_',
  },
}))
vi.mock('../chat-storage', () => ({
  // useChatStorage supplies persistence doubles without issuing remote requests.
  useChatStorage: () => ({
    saveMessage: mocks.save,
    insertMessagePair: vi.fn().mockResolvedValue(undefined),
    loadMessages: vi.fn(),
    clearMessages: vi.fn(),
    deleteMessage: vi.fn(),
    markSessionMutation: vi.fn(),
  }),
}))
vi.mock('../chat-streaming', () => ({
  // useChatStreaming records the config passed to ordinary streaming.
  useChatStreaming: (options: { config: unknown }) => {
    mocks.streamingConfig(options.config)
    return { streamAssistantReply: mocks.stream }
  },
}))
vi.mock('../chat-media', () => ({
  runImageModelFlow: mocks.image,
  runMaskInpainting: vi.fn(),
}))

describe('llm-storm research retirement', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    window.history.replaceState({}, '', '/')
  })

  it('removes the custom research model from every selectable catalog', () => {
    expect(AllModels).not.toContain('deep-research')
    expect(Object.values(ModelCategories).flat()).not.toContain('deep-research')
  })

  it('normalizes stored research selections while preserving ordinary settings', () => {
    const config = normalizeConfigNumericFields({
      ...DefaultSessionConfig,
      selected_model: 'deep-research',
      selected_chat_model: 'deep-research',
      selected_draw_model: ImageModelFluxDev,
      chat_switch: { ...DefaultSessionConfig.chat_switch, enable_mcp: true },
    })
    expect(config.selected_model).toBe(DefaultModel)
    expect(config.selected_chat_model).toBe(DefaultModel)
    expect(config.selected_draw_model).toBe(ImageModelFluxDev)
    expect(config.chat_switch.enable_mcp).toBe(true)
  })

  it('does not reintroduce retired research through a shared URL', () => {
    window.history.replaceState(
      {},
      '',
      '/?model=deep-research&selected_chat_model=deep-research',
    )
    const { config } = applyUrlOverridesToConfig({ ...DefaultSessionConfig })
    expect(config.selected_model).toBe(DefaultModel)
    expect(config.selected_chat_model).toBe(DefaultModel)
    expect(AllModels).not.toContain('deep-research')
  })

  it.each([
    'o3-deep-research',
    'o4-mini-deep-research',
    'o3-deepresearch',
    'o4-mini-deepresearch',
  ])('preserves provider model %s', (model) => {
    const config = normalizeConfigNumericFields({
      ...DefaultSessionConfig,
      selected_model: model,
    })
    expect(config.selected_model).toBe(model)
  })

  it('uses ordinary streaming for all four entry points with a stale research config', async () => {
    const { result } = renderHook(() =>
      useChat({
        sessionId: 1,
        config: { ...DefaultSessionConfig, selected_model: 'deep-research' },
      }),
    )
    await act(async () => {
      await result.current.sendMessage('Retained question')
    })
    const chatId = result.current.messages[0].chatID
    await act(async () => {
      await result.current.insertMessageAt(0, 'Inserted question')
    })
    await act(async () => {
      await result.current.regenerateMessage(chatId)
    })
    await act(async () => {
      await result.current.editAndRetry(chatId, 'Edited question')
    })
    expect(mocks.stream).toHaveBeenCalledTimes(4)
    expect(mocks.streamingConfig).toHaveBeenLastCalledWith(
      expect.objectContaining({ selected_model: DefaultModel }),
    )
    expect(mocks.image).not.toHaveBeenCalled()
  })

  it('preserves image model dispatch', async () => {
    const { result } = renderHook(() =>
      useChat({
        sessionId: 1,
        config: { ...DefaultSessionConfig, selected_model: ImageModelFluxDev },
      }),
    )
    await act(async () => {
      await result.current.sendMessage('Draw a tree')
    })
    expect(mocks.image).toHaveBeenCalledOnce()
    expect(mocks.stream).not.toHaveBeenCalled()
  })
})
