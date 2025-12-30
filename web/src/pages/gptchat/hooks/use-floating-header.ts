/**
 * Hook to track which message header has scrolled out of view.
 * Returns the message data for the floating header display.
 */
import { useCallback, useEffect, useRef, useState } from 'react'

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
  /** The message that should show in the floating header */
  message: ChatMessageData | null
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
    message: null,
    visible: false,
  })

  const rafRef = useRef<number | null>(null)

  const updateFloatingHeader = useCallback(() => {
    if (!containerRef.current || messages.length === 0) {
      if (state.visible) {
        setState({ message: null, visible: false })
      }
      return
    }

    // Find all message elements
    const messageElements = containerRef.current.querySelectorAll(
      '[id^="chat-message-"]',
    )

    if (messageElements.length === 0) {
      if (state.visible) {
        setState({ message: null, visible: false })
      }
      return
    }

    // Floating header threshold (where the floating header appears)
    const floatingHeaderThreshold = topOffset + 40 // Top offset + floating header height

    let targetMessage: ChatMessageData | null = null

    // Find the message whose header is above the threshold but body is still visible
    for (let i = 0; i < messageElements.length; i++) {
      const element = messageElements[i] as HTMLElement
      const rect = element.getBoundingClientRect()

      // Parse the message ID from the element ID (format: chat-message-{chatId}-{role})
      const idParts = element.id.split('-')
      if (idParts.length < 4) continue

      const chatId = idParts.slice(2, -1).join('-')
      const role = idParts[idParts.length - 1]

      // Find the corresponding message
      const msg = messages.find((m) => m.chatID === chatId && m.role === role)
      if (!msg) continue

      // The message header (first ~36px of the card) has scrolled above the threshold
      // but the bottom of the message is still below the threshold
      const headerScrolledOut = rect.top < floatingHeaderThreshold
      const bodyStillVisible = rect.bottom > floatingHeaderThreshold + 50 // At least 50px of body visible

      if (headerScrolledOut && bodyStillVisible) {
        targetMessage = msg
        // Continue checking - we want the last one that matches (topmost message with header out of view)
        break
      }
    }

    const newVisible = targetMessage !== null
    const currentChatId = state.message?.chatID
    const currentRole = state.message?.role
    const newChatId = targetMessage?.chatID
    const newRole = targetMessage?.role

    // Only update state if something changed
    if (
      newVisible !== state.visible ||
      currentChatId !== newChatId ||
      currentRole !== newRole
    ) {
      setState({
        message: targetMessage,
        visible: newVisible,
      })
    }
  }, [containerRef, messages, state.message, state.visible, topOffset])

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
