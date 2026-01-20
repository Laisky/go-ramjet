/**
 * Floating message header component that shows action buttons for the
 * currently visible message when its header has scrolled out of view.
 */
import { useCallback } from 'react'

import { cn } from '@/utils/cn'
import { useTTS } from '../hooks/use-tts'
import type { ChatAttachment, ChatMessageData } from '../types'
import { ChatMessageHeader } from './chat-message-header'

export interface FloatingMessageHeaderProps {
  /** The full messages array to look up latest content */
  messages: ChatMessageData[]
  /** The message ID to display header for (from useFloatingHeader) */
  chatId: string | null
  /** The message role */
  role: string | null
  /** Whether the floating header should be visible */
  visible: boolean
  /** Callback to delete the message */
  onDelete?: (chatId: string) => void
  /** Callback to regenerate the message */
  onRegenerate?: (chatId: string) => void
  /** Callback for edit and resend */
  onEditResend?: (payload: {
    chatId: string
    content: string
    attachments?: ChatAttachment[]
  }) => void
  /** Callback to fork the session */
  onFork?: (chatId: string, role: string) => void
  /** The paired user message (for edit/resend) */
  pairedUserMessage?: ChatMessageData
  /** Whether the message is streaming */
  isStreaming?: boolean
  /** Scroll container ref for positioning */
  containerRef?: React.RefObject<HTMLElement>
  /** API token for TTS functionality */
  apiToken?: string
  /** The index of this message in the list */
  messageIndex?: number | null
  /** Called when user clicks the message to toggle selection */
  onSelect?: (index: number) => void
}

/**
 * FloatingMessageHeader renders a fixed header bar with action buttons
 * for the currently visible message when its inline header has scrolled
 * out of view.
 */
export function FloatingMessageHeader({
  messages,
  chatId,
  role,
  visible,
  onDelete,
  onRegenerate,
  onEditResend,
  onFork,
  pairedUserMessage,
  isStreaming,
  apiToken,
  messageIndex,
  onSelect,
}: FloatingMessageHeaderProps) {
  // Find the CURRENT version of the message from the source array
  // This ensures we always have the latest content even during streaming
  const message = messages.find((m) => m.chatID === chatId && m.role === role)
  const isUser = message?.role === 'user'

  const {
    isLoading: ttsLoading,
    audioUrl: ttsAudioUrl,
    error: ttsError,
    requestTTS,
    stopTTS,
  } = useTTS({
    apiToken: apiToken || '',
  })

  const handleHeaderClick = useCallback(
    (e: React.MouseEvent) => {
      // Don't toggle selection if clicking on interactive elements
      const target = e.target as HTMLElement
      if (target.closest('button')) {
        return
      }

      // If there's a selection, don't toggle message selection
      const selection = window.getSelection()
      if (selection && selection.toString().trim().length > 0) {
        return
      }

      if (onSelect !== undefined && typeof messageIndex === 'number') {
        onSelect(messageIndex)
      }
    },
    [onSelect, messageIndex],
  )

  if (!message || !visible) {
    return null
  }

  return (
    <div
      onClick={handleHeaderClick}
      className={cn(
        'fixed left-10 right-0 top-12 z-20 border-b shadow-md backdrop-blur-sm transition-all duration-300 ease-in-out',
        visible
          ? 'translate-y-0 opacity-100'
          : '-translate-y-full opacity-0 pointer-events-none',
        isUser ? 'bg-primary/10 border-primary/20' : 'bg-card/95 border-border',
        onSelect && 'cursor-pointer',
      )}
    >
      <ChatMessageHeader
        message={message}
        onDelete={onDelete}
        onRegenerate={onRegenerate}
        onEditResend={onEditResend}
        onFork={onFork}
        pairedUserMessage={pairedUserMessage}
        isStreaming={isStreaming}
        apiToken={apiToken}
        isFloating
        showActionsAlways
        className="px-3"
        ttsStatus={{
          isLoading: ttsLoading,
          audioUrl: ttsAudioUrl,
          error: ttsError,
          requestTTS,
          stopTTS,
        }}
      />
    </div>
  )
}
