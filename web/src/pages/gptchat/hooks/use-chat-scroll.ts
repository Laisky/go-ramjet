import { useCallback, useEffect, useRef, useState } from 'react'
import type { ChatMessageData } from '../types'

/**
 * ScrollMode represents the current scroll behavior state.
 *
 * State machine transitions:
 *
 *   AUTO_FOLLOW  ──wheel/touch/kbd──▶  USER_SCROLLED
 *        │                                   │
 *        │ edit/regenerate/navigate           │ edit/regenerate/navigate
 *        ▼                                   ▼
 *   VIEWPORT_LOCKED  ◀──────────────  VIEWPORT_LOCKED
 *
 * Exit VIEWPORT_LOCKED:
 *   → user sends message / clicks scroll-to-bottom → AUTO_FOLLOW
 *   → user wheel/touch scrolls → USER_SCROLLED
 *
 * Exit USER_SCROLLED:
 *   → user actively scrolls near bottom (wheel/touch) → AUTO_FOLLOW
 *   → user sends message / clicks scroll-to-bottom → AUTO_FOLLOW
 *   → session change → AUTO_FOLLOW
 */
export type ScrollMode = 'auto-follow' | 'user-scrolled' | 'viewport-locked'

interface UseChatScrollOptions {
  messages: ChatMessageData[]
  pageSize: number
  sessionId: string | number
  contentRef?: React.RefObject<HTMLElement | null>
  /** Height of the fixed footer in pixels, used to cap scroll position. */
  footerHeight?: number
}

/**
 * useChatScroll manages scrolling behavior for the chat interface.
 */
