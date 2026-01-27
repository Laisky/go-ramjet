import { renderHook, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { ChatMessageData } from '../../types'
import { useChatScroll } from '../use-chat-scroll'

let scrollTopValue = 0
let scrollHeightValue = 0
let clientHeightValue = 0
let resizeObserverCallback: ResizeObserverCallback | null = null
let resizeObserverInstance: ResizeObserver | null = null

/**
 * buildMessages creates mock chat messages for scroll hook tests.
 */
const buildMessages = (count: number): ChatMessageData[] => {
  return Array.from({ length: count }, (_, i) => ({
    chatID: `chat-${i}`,
    role: i % 2 === 0 ? 'user' : 'assistant',
    content: `message ${i}`,
  }))
}

/**
 * setScrollMetrics updates the mocked scroll metrics for the window.
 */
const setScrollMetrics = (
  scrollTop: number,
  scrollHeight: number,
  clientHeight: number,
) => {
  scrollTopValue = scrollTop
  scrollHeightValue = scrollHeight
  clientHeightValue = clientHeight
}

describe('useChatScroll', () => {
  beforeEach(() => {
    setScrollMetrics(0, 1000, 500)
    resizeObserverCallback = null
    resizeObserverInstance = null
    // Both documentElement.scrollTo and window.scrollTo are used
    const scrollToMock = vi.fn((options: { top?: number }) => {
      const top = options?.top ?? 0
      scrollTopValue = top
    })

    Object.defineProperty(window, 'scrollY', {
      get: () => scrollTopValue,
      set: (value: number) => {
        scrollTopValue = value
      },
      configurable: true,
    })
    Object.defineProperty(window, 'pageYOffset', {
      get: () => scrollTopValue,
      configurable: true,
    })
    Object.defineProperty(window, 'innerHeight', {
      get: () => clientHeightValue,
      configurable: true,
    })
    Object.defineProperty(document.documentElement, 'scrollTo', {
      value: scrollToMock,
      writable: true,
      configurable: true,
    })
    Object.defineProperty(window, 'scrollTo', {
      value: scrollToMock,
      writable: true,
      configurable: true,
    })
    Object.defineProperty(document.body, 'scrollTop', {
      get: () => scrollTopValue,
      set: (value: number) => {
        scrollTopValue = value
      },
      configurable: true,
    })
    Object.defineProperty(document.documentElement, 'scrollTop', {
      get: () => scrollTopValue,
      set: (value: number) => {
        scrollTopValue = value
      },
      configurable: true,
    })
    Object.defineProperty(document.documentElement, 'scrollHeight', {
      get: () => scrollHeightValue,
      configurable: true,
    })
    Object.defineProperty(document.body, 'scrollHeight', {
      get: () => scrollHeightValue,
      configurable: true,
    })
    Object.defineProperty(document.documentElement, 'clientHeight', {
      get: () => clientHeightValue,
      configurable: true,
    })
    vi.spyOn(window, 'requestAnimationFrame').mockImplementation((cb) => {
      cb(0)
      return 0
    })
    vi.stubGlobal(
      'ResizeObserver',
      class {
        constructor(callback: ResizeObserverCallback) {
          resizeObserverCallback = callback
          resizeObserverInstance = this as unknown as ResizeObserver
        }
        observe() {
          return undefined
        }
        disconnect() {
          return undefined
        }
        unobserve() {
          return undefined
        }
      },
    )
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

    const scrollToSpy = window.scrollTo as unknown as ReturnType<typeof vi.fn>

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
      expect(window.scrollY).toBe(600)
    })
  })

  it('auto-scrolls to bottom when new messages arrive and auto-follow is enabled', async () => {
    const { rerender } = renderHook(
      ({ messages }) => useChatScroll({ messages, pageSize: 40, sessionId: 1 }),
      {
        initialProps: {
          messages: buildMessages(1),
        },
      },
    )

    setScrollMetrics(0, 1000, 500)

    const scrollToSpy = window.scrollTo as unknown as ReturnType<typeof vi.fn>
    scrollToSpy.mockClear()

    setScrollMetrics(0, 1200, 500)
    rerender({ messages: buildMessages(2) })

    await waitFor(() => {
      expect(scrollToSpy).toHaveBeenCalledWith({ top: 700, behavior: 'auto' })
    })
  })

  it('stops auto-follow immediately on manual scroll input', async () => {
    const { result, rerender } = renderHook(
      ({ messages }) => useChatScroll({ messages, pageSize: 40, sessionId: 1 }),
      {
        initialProps: {
          messages: buildMessages(2),
        },
      },
    )

    setScrollMetrics(0, 2000, 500)

    const scrollToSpy = window.scrollTo as unknown as ReturnType<typeof vi.fn>

    await waitFor(() => {
      expect(scrollToSpy).toHaveBeenCalled()
    })

    scrollToSpy.mockClear()

    window.dispatchEvent(new Event('wheel'))

    expect(result.current.autoScrollRef.current).toBe(false)

    rerender({ messages: buildMessages(3) })

    await waitFor(() => {
      expect(scrollToSpy).not.toHaveBeenCalled()
    })
  })

  it('re-enables auto-follow when forced scrollToBottom is used', async () => {
    const { result, rerender } = renderHook(
      ({ messages }) => useChatScroll({ messages, pageSize: 40, sessionId: 1 }),
      {
        initialProps: {
          messages: buildMessages(2),
        },
      },
    )

    result.current.autoScrollRef.current = false

    const scrollToSpy = window.scrollTo as unknown as ReturnType<typeof vi.fn>
    scrollToSpy.mockClear()

    result.current.scrollToBottom({ force: true, behavior: 'auto' })

    expect(result.current.autoScrollRef.current).toBe(true)

    setScrollMetrics(0, 1500, 500)
    rerender({ messages: buildMessages(3) })

    await waitFor(() => {
      expect(scrollToSpy).toHaveBeenCalled()
    })
  })

  it('scrolls to bottom after session load even when auto-follow was disabled', async () => {
    const { rerender } = renderHook(
      ({ sessionId, messages }) =>
        useChatScroll({ messages, pageSize: 40, sessionId }),
      {
        initialProps: {
          sessionId: 1,
          messages: buildMessages(2),
        },
      },
    )

    setScrollMetrics(0, 1000, 500)

    const scrollToSpy = window.scrollTo as unknown as ReturnType<typeof vi.fn>
    scrollToSpy.mockClear()

    rerender({ sessionId: 2, messages: [] })

    setScrollMetrics(0, 800, 500)
    rerender({ sessionId: 2, messages: buildMessages(3) })

    await waitFor(() => {
      expect(scrollToSpy).toHaveBeenCalledWith({ top: 300, behavior: 'auto' })
    })
  })

  it('clamps scroll position when content height shrinks without message count change', async () => {
    renderHook(
      ({ messages }) => useChatScroll({ messages, pageSize: 40, sessionId: 1 }),
      {
        initialProps: {
          messages: buildMessages(5),
        },
      },
    )

    setScrollMetrics(600, 1000, 400)

    setScrollMetrics(600, 700, 400)

    if (resizeObserverCallback && resizeObserverInstance) {
      resizeObserverCallback([], resizeObserverInstance)
    }

    await waitFor(() => {
      expect(window.scrollY).toBe(300)
    })
  })

  it('resets scroll and enables auto-follow when messages are cleared within the same session', async () => {
    const { result, rerender } = renderHook(
      ({ messages }) => useChatScroll({ messages, pageSize: 40, sessionId: 1 }),
      {
        initialProps: {
          messages: buildMessages(10),
        },
      },
    )

    // Simulate being scrolled down and auto-follow disabled
    setScrollMetrics(800, 2000, 500)
    result.current.autoScrollRef.current = false

    const scrollToSpy = window.scrollTo as unknown as ReturnType<typeof vi.fn>
    scrollToSpy.mockClear()

    // Clear messages
    rerender({ messages: [] })

    await waitFor(() => {
      expect(scrollToSpy).toHaveBeenCalledWith({ top: 0, behavior: 'auto' })
      expect(result.current.autoScrollRef.current).toBe(true)
    })
  })

  it('maintains auto-follow if messages are cleared and then immediately repopulated', async () => {
    const { result, rerender } = renderHook(
      ({ messages }) => useChatScroll({ messages, pageSize: 40, sessionId: 1 }),
      {
        initialProps: {
          messages: buildMessages(10),
        },
      },
    )

    // Clear messages
    rerender({ messages: [] })

    // Check if auto-follow is enabled
    expect(result.current.autoScrollRef.current).toBe(true)

    const scrollToSpy = window.scrollTo as unknown as ReturnType<typeof vi.fn>
    scrollToSpy.mockClear()

    // Repopulate messages
    rerender({ messages: buildMessages(5) })

    await waitFor(() => {
      // Should auto-scroll to bottom of the new messages
      expect(scrollToSpy).toHaveBeenCalled()
      expect(result.current.autoScrollRef.current).toBe(true)
    })
  })
})
