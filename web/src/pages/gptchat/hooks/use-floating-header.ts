/**
 * Hook to track which message header has scrolled out of view.
 * Returns the message data for the floating header display.
 */
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'

import type { ChatMessageData } from '../types'

export interface UseFloatingHeaderOptions {
  /** Array of messages being displayed */
  messages: ChatMessageData[]
  /** The messages container ref */
  containerRef: React.RefObject<HTMLElement | null>
  /** Offset from top where the header starts (e.g., fixed header height) */
  topOffset?: number
}

export interface FloatingHeaderState {
  /** The message ID that should show in the floating header */
  chatId: string | null
  /** The message role */
  role: string | null
  /** The index of the message in the messages array */
  index: number | null
  /** Whether the floating header should be visible */
  visible: boolean
}

/**
 * useFloatingHeader tracks scroll position and determines which message's
 * header should be shown in the floating header bar.
 *
 * The floating header becomes visible when:
 * 1. A message's inline header has scrolled above the viewport
 * 2. The message body is still partially visible
 *
 * @param options - Configuration options
 * @returns The floating header state
 */
export function useFloatingHeader({
  messages,
  containerRef,
  topOffset = 48, // Default header height (12 * 4 = 48px for top-12)
}: UseFloatingHeaderOptions): FloatingHeaderState {
  const [state, setState] = useState<FloatingHeaderState>({
    chatId: null,
    role: null,
    index: null,
    visible: false,
  })

  // Create a lookup map for performance if messages array is large
  const messageLookup = useMemo(() => {
    const map = new Map<string, number>()
    messages.forEach((m, i) => {
      map.set(`${m.chatID}-${m.role}`, i)
    })
    return map
  }, [messages])

  const rafRef = useRef<number | null>(null)

  const updateFloatingHeader = useCallback(() => {
    if (!containerRef.current || messages.length === 0) {
      if (state.visible) {
        setState({ chatId: null, role: null, index: null, visible: false })
      }
      return
    }

    // Find all message elements
    const messageElements = containerRef.current.querySelectorAll(
      '[id^="chat-message-"]',
    )

    if (messageElements.length === 0) {
      if (state.visible) {
        setState({ chatId: null, role: null, index: null, visible: false })
      }
      return
    }

    // Floating header threshold (where the floating header appears)
    // The floating header should appear when the message's inline header
    // starts to be covered by the main header.
    const floatingHeaderThreshold = topOffset

    let targetChatId: string | null = null
    let targetRole: string | null = null
    let targetIndex: number | null = null

    // Find the message whose header is above the threshold but body is still visible
    for (let i = 0; i < messageElements.length; i++) {
      const element = messageElements[i] as HTMLElement
      const rect = element.getBoundingClientRect()

      // Parse the message ID from the element ID (format: chat-message-{chatId}-{role})
      const idParts = element.id.split('-')
      if (idParts.length < 4) continue

      const chatId = idParts.slice(2, -1).join('-')
      const role = idParts[idParts.length - 1]

      // The message header has scrolled above the threshold
      // We use a small buffer to ensure it's actually being covered by the main header
      const headerScrolledOut = rect.top < floatingHeaderThreshold - 10
      const bodyStillVisible = rect.bottom > floatingHeaderThreshold + 60 // At least 60px of body visible

      if (headerScrolledOut && bodyStillVisible) {
        targetChatId = chatId
        targetRole = role
        targetIndex = messageLookup.get(`${chatId}-${role}`) ?? null
        // Stop checking - we want the first one that matches
        break
      }
    }

    const newVisible = targetChatId !== null

    setState((prev) => {
      // Only update state if something changed.
      // We check for:
      // 1. Visibility change
      // 2. Different message (id or role changed)
      if (
        newVisible !== prev.visible ||
        prev.chatId !== targetChatId ||
        prev.role !== targetRole
      ) {
        return {
          chatId: targetChatId,
          role: targetRole,
          index: targetIndex,
          visible: newVisible,
        }
      }
      return prev
    })
  }, [containerRef, messages.length, topOffset, state.visible, messageLookup])

  // Handle scroll events with requestAnimationFrame for performance
  const handleScroll = useCallback(() => {
    if (rafRef.current !== null) {
      cancelAnimationFrame(rafRef.current)
    }
    rafRef.current = requestAnimationFrame(() => {
      updateFloatingHeader()
    })
  }, [updateFloatingHeader])

  // Set up scroll listener on window (since the page uses window scrolling)
  useEffect(() => {
    window.addEventListener('scroll', handleScroll, { passive: true })

    // Initial check
    updateFloatingHeader()

    return () => {
      window.removeEventListener('scroll', handleScroll)
      if (rafRef.current !== null) {
        cancelAnimationFrame(rafRef.current)
      }
    }
  }, [handleScroll, updateFloatingHeader])

  // Update when messages change
  useEffect(() => {
    updateFloatingHeader()
  }, [messages, updateFloatingHeader])

  return state
}
