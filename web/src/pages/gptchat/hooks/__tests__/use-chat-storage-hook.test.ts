import { kvGet, kvSet } from '@/utils/storage'
import { renderHook } from '@testing-library/react'
import { type Mock, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ChatMessageData } from '../../types'
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
  const setMessagesAlt = vi.fn()

  const deferred = <T>() => {
    let resolve!: (value: T | PromiseLike<T>) => void
    const promise = new Promise<T>((res) => {
      resolve = res
    })
    return { promise, resolve }
  }

  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('should load messages for the given sessionId', async () => {
    const mockHistory = [{ chatID: 'chat1', role: 'user', content: 'hi' }]
    const mockUserData = { chatID: 'chat1', role: 'user', content: 'hi' }

    ;(kvGet as Mock).mockImplementation((key: string) => {
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

  it('should replay messages in persisted role order', async () => {
    const history = [
      { chatID: 'old', role: 'user' as const, content: 'old question' },
      { chatID: 'inserted', role: 'user' as const, content: 'new question' },
      {
        chatID: 'inserted',
        role: 'assistant' as const,
        content: 'new answer',
      },
      { chatID: 'old', role: 'assistant' as const, content: 'old answer' },
    ]
    const messages = new Map([
      ['chat_data_user_old', history[0]],
      ['chat_data_user_inserted', history[1]],
      ['chat_data_assistant_inserted', history[2]],
      ['chat_data_assistant_old', history[3]],
    ])

    ;(kvGet as Mock).mockImplementation((key: string) => {
      if (key === 'chat_user_session_1') return Promise.resolve(history)
      return Promise.resolve(messages.get(key) || null)
    })

    const { result } = renderHook(() =>
      useChatStorage({
        sessionId: 1,
        setMessages,
        setError,
      }),
    )

    await result.current.loadMessages()

    expect(setMessages).toHaveBeenCalledWith([
      expect.objectContaining({ chatID: 'old', role: 'user' }),
      expect.objectContaining({ chatID: 'inserted', role: 'user' }),
      expect.objectContaining({ chatID: 'inserted', role: 'assistant' }),
      expect.objectContaining({ chatID: 'old', role: 'assistant' }),
    ])
  })

  it('should abort loading if sessionId changes during fetch (race condition)', async () => {
    let resolveFetch: (val: unknown) => void
    const fetchPromise = new Promise((resolve) => {
      resolveFetch = resolve
    })

    ;(kvGet as Mock).mockImplementation((key: string) => {
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

  it('should abort stale load when session changes during per-item fetch', async () => {
    const userFetch = deferred<unknown>()

    ;(kvGet as Mock).mockImplementation((key: string) => {
      if (key === 'chat_user_session_1') {
        return Promise.resolve([
          { chatID: 'chat1', role: 'user', content: 'a' },
        ])
      }
      if (key === 'chat_data_user_chat1') {
        return userFetch.promise
      }
      if (key === 'chat_data_assistant_chat1') {
        return Promise.resolve({
          chatID: 'chat1',
          role: 'assistant',
          content: 'reply',
        })
      }
      return Promise.resolve(null)
    })

    const { result, rerender } = renderHook(
      ({ sessionId }) => useChatStorage({ sessionId, setMessages, setError }),
      { initialProps: { sessionId: 1 } },
    )

    const loadPromise = result.current.loadMessages()
    rerender({ sessionId: 2 })

    userFetch.resolve({
      chatID: 'chat1',
      role: 'user',
      content: 'session 1 msg',
    })
    await loadPromise

    expect(setMessages).not.toHaveBeenCalledWith(
      expect.arrayContaining([
        expect.objectContaining({ content: 'session 1 msg' }),
      ]),
    )
  })

  it('should not overwrite newer local updates from an in-flight load', async () => {
    const historyFetch = deferred<unknown>()

    ;(kvGet as Mock).mockImplementation((key: string) => {
      if (key === 'chat_user_session_1') {
        return historyFetch.promise
      }
      return Promise.resolve(null)
    })

    const { result } = renderHook(() =>
      useChatStorage({
        sessionId: 1,
        setMessages,
        setError,
      }),
    )

    const loadPromise = result.current.loadMessages()
    expect(setMessages).not.toHaveBeenCalled()

    const savePromise = result.current.saveMessage({
      chatID: 'chat-local',
      role: 'user',
      content: 'local update',
    } as ChatMessageData)

    historyFetch.resolve([])
    await savePromise
    await loadPromise

    expect(setMessages).not.toHaveBeenCalled()
  })

  it('should keep existing UI messages when stale load is invalidated', async () => {
    const historyFetch = deferred<unknown>()
    let historyGetCount = 0

    ;(kvGet as Mock).mockImplementation((key: string) => {
      if (key === 'chat_user_session_1') {
        historyGetCount += 1
        if (historyGetCount === 1) {
          return historyFetch.promise
        }
        return Promise.resolve([])
      }
      return Promise.resolve(null)
    })

    const { result } = renderHook(() =>
      useChatStorage({
        sessionId: 1,
        setMessages,
        setError,
      }),
    )

    const loadPromise = result.current.loadMessages()

    await result.current.saveMessage({
      chatID: 'chat-fresh',
      role: 'user',
      content: 'fresh message',
    } as ChatMessageData)

    historyFetch.resolve([{ chatID: 'old', role: 'user', content: 'old' }])
    await loadPromise

    expect(setMessages).not.toHaveBeenCalledWith([])
    expect(setMessages).not.toHaveBeenCalledWith(
      expect.arrayContaining([
        expect.objectContaining({ chatID: 'old', content: 'old' }),
      ]),
    )
  })

  it('should not invalidate session load by mutations from another session', async () => {
    const historyFetchSession2 = deferred<unknown>()

    ;(kvGet as Mock).mockImplementation((key: string) => {
      if (key === 'chat_user_session_2') {
        return historyFetchSession2.promise
      }

      if (key === 'chat_data_user_chat2') {
        return Promise.resolve({
          chatID: 'chat2',
          role: 'user',
          content: 'session2 message',
        })
      }

      if (key === 'chat_data_assistant_chat2') {
        return Promise.resolve(null)
      }

      if (key === 'chat_data_user_chat1') {
        return Promise.resolve(null)
      }

      if (key === 'chat_user_session_1') {
        return Promise.resolve([])
      }

      return Promise.resolve(null)
    })

    const hookSession1 = renderHook(() =>
      useChatStorage({
        sessionId: 1,
        setMessages: setMessagesAlt,
        setError,
      }),
    )

    const hookSession2 = renderHook(() =>
      useChatStorage({
        sessionId: 2,
        setMessages,
        setError,
      }),
    )

    const loadPromise = hookSession2.result.current.loadMessages()

    // Trigger a mutation in another session while session 2 is loading.
    await hookSession1.result.current.saveMessage({
      chatID: 'chat1',
      role: 'user',
      content: 'session1 write',
    } as ChatMessageData)

    historyFetchSession2.resolve([
      { chatID: 'chat2', role: 'user', content: 'session2 message' },
    ])
    await loadPromise

    expect(setMessages).toHaveBeenCalledWith(
      expect.arrayContaining([
        expect.objectContaining({
          chatID: 'chat2',
          content: 'session2 message',
        }),
      ]),
    )
  })

  it('should insert a pair before the requested history anchor', async () => {
    const history = [
      { chatID: 'first', role: 'user' as const, content: 'first' },
      { chatID: 'first', role: 'assistant' as const, content: 'reply' },
      { chatID: 'second', role: 'user' as const, content: 'second' },
    ]
    ;(kvGet as Mock).mockImplementation((key: string) => {
      if (key === 'chat_user_session_1') return Promise.resolve(history)
      return Promise.resolve(null)
    })

    const { result } = renderHook(() =>
      useChatStorage({
        sessionId: 1,
        setMessages,
        setError,
      }),
    )

    await result.current.insertMessagePair(
      { chatID: 'inserted', role: 'user', content: 'inserted question' },
      { chatID: 'inserted', role: 'assistant', content: '' },
      { chatID: 'second', role: 'user' },
    )

    expect(kvSet).toHaveBeenLastCalledWith('chat_user_session_1', [
      history[0],
      history[1],
      {
        chatID: 'inserted',
        role: 'user',
        content: 'inserted question',
        model: undefined,
        timestamp: undefined,
      },
      {
        chatID: 'inserted',
        role: 'assistant',
        content: '',
        model: undefined,
        timestamp: undefined,
      },
      history[2],
    ])
  })
})