export function useChatScroll({
  messages,
  pageSize,
  sessionId,
  contentRef,
  footerHeight = 112,
}: UseChatScrollOptions) {
  const messagesEndRef = useRef<HTMLDivElement>(null)
  const [showScrollButton, setShowScrollButton] = useState(false)
  const [visibleCount, setVisibleCount] = useState(pageSize)
  const pendingSessionScrollRef = useRef(false)

  /**
   * Single source of truth for scroll behavior.
   * Replaces the previous autoScrollRef / manualScrollRef / suppressAutoScrollOnceRef
   * triple which had race conditions between content-change-induced scroll events
   * and the manual-mode reset logic.
   */
  const scrollModeRef = useRef<ScrollMode>('auto-follow')

  /**
   * Set to true by wheel / touch / keyboard handlers (real user interaction).
   * Checked by the scroll event handler to distinguish "user scrolled to bottom"
   * from "content change pushed viewport to bottom".
   * Cleared after the scroll handler processes it.
   */
  const userScrollIntentRef = useRef(false)

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
      // Use the scrolling element's own scrollHeight rather than
      // Math.max across multiple sources.  During DOM transitions
      // (e.g. session switching) body.scrollHeight can retain a stale
      // larger value from previous content, causing scrollToBottom to
      // overshoot and leave a large blank space below the last message.
      const scrollHeight = scrollElement.scrollHeight
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
          } catch {
            // Ignore jsdom scrollTop assignment errors.
          }
          return
        }
        try {
          window.scrollTo({ top, behavior })
        } catch {
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

  /**
   * getContentMaxScroll computes the tightest upper-bound scroll position
   * based on the actual content-end marker (messagesEndRef).  When the marker
   * is available it returns a scroll position that places the marker just
   * above the fixed footer; otherwise it falls back to scrollHeight-based max.
   */
  const getContentMaxScroll = useCallback(() => {
    const { scrollTop, scrollHeight, clientHeight } = getScrollMetrics()
    const docMax = Math.max(scrollHeight - clientHeight, 0)

    const endEl = messagesEndRef.current
    if (!endEl) return docMax

    // Absolute document-Y of the content-end marker.
    const endAbsoluteY = scrollTop + endEl.getBoundingClientRect().top
    // Place the marker at (clientHeight - footerHeight) from the viewport top
    // so it sits just above the fixed footer.
    const contentMax = Math.max(
      endAbsoluteY - clientHeight + footerHeight + 8,
      0,
    )
    return Math.min(docMax, contentMax)
  }, [getScrollMetrics, footerHeight])

  const clampScrollPosition = useCallback(
    (reason: string) => {
      const { scrollTop } = getScrollMetrics()
      const maxScrollTop = getContentMaxScroll()
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
    [
      getScrollMetrics,
      getContentMaxScroll,
      scrollToPosition,
      sessionId,
      messages.length,
    ],
  )

  // Reset state when session changes
  useEffect(() => {
    setVisibleCount(pageSize) // eslint-disable-line react-hooks/set-state-in-effect -- reset on session change
    scrollModeRef.current = 'auto-follow'
    userScrollIntentRef.current = false
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
  }, [sessionId, pageSize, scrollToPosition]) // eslint-disable-line react-hooks/exhaustive-deps

  const isNearBottom = useCallback(() => {
    const { scrollTop, scrollHeight, clientHeight } = getScrollMetrics()
    // Distance to absolute bottom. Using a slightly larger threshold (160px)
    // to account for footers and varied input heights.
    return scrollHeight - scrollTop - clientHeight < 160
  }, [getScrollMetrics])

  const scrollToBottom = useCallback(
    (options?: { force?: boolean; behavior?: ScrollBehavior }) => {
      if (options?.force) {
        scrollModeRef.current = 'auto-follow'
        userScrollIntentRef.current = false
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

      // Scroll to the bottom of the content.  We prefer the content-end
      // marker (messagesEndRef) to compute the target so we never overshoot
      // past the actual messages, even if scrollHeight is temporarily stale.
      requestAnimationFrame(() => {
        const contentMax = getContentMaxScroll()
        const { scrollHeight, clientHeight } = getScrollMetrics()
        const docMax = Math.max(0, scrollHeight - clientHeight)
        const targetScrollTop = Math.min(docMax, Math.max(contentMax, 0))

        scrollToPosition(targetScrollTop, options?.behavior || 'smooth')
      })
    },
    [
      isNearBottom,
      getScrollMetrics,
      getContentMaxScroll,
      scrollToPosition,
      sessionId,
    ],
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
    scrollModeRef.current = 'auto-follow'
    userScrollIntentRef.current = false
    scrollToPosition(0, 'auto')
  }, [pageSize, scrollToPosition])

  /**
   * lockViewport transitions to viewport-locked mode.
   * Use this before operations that change content but should keep the viewport stable
   * (e.g., edit-and-retry, regenerate, message navigation).
   *
   * The lock persists until explicitly released by:
   * - scrollToBottom({ force: true }) (send message, click scroll-to-bottom button)
   * - User wheel/touch scroll (transitions to user-scrolled)
   * - Session change (resets to auto-follow)
   */
  const lockViewport = useCallback(() => {
    scrollModeRef.current = 'viewport-locked'
    userScrollIntentRef.current = false
    console.debug('[useChatScroll] viewport locked', { sessionId })
  }, [sessionId])

  /**
   * isAtBottom checks whether the viewport is effectively at the bottom.
   */
  const isAtBottom = useCallback(() => {
    const { scrollTop, scrollHeight, clientHeight } = getScrollMetrics()
    return scrollHeight - scrollTop - clientHeight <= 8
  }, [getScrollMetrics])

  // Auto-scroll only when in auto-follow mode.
  // In user-scrolled or viewport-locked mode, the user/operation controls the viewport.
  useEffect(() => {
    if (scrollModeRef.current !== 'auto-follow') {
      return
    }
    scrollToBottom({ force: true, behavior: 'auto' })
  }, [messages, scrollToBottom])

  useEffect(() => {
    setVisibleCount((prev) => {
      // eslint-disable-line react-hooks/set-state-in-effect -- clamp visible count to message bounds
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

      const mode = scrollModeRef.current

      if (mode === 'viewport-locked') {
        // Viewport-locked mode is immune to scroll events.
        // Only explicit user actions (wheel/touch → user-scrolled, or
        // force-scroll → auto-follow) can exit this state.
        return
      }

      if (mode === 'user-scrolled') {
        // Only re-enable auto-follow if the user ACTIVELY scrolled near the bottom.
        // Content-change scroll events (clamp, resize) have userScrollIntentRef=false
        // and must NOT re-enable auto-follow.
        if (near && userScrollIntentRef.current) {
          scrollModeRef.current = 'auto-follow'
          userScrollIntentRef.current = false
          console.debug('[useChatScroll] auto-follow enabled', {
            sessionId,
            reason: 'user-returned-to-bottom',
          })
        }
        return
      }

      // mode === 'auto-follow'
      if (!near) {
        // Scrolled away from bottom without explicit user intent
        // (e.g., content prepended above). Stay in auto-follow;
        // the next messages-change effect will scroll back to bottom.
        // If the user intended to scroll away, the wheel/touch handler
        // already transitioned to user-scrolled before this fires.
      }
    }

    window.addEventListener('scroll', handleScroll, { passive: true })
    return () => window.removeEventListener('scroll', handleScroll)
  }, [isNearBottom, isAtBottom, sessionId])

  // Detect explicit user scroll intent (wheel/touch/keyboard) to transition modes.
  useEffect(() => {
    const handleUserScroll = (source: string) => {
      userScrollIntentRef.current = true
      const mode = scrollModeRef.current

      if (mode === 'auto-follow') {
        scrollModeRef.current = 'user-scrolled'
        console.debug('[useChatScroll] user-scrolled (from auto-follow)', {
          sessionId,
          source,
        })
      } else if (mode === 'viewport-locked') {
        // User is taking back control from the locked state.
        scrollModeRef.current = 'user-scrolled'
        console.debug('[useChatScroll] user-scrolled (from viewport-locked)', {
          sessionId,
          source,
        })
      }
      // If already user-scrolled, just update the intent flag.
    }

    const handleWheel = () => handleUserScroll('wheel')
    const handleTouchMove = () => handleUserScroll('touchmove')
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
        handleUserScroll('keyboard')
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
      scrollModeRef.current = 'auto-follow'
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
    scrollModeRef,
    lockViewport,
    scrollToBottom,
    scrollToTop,
    resetScroll,
    handleLoadOlder,
    isNearBottom,
    scrollToMessage,
  }
}
