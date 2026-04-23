import { renderHook, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { ChatMessageData } from '../../types'
import { useChatScroll } from '../use-chat-scroll'

let scrollTopValue = 0
let scrollHeightValue = 0
let bodyScrollHeightValue = 0
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
  bodyScrollHeight?: number,
) => {
  scrollTopValue = scrollTop
  scrollHeightValue = scrollHeight
  bodyScrollHeightValue = bodyScrollHeight ?? scrollHeight
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
      get: () => bodyScrollHeightValue,
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

    // Lock viewport to simulate being in a non-auto-follow state
    result.current.lockViewport()

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

    // Put in viewport-locked mode so auto-scroll doesn't interfere
    result.current.lockViewport()

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

    // Wheel event transitions from auto-follow to user-scrolled
    window.dispatchEvent(new Event('wheel'))

    expect(result.current.scrollModeRef.current).toBe('user-scrolled')

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

    result.current.scrollModeRef.current = 'user-scrolled'

    const scrollToSpy = window.scrollTo as unknown as ReturnType<typeof vi.fn>
    scrollToSpy.mockClear()

    result.current.scrollToBottom({ force: true, behavior: 'auto' })

    expect(result.current.scrollModeRef.current).toBe('auto-follow')

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

    // Simulate being scrolled down and in user-scrolled mode
    setScrollMetrics(800, 2000, 500)
    result.current.scrollModeRef.current = 'user-scrolled'

    const scrollToSpy = window.scrollTo as unknown as ReturnType<typeof vi.fn>
    scrollToSpy.mockClear()

    // Clear messages
    rerender({ messages: [] })

    await waitFor(() => {
      expect(scrollToSpy).toHaveBeenCalledWith({ top: 0, behavior: 'auto' })
      expect(result.current.scrollModeRef.current).toBe('auto-follow')
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
    expect(result.current.scrollModeRef.current).toBe('auto-follow')

    const scrollToSpy = window.scrollTo as unknown as ReturnType<typeof vi.fn>
    scrollToSpy.mockClear()

    // Repopulate messages
    rerender({ messages: buildMessages(5) })

    await waitFor(() => {
      // Should auto-scroll to bottom of the new messages
      expect(scrollToSpy).toHaveBeenCalled()
      expect(result.current.scrollModeRef.current).toBe('auto-follow')
    })
  })

  it('does not overshoot when body.scrollHeight is stale after session switch', async () => {
    // Regression: switching sessions could cause body.scrollHeight to retain
    // a stale larger value from the previous session, making scrollToBottom
    // overshoot past the actual content.
    const { rerender } = renderHook(
      ({ sessionId, messages }) =>
        useChatScroll({ messages, pageSize: 40, sessionId }),
      {
        initialProps: {
          sessionId: 1,
          messages: buildMessages(50),
        },
      },
    )

    // Session 1 had a tall document.
    // documentElement.scrollHeight = 800 (new session), but body.scrollHeight
    // is stale at 5000 (old session).
    setScrollMetrics(0, 800, 500, /* bodyScrollHeight */ 5000)

    const scrollToSpy = window.scrollTo as unknown as ReturnType<typeof vi.fn>
    scrollToSpy.mockClear()

    // Switch to session 2 with fewer messages.
    rerender({ sessionId: 2, messages: [] })
    setScrollMetrics(0, 800, 500, 5000)
    rerender({ sessionId: 2, messages: buildMessages(3) })

    await waitFor(() => {
      // The scroll position should be based on the actual scrollHeight (800),
      // not the stale body.scrollHeight (5000).
      // Max scroll = 800 - 500 = 300.  Must never exceed this.
      const calls = scrollToSpy.mock.calls as Array<[{ top: number }]>
      const maxTopSeen = Math.max(...calls.map((c) => c[0]?.top ?? 0))
      expect(maxTopSeen).toBeLessThanOrEqual(300)
    })
  })

  it('clamps scroll when content-end marker is above viewport', async () => {
    // Regression: if scrollHeight is inflated, scrollToBottom might place
    // the content-end marker above the visible viewport, showing blank
    // space.  The clampScrollPosition should bring it back.

    const { rerender } = renderHook(
      ({ sessionId, messages }) =>
        useChatScroll({ messages, pageSize: 40, sessionId }),
      {
        initialProps: {
          sessionId: 1,
          messages: buildMessages(10),
        },
      },
    )

    // Simulate switching to a shorter session.
    setScrollMetrics(0, 600, 500)
    rerender({ sessionId: 2, messages: [] })
    rerender({ sessionId: 2, messages: buildMessages(2) })

    await waitFor(() => {
      // scrollHeight (600) - clientHeight (500) = 100.
      // Scroll must not exceed 100.
      expect(window.scrollY).toBeLessThanOrEqual(100)
    })
  })

  it('viewport-locked mode survives content-change scroll events', async () => {
    // Core regression test: after edit/regenerate, content changes should
    // NOT re-enable auto-scroll. This was the root cause of the positioning bug.
    const { result, rerender } = renderHook(
      ({ messages }) => useChatScroll({ messages, pageSize: 40, sessionId: 1 }),
      {
        initialProps: {
          messages: buildMessages(10),
        },
      },
    )

    const scrollToSpy = window.scrollTo as unknown as ReturnType<typeof vi.fn>

    // Simulate edit/regenerate: lock viewport
    result.current.lockViewport()
    expect(result.current.scrollModeRef.current).toBe('viewport-locked')

    scrollToSpy.mockClear()

    // Simulate content change (assistant message cleared then streaming)
    setScrollMetrics(0, 800, 500) // Content shrunk, now at bottom
    rerender({ messages: buildMessages(9) }) // Message removed

    // Dispatch a passive scroll event (as would happen from clamp/resize)
    window.dispatchEvent(new Event('scroll'))

    // Mode must still be viewport-locked
    expect(result.current.scrollModeRef.current).toBe('viewport-locked')

    // No auto-scroll should have happened
    expect(scrollToSpy).not.toHaveBeenCalled()
  })

  it('viewport-locked mode transitions to user-scrolled on wheel', async () => {
    const { result } = renderHook(
      ({ messages }) => useChatScroll({ messages, pageSize: 40, sessionId: 1 }),
      {
        initialProps: {
          messages: buildMessages(10),
        },
      },
    )

    result.current.lockViewport()
    expect(result.current.scrollModeRef.current).toBe('viewport-locked')

    // User takes control via wheel
    window.dispatchEvent(new Event('wheel'))

    expect(result.current.scrollModeRef.current).toBe('user-scrolled')
  })

  it('viewport-locked mode transitions to auto-follow on force scroll', async () => {
    const { result } = renderHook(
      ({ messages }) => useChatScroll({ messages, pageSize: 40, sessionId: 1 }),
      {
        initialProps: {
          messages: buildMessages(10),
        },
      },
    )

    result.current.lockViewport()
    expect(result.current.scrollModeRef.current).toBe('viewport-locked')

    // Force scroll (send message / click scroll-to-bottom)
    result.current.scrollToBottom({ force: true })

    expect(result.current.scrollModeRef.current).toBe('auto-follow')
  })

  it('transitions to user-scrolled when the user drags the scrollbar upward', async () => {
    // Regression: scrollbar drag emits a `scroll` event but no wheel / touch /
    // keyboard event, so the hook used to stay in auto-follow and keep
    // auto-scrolling back to the bottom on every streaming update.  After the
    // fix, the scroll handler itself detects the unexplained upward scroll
    // and hands control back to the user.
    const { result, rerender } = renderHook(
      ({ messages }) => useChatScroll({ messages, pageSize: 40, sessionId: 1 }),
      {
        initialProps: {
          messages: buildMessages(2),
        },
      },
    )

    setScrollMetrics(800, 2000, 500)
    const scrollToSpy = window.scrollTo as unknown as ReturnType<typeof vi.fn>

    // Baseline the internal lastScrollTop tracker to 800 via an initial scroll
    // event (simulates an earlier programmatic or user scroll settling there).
    window.dispatchEvent(new Event('scroll'))
    expect(result.current.scrollModeRef.current).toBe('auto-follow')

    // Now the user drags the scrollbar upward: scrollTop decreases without
    // any wheel / touch / keyboard event firing.
    setScrollMetrics(400, 2000, 500)
    window.dispatchEvent(new Event('scroll'))

    expect(result.current.scrollModeRef.current).toBe('user-scrolled')

    scrollToSpy.mockClear()

    // Subsequent streaming updates must NOT force us back to the bottom.
    rerender({ messages: buildMessages(3) })
    await waitFor(() => {
      expect(scrollToSpy).not.toHaveBeenCalled()
    })
  })

  it('does not treat programmatic scrolls as user upward scrolls', async () => {
    // When the hook itself scrolls (session switch, clamp after resize), the
    // scroll event that follows must not flip the mode to user-scrolled just
    // because scrollTop decreased.
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

    setScrollMetrics(600, 1200, 500)
    window.dispatchEvent(new Event('scroll'))

    // Switch to a new session: the hook programmatically scrolls to 0.  The
    // ensuing scroll event observes a large downward delta, which must NOT
    // be interpreted as user intent.
    rerender({ sessionId: 2, messages: buildMessages(2) })
    // Simulate the browser's scroll event post-scrollTo.
    window.dispatchEvent(new Event('scroll'))

    expect(result.current.scrollModeRef.current).toBe('auto-follow')
  })

  it('respects a concurrent user scroll that fires between scheduling and running the auto-follow rAF', async () => {
    // Regression: the old implementation used `{ force: true }` inside the
    // messages-change auto-scroll effect, which unconditionally reset mode to
    // auto-follow even if the user had just wheeled away.  The rAF would then
    // scroll to bottom anyway.
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
    scrollToSpy.mockClear()

    // After the initial render settles, switch rAF to a queued implementation
    // so we can interleave a user scroll before the rAF fires.
    const rafCallbacks: FrameRequestCallback[] = []
    ;(
      window.requestAnimationFrame as unknown as ReturnType<typeof vi.fn>
    ).mockImplementation((cb: FrameRequestCallback) => {
      rafCallbacks.push(cb)
      return 0
    })

    // Trigger the messages-change effect, which schedules an auto-scroll rAF.
    rerender({ messages: buildMessages(3) })

    // Before the rAF runs, the user wheels (transitions to user-scrolled).
    window.dispatchEvent(new Event('wheel'))
    expect(result.current.scrollModeRef.current).toBe('user-scrolled')

    // Now drain any queued rAFs.  They must detect the mode change and skip
    // the programmatic scroll.
    while (rafCallbacks.length > 0) {
      const cb = rafCallbacks.shift()!
      cb(0)
    }

    expect(scrollToSpy).not.toHaveBeenCalled()
  })

  it('anchors auto-follow scrolling to the streaming message element', async () => {
    // The "reload/edit causes wrong scroll position" bug: when a mid-
    // conversation message is being streamed/regenerated, auto-scrolling to
    // messagesEndRef (the end of the list) overshoots past the growing
    // content to the unchanged final message.  With streamingChatId, the
    // anchor is the streaming message's bottom.
    const streamingId = 'streaming-target'
    const otherId = 'final-message'
    const streamingEl = document.createElement('div')
    streamingEl.id = `chat-message-${streamingId}-assistant`
    Object.defineProperty(streamingEl, 'getBoundingClientRect', {
      value: () => ({
        top: 100,
        bottom: 300,
        left: 0,
        right: 0,
        width: 0,
        height: 200,
        x: 0,
        y: 100,
        toJSON: () => ({}),
      }),
      configurable: true,
    })
    document.body.appendChild(streamingEl)

    // The messagesEndRef would point well past the streaming message, since
    // there's another (unchanged) message after it.  If the hook used that
    // anchor it would scroll to a much larger target.
    const endEl = document.createElement('div')
    endEl.id = `chat-message-${otherId}-assistant`
    document.body.appendChild(endEl)

    try {
      // Viewport: clientHeight 500, scrollTop 0, footer 100.
      // Streaming message bottom is at document-Y 0 + 300 = 300.
      // Expected contentMax = 300 - 500 + 100 + 8 = max(-92, 0) = 0.
      setScrollMetrics(0, 2000, 500)

      const { rerender } = renderHook(
        ({ messages }) =>
          useChatScroll({
            messages,
            pageSize: 40,
            sessionId: 1,
            streamingChatId: streamingId,
            footerHeight: 100,
          }),
        {
          initialProps: {
            messages: buildMessages(10),
          },
        },
      )

      const scrollToSpy = window.scrollTo as unknown as ReturnType<typeof vi.fn>
      scrollToSpy.mockClear()

      // Simulate a streaming content update.
      rerender({ messages: buildMessages(11) })

      await waitFor(() => {
        expect(scrollToSpy).toHaveBeenCalled()
      })

      // The target must be computed from the streaming message's bottom (300)
      // and clamped to the docMax window, NOT the unchanged tail (2000-500=1500).
      const calls = scrollToSpy.mock.calls as Array<[{ top: number }]>
      const maxTopSeen = Math.max(...calls.map((c) => c[0]?.top ?? 0))
      expect(maxTopSeen).toBeLessThanOrEqual(8)
    } finally {
      streamingEl.remove()
      endEl.remove()
    }
  })

  it('falls back to messagesEndRef when streamingChatId element is not mounted', async () => {
    // If streamingChatId refers to a message that hasn't yet been rendered,
    // the hook falls back to the end-of-list marker so the user still sees
    // auto-follow behaviour during normal (end-of-list) streaming.
    setScrollMetrics(0, 1200, 500)

    const { rerender } = renderHook(
      ({ messages }) =>
        useChatScroll({
          messages,
          pageSize: 40,
          sessionId: 1,
          streamingChatId: 'not-mounted',
        }),
      {
        initialProps: {
          messages: buildMessages(1),
        },
      },
    )

    const scrollToSpy = window.scrollTo as unknown as ReturnType<typeof vi.fn>
    scrollToSpy.mockClear()

    setScrollMetrics(0, 1400, 500)
    rerender({ messages: buildMessages(2) })

    await waitFor(() => {
      expect(scrollToSpy).toHaveBeenCalled()
    })
  })
})
