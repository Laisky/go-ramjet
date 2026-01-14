import { useCallback, useEffect, useRef, useState } from 'react'
import type { ChatMessageData } from '../types'

interface UseChatScrollOptions {
  messages: ChatMessageData[]
  pageSize: number
  sessionId: string | number
}

/**
 * useChatScroll manages scrolling behavior for the chat interface.
 */
export function useChatScroll({
  messages,
  pageSize,
  sessionId,
}: UseChatScrollOptions) {
  const messagesEndRef = useRef<HTMLDivElement>(null)
  const [showScrollButton, setShowScrollButton] = useState(false)
  const [visibleCount, setVisibleCount] = useState(pageSize)
  const autoScrollRef = useRef(true)
  const suppressAutoScrollOnceRef = useRef(false)

  // Reset state when session changes
  useEffect(() => {
    setVisibleCount(pageSize)
    autoScrollRef.current = true
    suppressAutoScrollOnceRef.current = false
    // Immediately scroll to top when switching sessions to prevent
    // being stuck at the bottom of the previous (possibly longer) session.
    window.scrollTo({ top: 0, behavior: 'auto' })
  }, [sessionId, pageSize])

  const isNearBottom = useCallback(() => {
    const { scrollTop, scrollHeight, clientHeight } = document.documentElement
    return scrollHeight - scrollTop - clientHeight < 120
  }, [])

  const scrollToBottom = useCallback(
    (options?: { force?: boolean; behavior?: ScrollBehavior }) => {
      if (!options?.force && !isNearBottom()) {
        return
      }
      window.scrollTo({
        top: document.documentElement.scrollHeight,
        behavior: options?.behavior || 'smooth',
      })
    },
    [isNearBottom],
  )

  const scrollToTop = useCallback(() => {
    window.scrollTo({ top: 0, behavior: 'smooth' })
  }, [])

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
      } else {
        autoScrollRef.current = true
      }
    }

    window.addEventListener('scroll', handleScroll, { passive: true })
    return () => window.removeEventListener('scroll', handleScroll)
  }, [isNearBottom])

  const handleLoadOlder = useCallback(() => {
    const prevScrollHeight = document.documentElement.scrollHeight
    const prevScrollTop = window.scrollY

    setVisibleCount((prev) => Math.min(prev + pageSize, messages.length))

    // Keep the viewport anchored after older messages are prepended.
    requestAnimationFrame(() => {
      const nextScrollHeight = document.documentElement.scrollHeight
      const delta = nextScrollHeight - prevScrollHeight
      window.scrollTo({ top: prevScrollTop + Math.max(delta, 0) })
    })
  }, [messages.length, pageSize])

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
    handleLoadOlder,
    isNearBottom,
    scrollToMessage,
  }
}
