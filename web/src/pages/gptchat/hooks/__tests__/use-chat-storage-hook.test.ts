import { kvGet } from '@/utils/storage'
import { renderHook } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useChatStorage } from '../chat-storage'

// Mock storage
vi.mock('@/utils/storage', () => ({
  kvGet: vi.fn(),
  kvSet: vi.fn(),
  kvDel: vi.fn(),
  StorageKeys: {
    SESSION_HISTORY_PREFIX: 'chat_user_session_',
    CHAT_DATA_PREFIX: 'chat_data_',
  },
}))

describe('useChatStorage hook', () => {
  const setMessages = vi.fn()
  const setError = vi.fn()

  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('should load messages for the given sessionId', async () => {
    const mockHistory = [{ chatID: 'chat1', role: 'user', content: 'hi' }]
    const mockUserData = { chatID: 'chat1', role: 'user', content: 'hi' }

    ;(kvGet as any).mockImplementation((key: string) => {
      if (key === 'chat_user_session_1') return Promise.resolve(mockHistory)
      if (key === 'chat_data_user_chat1') return Promise.resolve(mockUserData)
      return Promise.resolve(null)
    })

    const { result } = renderHook(() =>
      useChatStorage({
        sessionId: 1,
        setMessages,
        setError,
      }),
    )

    await result.current.loadMessages()

    expect(setMessages).toHaveBeenCalledWith(
      expect.arrayContaining([expect.objectContaining({ content: 'hi' })]),
    )
  })

  it('should abort loading if sessionId changes during fetch (race condition)', async () => {
    let resolveFetch: (val: any) => void
    const fetchPromise = new Promise((resolve) => {
      resolveFetch = resolve
    })

    ;(kvGet as any).mockImplementation((key: string) => {
      if (key === 'chat_user_session_1') return fetchPromise
      return Promise.resolve(null)
    })

    const { result, rerender } = renderHook(
      ({ sessionId }) => useChatStorage({ sessionId, setMessages, setError }),
      { initialProps: { sessionId: 1 } },
    )

    // Start loading for session 1
    const loadPromise = result.current.loadMessages()

    // Change sessionId to 2
    rerender({ sessionId: 2 })

    // Resolve the fetch for session 1
    resolveFetch!([{ chatID: 'chat1', role: 'user', content: 'session 1 msg' }])
    await loadPromise

    // setMessages should NOT have been called with session 1 messages
    expect(setMessages).not.toHaveBeenCalledWith(
      expect.arrayContaining([
        expect.objectContaining({ content: 'session 1 msg' }),
      ]),
    )
  })
})
