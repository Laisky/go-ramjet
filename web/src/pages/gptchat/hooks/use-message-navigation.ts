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
    setSelectedMessageIndex(-1)
  }, [displayedMessages.length, sessionId])

  /**
   * findFirstVisibleMessageIndex finds the index of the first message
   * that is currently visible in the viewport.
   */
  const findFirstVisibleMessageIndex = useCallback((): number => {
    if (displayedMessages.length === 0) return -1

    const viewportTop = 0
    const viewportBottom = window.innerHeight

    for (let i = 0; i < displayedMessages.length; i++) {
      const msg = displayedMessages[i]
      const el = document.getElementById(
        `chat-message-${msg.chatID}-${msg.role}`,
      )
      if (el) {
        const rect = el.getBoundingClientRect()
        if (rect.bottom > viewportTop && rect.top < viewportBottom) {
          return i
        }
      }
    }
    return 0
  }, [displayedMessages])

  const handleMessageSelect = useCallback((index: number) => {
    isKeyboardSelectRef.current = false
    setSelectedMessageIndex((prev) => (prev === index ? -1 : index))
  }, [])

  // Keyboard shortcuts for message navigation
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
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
        setSelectedMessageIndex((prev) => {
          isKeyboardSelectRef.current = true
          if (prev === -1) {
            const visibleIdx = findFirstVisibleMessageIndex()
            return visibleIdx >= 0 ? visibleIdx : displayedMessages.length - 1
          }
          return Math.max(0, prev - 1)
        })
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
          if (prev === -1) {
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
  }, [displayedMessages, findFirstVisibleMessageIndex])

  // Scroll selected message into view
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
        el.scrollIntoView({ behavior: 'smooth', block: 'nearest' })
      }
    }
  }, [selectedMessageIndex, displayedMessages])

  return {
    selectedMessageIndex,
    setSelectedMessageIndex,
    handleMessageSelect,
  }
}
