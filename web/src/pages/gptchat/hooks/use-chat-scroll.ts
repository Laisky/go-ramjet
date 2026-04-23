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
  /**
   * chatID of the message that is currently being streamed or reloaded.
   * When provided, auto-follow scrolling anchors to that message's BOTTOM
   * instead of the end-of-list marker.  This ensures regenerating or
   * editing a mid-conversation message keeps its streaming content visible
   * rather than scrolling past it to the (unchanged) final message.
   */
  streamingChatId?: string | null
  /** Role of the streaming message. Defaults to 'assistant'. */
  streamingRole?: string
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
  streamingChatId = null,
  streamingRole = 'assistant',
}: UseChatScrollOptions) {
  const messagesEndRef = useRef<HTMLDivElement>(null)
  const [showScrollButton, setShowScrollButton] = useState(false)
  const [visibleCount, setVisibleCount] = useState(pageSize)
  const pendingSessionScrollRef = useRef(false)

  /**
   * Ref mirror of streamingChatId so scroll callbacks stay stable while still
   * observing the latest value.  The value changes every time a new message
   * starts streaming, and rebuilding every useCallback would churn the scroll
   * effect deps unnecessarily.
   */
  const streamingChatIdRef = useRef<string | null>(streamingChatId)
  const streamingRoleRef = useRef<string>(streamingRole)
  // eslint-disable-next-line react-hooks/refs -- keep the anchor identifiers in sync without churning scroll callback deps
  streamingChatIdRef.current = streamingChatId
  // eslint-disable-next-line react-hooks/refs -- same reason as above
  streamingRoleRef.current = streamingRole

  /**
   * Tracks the most recent scrollTop observed in the scroll handler so we can
   * detect user-initiated upward movement (scrollbar drag, momentum) that the
   * wheel/touch/keyboard listeners do not surface.
   */
  const lastScrollTopRef = useRef(0)

  /**
   * Target of the last programmatic scrollToPosition call.  When the next
   * scroll event matches this value (within a few pixels), we treat it as
   * our own scroll and do not interpret it as user intent.
   */
  const lastProgrammaticTargetRef = useRef<number | null>(null)

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
      // Record the target so the scroll event handler can recognise this as
      // a programmatic scroll and avoid mis-attributing it to the user.
      lastProgrammaticTargetRef.current = top

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
   * getContentMaxScroll computes the tightest upper-bound scroll position.
   *
   * When a message is currently being streamed/reloaded (streamingChatId is
   * provided and its DOM element is mounted), the target is calculated so the
   * BOTTOM of that message sits just above the fixed footer.  This is critical
   * for regenerating or editing a mid-conversation message: anchoring on the
   * end-of-list marker would scroll past the streaming content to the
   * (unchanged) final message, which the user would perceive as "auto-scroll
   * went to the wrong place".
   *
   * When no streaming anchor is available, the content-end marker
   * (messagesEndRef) is used as the anchor instead.  Both paths are capped at
   * scrollHeight - clientHeight so we never request a scroll past the page end.
   */
  const getContentMaxScroll = useCallback(() => {
    const { scrollTop, scrollHeight, clientHeight } = getScrollMetrics()
    const docMax = Math.max(scrollHeight - clientHeight, 0)

    const streamingId = streamingChatIdRef.current
    const streamingMsgRole = streamingRoleRef.current || 'assistant'

    let anchorAbsoluteY: number | null = null

    if (streamingId && typeof document !== 'undefined') {
      const el = document.getElementById(
        `chat-message-${streamingId}-${streamingMsgRole}`,
      )
      if (el) {
        // Use the streaming message's BOTTOM so its latest content stays
        // visible while it grows, regardless of how many messages follow it
        // or how their heights have changed from prior edits/reloads.
        anchorAbsoluteY = scrollTop + el.getBoundingClientRect().bottom
      }
    }

    if (anchorAbsoluteY === null) {
      const endEl = messagesEndRef.current
      if (!endEl) return docMax
      anchorAbsoluteY = scrollTop + endEl.getBoundingClientRect().top
    }

    // Place the anchor at (clientHeight - footerHeight - 8) from the viewport
    // top so it sits just above the fixed footer with a small buffer.
    const contentMax = Math.max(
      anchorAbsoluteY - clientHeight + footerHeight + 8,
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
    (options?: {
      force?: boolean
      behavior?: ScrollBehavior
      /**
       * When true, bypass the isNearBottom short-circuit.  Used by the
       * streaming auto-follow effect so content growth doesn't drop us out of
       * auto-follow just because the page became taller than the viewport.
       * Unlike `force: true`, this does NOT reset the scroll mode, so a
       * concurrent user scroll still takes precedence inside the rAF.
       */
      ignoreNearBottom?: boolean
    }) => {
      if (options?.force) {
        scrollModeRef.current = 'auto-follow'
        userScrollIntentRef.current = false
        console.debug('[useChatScroll] auto-follow enabled', {
          sessionId,
          reason: 'force-scroll',
        })
      }

      if (!options?.force && !options?.ignoreNearBottom && !isNearBottom()) {
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

      // Scroll to the bottom of the content.  We prefer an anchor element
      // (the streaming message, falling back to the content-end marker) so we
      // never overshoot past the meaningful point of interest, even if
      // scrollHeight is temporarily stale.
      requestAnimationFrame(() => {
        // Re-check the scroll mode inside the rAF: a concurrent user scroll
        // between scheduling and firing this callback should still win.
        // `force: true` is the only caller allowed to bypass this — it
        // represents an explicit user action (send message, click scroll-to-
        // bottom) which re-asserts auto-follow regardless of prior state.
        if (!options?.force && scrollModeRef.current !== 'auto-follow') {
          return
        }

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
  //
  // We intentionally do NOT use `force: true` here: force would blindly reset
  // scroll mode to auto-follow even if the user had just scrolled away in the
  // interval between this effect being scheduled and the rAF running, making
  // it impossible to stop mid-stream auto-scroll via scrollbar drag or any
  // other scroll input that fires between renders.
  useEffect(() => {
    if (scrollModeRef.current !== 'auto-follow') {
      return
    }
    scrollToBottom({ ignoreNearBottom: true, behavior: 'auto' })
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

  // Track scroll position for scroll-to-bottom button (using window scroll).
  // Also detects user-initiated upward scroll that does NOT come through
  // wheel / touch / keyboard (e.g. scrollbar drag, trackpad inertia) so we
  // can honour the requirement that any manual scroll during loading hands
  // control back to the user.
  useEffect(() => {
    const handleScroll = () => {
      const { scrollTop } = getScrollMetrics()
      const near = isNearBottom()
      setShowScrollButton(!near)

      const mode = scrollModeRef.current
      const prev = lastScrollTopRef.current
      lastScrollTopRef.current = scrollTop

      const progTarget = lastProgrammaticTargetRef.current
      const isProgrammatic =
        progTarget !== null && Math.abs(scrollTop - progTarget) <= 4
      if (isProgrammatic) {
        lastProgrammaticTargetRef.current = null
      }

      // Significant upward movement that isn't our own scrollToPosition call
      // is always treated as user intent — this is the only way to catch
      // scrollbar drags and touchpad momentum without wheel/touch events.
      const isUserUpward = !isProgrammatic && scrollTop < prev - 4

      if (mode === 'viewport-locked') {
        if (isUserUpward) {
          scrollModeRef.current = 'user-scrolled'
          userScrollIntentRef.current = true
          console.debug(
            '[useChatScroll] user-scrolled (from viewport-locked via scroll event)',
            { sessionId, scrollTop, prev },
          )
        }
        // Otherwise viewport-locked is immune to scroll events.
        return
      }

      if (mode === 'auto-follow') {
        if (isUserUpward) {
          scrollModeRef.current = 'user-scrolled'
          userScrollIntentRef.current = true
          console.debug(
            '[useChatScroll] user-scrolled (from auto-follow via scroll event)',
            { sessionId, scrollTop, prev },
          )
        }
        return
      }

      // mode === 'user-scrolled'
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
    }

    window.addEventListener('scroll', handleScroll, { passive: true })
    return () => window.removeEventListener('scroll', handleScroll)
  }, [getScrollMetrics, isNearBottom, isAtBottom, sessionId])

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
