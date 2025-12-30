/**
 * Floating message header component that shows action buttons for the
 * currently visible message when its header has scrolled out of view.
 */
import {
  Bot,
  Check,
  Copy,
  Edit2,
  Loader2,
  RotateCcw,
  Trash2,
  User,
  Volume2,
  VolumeX,
} from 'lucide-react'
import { useCallback, useEffect, useState } from 'react'

import { Button } from '@/components/ui/button'
import { cn } from '@/utils/cn'
import { useTTS } from '../hooks/use-tts'
import type { ChatMessageData } from '../types'

export interface FloatingMessageHeaderProps {
  /** The message to display header for */
  message: ChatMessageData | null
  /** Whether the floating header should be visible */
  visible: boolean
  /** Callback to delete the message */
  onDelete?: (chatId: string) => void
  /** Callback to regenerate the message */
  onRegenerate?: (chatId: string) => void
  /** Callback for edit and resend */
  onEditResend?: (payload: { chatId: string; content: string }) => void
  /** The paired user message (for edit/resend) */
  pairedUserMessage?: ChatMessageData
  /** Whether the message is streaming */
  isStreaming?: boolean
  /** Scroll container ref for positioning */
  containerRef?: React.RefObject<HTMLElement>
  /** API token for TTS functionality */
  apiToken?: string
}

/**
 * FloatingMessageHeader renders a fixed header bar with action buttons
 * for the currently visible message when its inline header has scrolled
 * out of view.
 */
export function FloatingMessageHeader({
  message,
  visible,
  onDelete,
  onRegenerate,
  onEditResend,
  pairedUserMessage,
  isStreaming,
  apiToken,
}: FloatingMessageHeaderProps) {
  const [copied, setCopied] = useState(false)

  const isUser = message?.role === 'user'
  const isAssistant = message?.role === 'assistant'

  // TTS hook - uses server-side Azure TTS
  const {
    isLoading: ttsLoading,
    audioUrl: ttsAudioUrl,
    requestTTS,
    stopTTS,
  } = useTTS({
    apiToken: apiToken || '',
  })

  // Cleanup TTS when message changes
  useEffect(() => {
    stopTTS()
  }, [message?.chatID]) // eslint-disable-line react-hooks/exhaustive-deps

  // Cleanup on unmount
  useEffect(() => {
    return () => {
      stopTTS()
    }
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  const pairedUserContent = isUser
    ? message?.content || ''
    : pairedUserMessage?.content || ''
  const canEditMessage = Boolean(onEditResend && pairedUserContent)
  // Show TTS button for assistant messages when API token is available
  const showSpeechButton = Boolean(apiToken && isAssistant && message?.content)
  const actionDisabled = Boolean(isStreaming && isAssistant)

  const handleCopy = useCallback(async () => {
    if (!message?.content) return
    try {
      await navigator.clipboard.writeText(message.content)
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    } catch (err) {
      console.error('Failed to copy:', err)
    }
  }, [message?.content])

  const handleDelete = useCallback(() => {
    if (onDelete && message) {
      onDelete(message.chatID)
    }
  }, [message, onDelete])

  const handleToggleSpeech = useCallback(() => {
    if (!apiToken || !message?.content) {
      console.debug('[FloatingMessageHeader] TTS not available:', {
        hasApiToken: !!apiToken,
        hasContent: !!message?.content,
      })
      return
    }
    if (ttsAudioUrl) {
      // Audio already loaded - stop it
      stopTTS()
      return
    }
    // Request new TTS audio
    requestTTS(message.content)
  }, [apiToken, message?.content, ttsAudioUrl, stopTTS, requestTTS])

  const handleRegenerate = useCallback(() => {
    if (onRegenerate && message) {
      onRegenerate(message.chatID)
    }
  }, [message, onRegenerate])

  const handleEditClick = useCallback(() => {
    if (canEditMessage && onEditResend && message) {
      onEditResend({ chatId: message.chatID, content: pairedUserContent })
    }
  }, [canEditMessage, message, onEditResend, pairedUserContent])

  if (!message || !visible) {
    return null
  }

  return (
    <div
      className={cn(
        'fixed left-10 right-0 top-12 z-50 flex items-center gap-2 border-b px-3 py-1.5 text-xs shadow-sm backdrop-blur-sm transition-all duration-200',
        visible
          ? 'translate-y-0 opacity-100'
          : '-translate-y-full opacity-0 pointer-events-none',
        isUser ? 'bg-primary/10 border-primary/20' : 'bg-card/95 border-border',
      )}
    >
      {/* Role icon */}
      <div
        className={cn(
          'flex h-5 w-5 shrink-0 items-center justify-center rounded-md',
          isUser
            ? 'bg-primary text-primary-foreground'
            : 'bg-muted text-muted-foreground',
        )}
      >
        {isUser ? <User className="h-3 w-3" /> : <Bot className="h-3 w-3" />}
      </div>

      {/* Role name */}
      <span className="font-semibold text-foreground">
        {isUser ? 'You' : 'Assistant'}
      </span>

      {/* Timestamp */}
      {message.timestamp && (
        <span className="text-[11px] text-muted-foreground">
          {new Date(message.timestamp).toLocaleTimeString()}
        </span>
      )}

      {/* Action buttons */}
      <div className="ml-auto flex items-center gap-1">
        {canEditMessage && (
          <Button
            variant="ghost"
            size="sm"
            onClick={handleEditClick}
            className="h-7 w-7 rounded-md p-0"
            title="Edit & resend"
          >
            <Edit2 className="h-3.5 w-3.5" />
          </Button>
        )}
        {isAssistant && onRegenerate && (
          <Button
            variant="ghost"
            size="sm"
            onClick={handleRegenerate}
            className="h-7 w-7 rounded-md p-0"
            disabled={actionDisabled}
            title="Regenerate response"
          >
            <RotateCcw className="h-3.5 w-3.5" />
          </Button>
        )}
        {showSpeechButton && (
          <Button
            variant="ghost"
            size="sm"
            onClick={handleToggleSpeech}
            className="h-7 w-7 rounded-md p-0"
            disabled={ttsLoading}
            title={
              ttsLoading
                ? 'Loading audio...'
                : ttsAudioUrl
                  ? 'Stop narration'
                  : 'Play narration'
            }
          >
            {ttsLoading ? (
              <Loader2 className="h-3.5 w-3.5 animate-spin" />
            ) : ttsAudioUrl ? (
              <VolumeX className="h-3.5 w-3.5" />
            ) : (
              <Volume2 className="h-3.5 w-3.5" />
            )}
          </Button>
        )}
        <Button
          variant="ghost"
          size="sm"
          onClick={handleCopy}
          className="h-7 w-7 rounded-md p-0"
          title="Copy message"
        >
          {copied ? (
            <Check className="h-3.5 w-3.5 text-success" />
          ) : (
            <Copy className="h-3.5 w-3.5" />
          )}
        </Button>
        {onDelete && (
          <Button
            variant="ghost"
            size="sm"
            onClick={handleDelete}
            className="h-7 w-7 rounded-md p-0 text-destructive"
            title="Delete message"
          >
            <Trash2 className="h-3.5 w-3.5" />
          </Button>
        )}
      </div>
    </div>
  )
}
