import { useCallback, useEffect, useRef, useState } from 'react'
import type { ChatMessageData } from '../types'

interface UseMessageNavigationOptions {
  displayedMessages: ChatMessageData[]
  sessionId: number
}

/**
 * useMessageNavigation manages keyboard navigation and selection of messages.
 */
export function useMessageNavigation({
  displayedMessages,
  sessionId,
}: UseMessageNavigationOptions) {
  const [selectedMessageIndex, setSelectedMessageIndex] = useState<number>(-1)
  const isKeyboardSelectRef = useRef(false)

  // Reset selection when session changes or messages length changes
  useEffect(() => {
    setSelectedMessageIndex(-1) // eslint-disable-line react-hooks/set-state-in-effect -- reset on session/messages change
  }, [displayedMessages.length, sessionId])

  /**
   * findFirstVisibleMessageIndex finds the index of the first message
   * that is currently visible in the viewport.
   */
  const findFirstVisibleMessageIndex = useCallback((): number => {
    if (displayedMessages.length === 0) return -1

    const headerHeight = 48 // fixed header h-12
    const viewportBottom = window.innerHeight

    for (let i = 0; i < displayedMessages.length; i++) {
      const msg = displayedMessages[i]
      const el = document.getElementById(
        `chat-message-${msg.chatID}-${msg.role}`,
      )
      if (el) {
        const rect = el.getBoundingClientRect()
        // Message is visible if its bottom is below the header and its top is above viewport bottom
        if (rect.bottom > headerHeight && rect.top < viewportBottom) {
          return i
        }
      }
    }
    return 0
  }, [displayedMessages])

  /**
   * isMessageVisible checks whether the message at the given index
   * is currently (at least partially) visible in the viewport.
   */
  const isMessageVisible = useCallback(
    (index: number): boolean => {
      if (index < 0 || index >= displayedMessages.length) return false
      const msg = displayedMessages[index]
      const el = document.getElementById(
        `chat-message-${msg.chatID}-${msg.role}`,
      )
      if (!el) return false
      const headerHeight = 48
      const rect = el.getBoundingClientRect()
      return rect.bottom > headerHeight && rect.top < window.innerHeight
    },
    [displayedMessages],
  )

  const handleMessageSelect = useCallback((index: number) => {
    isKeyboardSelectRef.current = false
    setSelectedMessageIndex((prev) => (prev === index ? -1 : index))
  }, [])

  /**
   * navigateMessageUp moves the current selection up by one message.
   * If no message is selected, or the selected message has scrolled out of
   * the viewport, it resets to the first visible message.
   */
  const navigateMessageUp = useCallback(() => {
    setSelectedMessageIndex((prev) => {
      isKeyboardSelectRef.current = true
      if (prev === -1 || !isMessageVisible(prev)) {
        const visibleIdx = findFirstVisibleMessageIndex()
        return visibleIdx >= 0 ? visibleIdx : 0
      }

      return Math.max(0, prev - 1)
    })
  }, [findFirstVisibleMessageIndex, isMessageVisible])

  // Keyboard shortcuts for message navigation
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      // Ignore keyboard events when composition is in progress (IME)
      if (e.isComposing) return

      const isInput =
        e.target instanceof HTMLInputElement ||
        e.target instanceof HTMLTextAreaElement

      if (e.key === 'ArrowUp') {
        if (isInput && !e.altKey) {
          if (e.target instanceof HTMLTextAreaElement) {
            if (e.target.selectionStart !== 0) return
          } else {
            return
          }
        }

        e.preventDefault()
        navigateMessageUp()
      } else if (e.key === 'ArrowDown') {
        if (isInput && !e.altKey) {
          if (e.target instanceof HTMLTextAreaElement) {
            if (e.target.selectionStart !== e.target.value.length) return
          } else {
            return
          }
        }

        e.preventDefault()
        setSelectedMessageIndex((prev) => {
          isKeyboardSelectRef.current = true
          if (prev === -1 || !isMessageVisible(prev)) {
            const visibleIdx = findFirstVisibleMessageIndex()
            return visibleIdx >= 0 ? visibleIdx : 0
          }
          if (prev === displayedMessages.length - 1) return -1
          return prev + 1
        })
      } else if (e.key === 'Escape') {
        setSelectedMessageIndex(-1)
      }
    }

    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [
    displayedMessages,
    findFirstVisibleMessageIndex,
    isMessageVisible,
    navigateMessageUp,
  ])

  // Scroll selected message into view, accounting for the fixed header (48px)
  useEffect(() => {
    if (
      selectedMessageIndex >= 0 &&
      selectedMessageIndex < displayedMessages.length &&
      isKeyboardSelectRef.current
    ) {
      const msg = displayedMessages[selectedMessageIndex]
      const el = document.getElementById(
        `chat-message-${msg.chatID}-${msg.role}`,
      )
      if (el) {
        const headerHeight = 48
        const rect = el.getBoundingClientRect()

        if (rect.top < headerHeight) {
          // Element is above or behind the fixed header – scroll it just below the header
          window.scrollBy({ top: rect.top - headerHeight, behavior: 'smooth' })
        } else if (rect.bottom > window.innerHeight) {
          // Element is below the viewport – scroll it into view from the bottom
          window.scrollBy({
            top: rect.bottom - window.innerHeight,
            behavior: 'smooth',
          })
        }
      }
    }
  }, [selectedMessageIndex, displayedMessages])

  return {
    selectedMessageIndex,
    setSelectedMessageIndex,
    handleMessageSelect,
    navigateMessageUp,
  }
}
