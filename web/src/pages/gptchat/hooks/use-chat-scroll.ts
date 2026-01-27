import { useCallback, useEffect, useRef, useState } from 'react'
import type { ChatMessageData } from '../types'

interface UseChatScrollOptions {
  messages: ChatMessageData[]
  pageSize: number
  sessionId: string | number
  contentRef?: React.RefObject<HTMLElement | null>
}

/**
 * useChatScroll manages scrolling behavior for the chat interface.
 */
export function useChatScroll({
  messages,
  pageSize,
  sessionId,
  contentRef,
}: UseChatScrollOptions) {
  const messagesEndRef = useRef<HTMLDivElement>(null)
  const [showScrollButton, setShowScrollButton] = useState(false)
  const [visibleCount, setVisibleCount] = useState(pageSize)
  const autoScrollRef = useRef(true)
  const suppressAutoScrollOnceRef = useRef(false)
  const manualScrollRef = useRef(false)
  const pendingSessionScrollRef = useRef(false)

  const getScrollElement = useCallback(() => {
    return document.scrollingElement || document.documentElement
  }, [])

  /**
   * getScrollMetrics returns the current scroll metrics for the active scroll container.
   */
  const getScrollMetrics = useCallback(() => {
    const scrollElement = getScrollElement()

    if (
      scrollElement === document.documentElement ||
      scrollElement === document.body
    ) {
      const doc = document.documentElement
      const body = document.body
      const scrollTop =
        window.scrollY ||
        window.pageYOffset ||
        doc.scrollTop ||
        body.scrollTop ||
        0
      const scrollHeight = Math.max(
        doc.scrollHeight,
        body?.scrollHeight || 0,
        scrollElement.scrollHeight || 0,
      )
      const clientHeight = window.innerHeight || doc.clientHeight

      return { scrollTop, scrollHeight, clientHeight }
    }

    return {
      scrollTop: scrollElement.scrollTop,
      scrollHeight: scrollElement.scrollHeight,
      clientHeight: scrollElement.clientHeight,
    }
  }, [getScrollElement])

  const scrollToPosition = useCallback(
    (top: number, behavior: ScrollBehavior) => {
      const scrollElement = getScrollElement()

      // Use window.scrollTo for the main document to ensure consistent behavior across browsers
      if (
        scrollElement === document.documentElement ||
        scrollElement === document.body
      ) {
        const isJsdom =
          typeof navigator !== 'undefined' &&
          navigator.userAgent.includes('jsdom')
        const isMockedScrollTo =
          typeof window.scrollTo === 'function' && 'mock' in window.scrollTo
        if (isJsdom && !isMockedScrollTo) {
          try {
            scrollElement.scrollTop = top
          } catch (err) {
            // Ignore jsdom scrollTop assignment errors.
          }
          return
        }
        try {
          window.scrollTo({ top, behavior })
        } catch (err) {
          scrollElement.scrollTop = top
        }
      } else if (typeof scrollElement.scrollTo === 'function') {
        scrollElement.scrollTo({ top, behavior })
      } else {
        scrollElement.scrollTop = top
      }
    },
    [getScrollElement],
  )

  const clampScrollPosition = useCallback(
    (reason: string) => {
      const { scrollTop, scrollHeight, clientHeight } = getScrollMetrics()
      const maxScrollTop = Math.max(scrollHeight - clientHeight, 0)
      if (scrollTop > maxScrollTop) {
        scrollToPosition(maxScrollTop, 'auto')
        console.debug('[useChatScroll] clamped scroll position', {
          sessionId,
          reason,
          messageCount: messages.length,
          maxScrollTop,
          currentScrollTop: scrollTop,
        })
      }
    },
    [getScrollMetrics, scrollToPosition, sessionId, messages.length],
  )

  // Reset state when session changes
  useEffect(() => {
    setVisibleCount(pageSize)
    autoScrollRef.current = true
    suppressAutoScrollOnceRef.current = false
    manualScrollRef.current = false
    pendingSessionScrollRef.current = true
    // Immediately scroll to top when switching sessions to prevent
    // being stuck at the bottom of the previous (possibly longer) session.
    scrollToPosition(0, 'auto')
    console.debug('[useChatScroll] reset scroll for session change', {
      sessionId,
      messageCount: messages.length,
    })
    // Use an extra frame to ensure any late-loading content is accounted for
    requestAnimationFrame(() => {
      scrollToPosition(0, 'auto')
    })
  }, [sessionId, pageSize, scrollToPosition])

  const isNearBottom = useCallback(() => {
    const { scrollTop, scrollHeight, clientHeight } = getScrollMetrics()
    // Distance to absolute bottom. Using a slightly larger threshold (160px)
    // to account for footers and varied input heights.
    return scrollHeight - scrollTop - clientHeight < 160
  }, [getScrollMetrics])

  const scrollToBottom = useCallback(
    (options?: { force?: boolean; behavior?: ScrollBehavior }) => {
      if (options?.force) {
        autoScrollRef.current = true
        manualScrollRef.current = false
        console.debug('[useChatScroll] auto-follow enabled', {
          sessionId,
          reason: 'force-scroll',
        })
      }

      if (!options?.force && !isNearBottom()) {
        const metrics = getScrollMetrics()
        console.debug('[useChatScroll] skip auto-scroll', {
          sessionId,
          reason: 'not-near-bottom',
          scrollTop: metrics.scrollTop,
          scrollHeight: metrics.scrollHeight,
          clientHeight: metrics.clientHeight,
        })
        return
      }

      // Always prefer calculating the absolute bottom because we have a
      // fixed footer and rely on document padding to push content above it.
      requestAnimationFrame(() => {
        const { scrollHeight, clientHeight } = getScrollMetrics()
        const targetScrollTop = Math.max(0, scrollHeight - clientHeight)

        scrollToPosition(targetScrollTop, options?.behavior || 'smooth')
      })
    },
    [isNearBottom, getScrollMetrics, scrollToPosition, sessionId],
  )

  useEffect(() => {
    if (!pendingSessionScrollRef.current) return
    if (messages.length === 0) return
    pendingSessionScrollRef.current = false
    scrollToBottom({ force: true, behavior: 'auto' })
    requestAnimationFrame(() => {
      clampScrollPosition('session-load')
    })
  }, [messages.length, scrollToBottom, clampScrollPosition])

  const scrollToTop = useCallback(() => {
    scrollToPosition(0, 'smooth')
  }, [scrollToPosition])

  /**
   * resetScroll resets scroll state and moves viewport to top.
   */
  const resetScroll = useCallback(() => {
    setVisibleCount(pageSize)
    autoScrollRef.current = true
    suppressAutoScrollOnceRef.current = false
    manualScrollRef.current = false
    scrollToPosition(0, 'auto')
  }, [pageSize, scrollToPosition])

  /**
   * isAtBottom checks whether the viewport is effectively at the bottom.
   */
  const isAtBottom = useCallback(() => {
    const { scrollTop, scrollHeight, clientHeight } = getScrollMetrics()
    return scrollHeight - scrollTop - clientHeight <= 8
  }, [getScrollMetrics])

  // Auto-scroll only when auto-follow is enabled (e.g., new send) or near bottom
  useEffect(() => {
    if (suppressAutoScrollOnceRef.current) {
      suppressAutoScrollOnceRef.current = false
      return
    }
    if (autoScrollRef.current || isNearBottom()) {
      scrollToBottom({ force: true, behavior: 'auto' })
    }
  }, [messages, scrollToBottom, isNearBottom])

  useEffect(() => {
    setVisibleCount((prev) => {
      if (messages.length === 0) {
        return pageSize
      }

      const desired = Math.min(pageSize, messages.length)

      if (prev < desired) {
        return desired
      }

      if (prev > messages.length) {
        return messages.length
      }

      return prev
    })
  }, [messages.length, pageSize])

  // Track scroll position for scroll-to-bottom button (using window scroll)
  useEffect(() => {
    const handleScroll = () => {
      const near = isNearBottom()
      setShowScrollButton(!near)
      // Disable auto-follow as soon as user scrolls away
      if (!near) {
        autoScrollRef.current = false
        return
      }

      if (manualScrollRef.current) {
        if (isAtBottom()) {
          manualScrollRef.current = false
          autoScrollRef.current = true
          console.debug('[useChatScroll] auto-follow enabled', {
            sessionId,
            reason: 'user-returned-to-bottom',
          })
        } else {
          autoScrollRef.current = false
        }
        return
      }

      autoScrollRef.current = true
    }

    window.addEventListener('scroll', handleScroll, { passive: true })
    return () => window.removeEventListener('scroll', handleScroll)
  }, [isNearBottom, isAtBottom, sessionId])

  // Detect explicit user scroll intent (wheel/touch/keyboard) to stop auto-follow immediately.
  useEffect(() => {
    // markManualScroll disables auto-follow based on explicit user scroll intent.
    const markManualScroll = (source: string) => {
      if (!manualScrollRef.current) {
        manualScrollRef.current = true
        autoScrollRef.current = false
        console.debug('[useChatScroll] auto-follow disabled', {
          sessionId,
          source,
        })
      }
    }

    const handleWheel = () => markManualScroll('wheel')
    const handleTouchMove = () => markManualScroll('touchmove')
    const handleKeyDown = (event: KeyboardEvent) => {
      const target = event.target as HTMLElement | null
      if (target?.isContentEditable) return
      if (target instanceof HTMLInputElement) return
      if (target instanceof HTMLTextAreaElement) return

      const keys = new Set([
        'PageUp',
        'PageDown',
        'Home',
        'End',
        'ArrowUp',
        'ArrowDown',
        ' ',
      ])
      if (keys.has(event.key)) {
        markManualScroll('keyboard')
      }
    }

    window.addEventListener('wheel', handleWheel, { passive: true })
    window.addEventListener('touchmove', handleTouchMove, { passive: true })
    window.addEventListener('keydown', handleKeyDown)

    return () => {
      window.removeEventListener('wheel', handleWheel)
      window.removeEventListener('touchmove', handleTouchMove)
      window.removeEventListener('keydown', handleKeyDown)
    }
  }, [sessionId])

  // Clamp scroll position when content shrinks (e.g., switching to shorter sessions).
  useEffect(() => {
    requestAnimationFrame(() => {
      clampScrollPosition('content-length-change')
    })
  }, [messages.length, sessionId, clampScrollPosition])

  // Automatically reset scroll and return to top when messages are cleared
  // in the current session.
  useEffect(() => {
    if (messages.length === 0 && !pendingSessionScrollRef.current) {
      scrollToPosition(0, 'auto')
      autoScrollRef.current = true
    }
  }, [messages.length, scrollToPosition])

  useEffect(() => {
    if (typeof ResizeObserver === 'undefined') return
    const target = contentRef?.current || document.body
    if (!target) return

    const observer = new ResizeObserver(() => {
      requestAnimationFrame(() => {
        clampScrollPosition('content-resize')
      })
    })

    observer.observe(target)
    return () => observer.disconnect()
  }, [contentRef, clampScrollPosition])

  const handleLoadOlder = useCallback(() => {
    const { scrollTop, scrollHeight } = getScrollMetrics()
    const prevScrollHeight = scrollHeight
    const prevScrollTop = scrollTop

    setVisibleCount((prev) => Math.min(prev + pageSize, messages.length))

    // Keep the viewport anchored after older messages are prepended.
    requestAnimationFrame(() => {
      const { scrollHeight: nextScrollHeight } = getScrollMetrics()
      const delta = nextScrollHeight - prevScrollHeight
      scrollToPosition(prevScrollTop + Math.max(delta, 0), 'auto')
    })
  }, [messages.length, pageSize, getScrollMetrics, scrollToPosition])

  const scrollToMessage = useCallback(
    (chatId: string, role: string) => {
      const id = `chat-message-${chatId}-${role}`

      // Find message index to update visibility if needed
      const msgIndex = messages.findIndex(
        (m) => m.chatID === chatId && m.role === role,
      )
      if (msgIndex === -1) return

      // If message is older than currently visible, expand visible count
      const isVisibleCount = messages.length - msgIndex
      if (isVisibleCount > visibleCount) {
        setVisibleCount(isVisibleCount)
      }

      // Wait for re-render if we expanded visibleCount
      requestAnimationFrame(() => {
        const element = document.getElementById(id)
        if (element) {
          element.scrollIntoView({ behavior: 'smooth', block: 'center' })
          // Add a temporary highlight effect
          element.classList.add('ring-2', 'ring-primary', 'ring-offset-2')
          setTimeout(() => {
            element.classList.remove('ring-2', 'ring-primary', 'ring-offset-2')
          }, 2000)
        }
      })
    },
    [messages, visibleCount],
  )

  return {
    messagesEndRef,
    showScrollButton,
    visibleCount,
    autoScrollRef,
    suppressAutoScrollOnceRef,
    scrollToBottom,
    scrollToTop,
    resetScroll,
    handleLoadOlder,
    isNearBottom,
    scrollToMessage,
  }
}
