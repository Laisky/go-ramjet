import { renderHook, act } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { useChatScroll } from '@/pages/gptchat/hooks/use-chat-scroll'
import type { ChatMessageData } from '@/pages/gptchat/types'

/**
 * createMessage builds a minimal chat message for hook testing.
 */
function createMessage(id: string, role: ChatMessageData['role']): ChatMessageData {
  return {
    chatID: id,
    role,
    content: 'hello',
  }
}

/**
 * setupWindowScroll stubs window scroll metrics for deterministic scroll tests.
 */
function setupWindowScroll(initial: {
  scrollTop: number
  scrollHeight: number
  clientHeight: number
}) {
  let scrollTop = initial.scrollTop
  let scrollHeight = initial.scrollHeight
  let clientHeight = initial.clientHeight

  Object.defineProperty(document, 'scrollingElement', {
    value: document.documentElement,
    configurable: true,
  })
  Object.defineProperty(window, 'scrollY', {
    get: () => scrollTop,
    set: (value: number) => {
      scrollTop = value
    },
    configurable: true,
  })
  Object.defineProperty(window, 'pageYOffset', {
    get: () => scrollTop,
    configurable: true,
  })
  Object.defineProperty(window, 'innerHeight', {
    get: () => clientHeight,
    configurable: true,
  })
  Object.defineProperty(document.documentElement, 'scrollHeight', {
    get: () => scrollHeight,
    configurable: true,
  })
  Object.defineProperty(document.body, 'scrollHeight', {
    get: () => scrollHeight,
    configurable: true,
  })
  Object.defineProperty(document.documentElement, 'clientHeight', {
    get: () => clientHeight,
    configurable: true,
  })
  Object.defineProperty(document.documentElement, 'scrollTop', {
    get: () => 0,
    configurable: true,
  })
  Object.defineProperty(document.body, 'scrollTop', {
    get: () => 0,
    configurable: true,
  })

  const scrollTo = vi.fn((options: ScrollToOptions | number, y?: number) => {
    if (typeof options === 'number') {
      scrollTop = y ?? 0
      return
    }
    if (options && typeof options === 'object') {
      scrollTop = options.top ?? scrollTop
    }
  })
  vi.stubGlobal('scrollTo', scrollTo)

  return {
    setScrollTop: (value: number) => {
      scrollTop = value
    },
    setScrollHeight: (value: number) => {
      scrollHeight = value
    },
    setClientHeight: (value: number) => {
      clientHeight = value
    },
    scrollTo,
  }
}

/**
 * setupAnimationFrames queues requestAnimationFrame callbacks for manual flushing.
 */
function setupAnimationFrames() {
  const frameQueue: FrameRequestCallback[] = []

  vi.stubGlobal('requestAnimationFrame', (cb: FrameRequestCallback) => {
    frameQueue.push(cb)
    return frameQueue.length
  })

  return {
    runAllFrames: () => {
      while (frameQueue.length > 0) {
        const cb = frameQueue.shift()
        cb?.(0)
      }
    },
  }
}

describe('useChatScroll', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('clamps scroll position when content shrinks', () => {
    const { runAllFrames } = setupAnimationFrames()
    const scroll = setupWindowScroll({
      scrollTop: 250,
      scrollHeight: 300,
      clientHeight: 200,
    })

    const messages = [createMessage('1', 'assistant')]
    renderHook(() =>
      useChatScroll({ messages, pageSize: 20, sessionId: 'session-1' }),
    )

    runAllFrames()

    const calledWithClamp = scroll.scrollTo.mock.calls.some(([arg]) => {
      if (typeof arg === 'object' && arg) {
        return arg.top === 100
      }
      return false
    })

    expect(calledWithClamp).toBe(true)
  })

  it('uses window scroll metrics when checking bottom', () => {
    const { runAllFrames } = setupAnimationFrames()
    setupWindowScroll({
      scrollTop: 190,
      scrollHeight: 300,
      clientHeight: 100,
    })

    const messages = [createMessage('1', 'assistant')]
    const { result } = renderHook(() =>
      useChatScroll({ messages, pageSize: 20, sessionId: 'session-2' }),
    )

    runAllFrames()

    expect(result.current.isNearBottom()).toBe(true)
  })

  it('keeps viewport anchored when loading older messages', () => {
    const { runAllFrames } = setupAnimationFrames()
    const scroll = setupWindowScroll({
      scrollTop: 120,
      scrollHeight: 400,
      clientHeight: 200,
    })

    const messages = Array.from({ length: 60 }, (_, idx) =>
      createMessage(`${idx}`, 'assistant'),
    )
    const { result } = renderHook(() =>
      useChatScroll({ messages, pageSize: 20, sessionId: 'session-3' }),
    )

    runAllFrames()
    scroll.setScrollTop(120)
    scroll.setScrollHeight(400)
    scroll.scrollTo.mockClear()

    act(() => {
      result.current.handleLoadOlder()
      scroll.setScrollHeight(520)
    })

    runAllFrames()

    const lastCallArg = scroll.scrollTo.mock.calls.at(-1)?.[0]
    expect(lastCallArg).toMatchObject({ top: 240 })
  })
})
