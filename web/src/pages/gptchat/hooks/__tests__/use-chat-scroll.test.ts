import { renderHook, waitFor } from '@testing-library/react'
import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest'

import { useChatScroll } from '../use-chat-scroll'
import type { ChatMessageData } from '../../types'

const buildMessages = (count: number): ChatMessageData[] => {
  return Array.from({ length: count }, (_, i) => ({
    chatID: `chat-${i}`,
    role: i % 2 === 0 ? 'user' : 'assistant',
    content: `message ${i}`,
  }))
}

const setScrollMetrics = (
  scrollTop: number,
  scrollHeight: number,
  clientHeight: number,
) => {
  Object.defineProperty(document.documentElement, 'scrollTop', {
    value: scrollTop,
    writable: true,
    configurable: true,
  })
  Object.defineProperty(document.documentElement, 'scrollHeight', {
    value: scrollHeight,
    writable: true,
    configurable: true,
  })
  Object.defineProperty(document.documentElement, 'clientHeight', {
    value: clientHeight,
    writable: true,
    configurable: true,
  })
}

describe('useChatScroll', () => {
  beforeEach(() => {
    setScrollMetrics(0, 1000, 500)
    Object.defineProperty(document.documentElement, 'scrollTo', {
      value: vi.fn((options: { top?: number }) => {
        const top = options?.top ?? 0
        document.documentElement.scrollTop = top
      }),
      writable: true,
      configurable: true,
    })
    Object.defineProperty(document.body, 'scrollTop', {
      value: 0,
      writable: true,
      configurable: true,
    })
    vi.spyOn(window, 'requestAnimationFrame').mockImplementation((cb) => {
      cb(0)
      return 0
    })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('resets scroll position when session changes', async () => {
    const { result, rerender } = renderHook(
      ({ sessionId, messages }) =>
        useChatScroll({ messages, pageSize: 40, sessionId }),
      {
        initialProps: {
          sessionId: 1,
          messages: buildMessages(10),
        },
      },
    )

    setScrollMetrics(800, 2000, 500)

    result.current.suppressAutoScrollOnceRef.current = true

    rerender({ sessionId: 2, messages: buildMessages(3) })

    const scrollToSpy = document.documentElement
      .scrollTo as unknown as ReturnType<typeof vi.fn>

    await waitFor(() => {
      expect(scrollToSpy).toHaveBeenCalledWith({ top: 0, behavior: 'auto' })
    })
  })

  it('clamps scroll position when content shrinks', async () => {
    const { result, rerender } = renderHook(
      ({ messages }) => useChatScroll({ messages, pageSize: 40, sessionId: 1 }),
      {
        initialProps: {
          messages: buildMessages(50),
        },
      },
    )

    result.current.autoScrollRef.current = false
    result.current.suppressAutoScrollOnceRef.current = true

    setScrollMetrics(900, 1000, 400)

    rerender({ messages: buildMessages(2) })

    await waitFor(() => {
      expect(document.documentElement.scrollTop).toBe(600)
    })
  })
})
