import {
  AlertCircle,
  Bot,
  Check,
  Copy,
  Edit2,
  GitFork,
  Loader2,
  RotateCcw,
  Trash2,
  User,
  Volume2,
  VolumeX,
} from 'lucide-react'
import { useCallback, useState } from 'react'

import { Button } from '@/components/ui/button'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { cn } from '@/utils/cn'
import type { ChatAttachment, ChatMessageData } from '../types'

export interface ChatMessageHeaderProps {
  message: ChatMessageData
  onDelete?: (chatId: string) => void
  isStreaming?: boolean
  onRegenerate?: (chatId: string) => void
  onEditResend?: (payload: {
    chatId: string
    content: string
    attachments?: ChatAttachment[]
  }) => void
  onFork?: (chatId: string, role: string) => void
  pairedUserMessage?: ChatMessageData
  /** API token for TTS functionality */
  apiToken?: string
  /** TTS state from parent to ensure consistency */
  ttsStatus?: {
    isLoading: boolean
    audioUrl: string | null
    error?: string | null
    requestTTS: (text: string) => void
    stopTTS: () => void
  }
  /** Custom class name for the container */
  className?: string
  /** Whether the header is in "floating" mode */
  isFloating?: boolean
  /** Whether to always show actions (even when not hovered) */
  showActionsAlways?: boolean
}

/**
 * Shared header component for chat messages, used both inline and in the floating header.
 */
export function ChatMessageHeader({
  message,
  onDelete,
  isStreaming,
  onRegenerate,
  onEditResend,
  onFork,
  pairedUserMessage,
  apiToken,
  ttsStatus,
  className,
  isFloating,
  showActionsAlways,
}: ChatMessageHeaderProps) {
  const [copied, setCopied] = useState(false)
  const [copyError, setCopyError] = useState(false)
  const isUser = message.role === 'user'
  const isAssistant = message.role === 'assistant'

  const pairedUserContent = isUser
    ? message.content
    : pairedUserMessage?.content || ''
  const canEditMessage = Boolean(onEditResend && pairedUserContent)

  const handleCopy = useCallback(async () => {
    try {
      await navigator.clipboard.writeText(message.content)
      setCopied(true)
      setCopyError(false)
      setTimeout(() => setCopied(false), 2000)
    } catch (err) {
      console.error('Failed to copy:', err)
      setCopyError(true)
      setTimeout(() => setCopyError(false), 3000)
    }
  }, [message.content])

  const handleDelete = useCallback(() => {
    if (onDelete) {
      onDelete(message.chatID)
    }
  }, [message.chatID, onDelete])

  const handleFork = useCallback(() => {
    if (onFork) {
      onFork(message.chatID, message.role)
    }
  }, [message.chatID, message.role, onFork])

  const handleRegenerate = useCallback(() => {
    if (onRegenerate) {
      onRegenerate(message.chatID)
    }
  }, [message.chatID, onRegenerate])

  const handleEditClick = useCallback(() => {
    if (canEditMessage && onEditResend) {
      onEditResend({
        chatId: message.chatID,
        content: pairedUserContent,
        attachments: isUser
          ? message.attachments
          : pairedUserMessage?.attachments,
      })
    }
  }, [
    canEditMessage,
    message.chatID,
    message.attachments,
    onEditResend,
    pairedUserContent,
    isUser,
    pairedUserMessage?.attachments,
  ])

  const handleToggleSpeech = useCallback(() => {
    if (!apiToken || !message.content || !ttsStatus) return
    if (ttsStatus.audioUrl) {
      ttsStatus.stopTTS()
    } else {
      ttsStatus.requestTTS(message.content)
    }
  }, [apiToken, message.content, ttsStatus])

  const showSpeechButton = Boolean(apiToken && isAssistant && message.content)
  const actionDisabled = Boolean(isStreaming && isAssistant)

  return (
    <TooltipProvider>
      <div
        className={cn(
          'flex flex-wrap items-center gap-2 px-2 py-1.5 text-xs transition-all',
          !isFloating &&
            'mb-2 -mx-2 -mt-1.5 rounded-t-md bg-inherit border-b border-border/10',
          className,
        )}
      >
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
        <span className={cn('font-semibold', 'text-foreground')}>
          {isUser ? 'You' : 'Assistant'}
        </span>
        {message.timestamp && (
          <span className="text-[11px] text-muted-foreground">
            {new Date(message.timestamp).toLocaleTimeString()}
          </span>
        )}

        <div
          className={cn(
            'ml-auto flex flex-wrap items-center gap-1 text-[11px] transition-opacity',
            showActionsAlways
              ? 'opacity-100'
              : 'opacity-100 md:opacity-0 md:group-hover/message:opacity-100 md:group-focus-within/message:opacity-100',
          )}
        >
          {canEditMessage && (
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={handleEditClick}
                  className="h-7 w-7 rounded-md p-0"
                  title="Edit & resend"
                >
                  <Edit2 className="h-3.5 w-3.5" />
                </Button>
              </TooltipTrigger>
              <TooltipContent side="top">Edit & resend</TooltipContent>
            </Tooltip>
          )}
          {isAssistant && onRegenerate && (
            <Tooltip>
              <TooltipTrigger asChild>
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
              </TooltipTrigger>
              <TooltipContent side="top">Regenerate response</TooltipContent>
            </Tooltip>
          )}
          {showSpeechButton && (
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={handleToggleSpeech}
                  className={cn(
                    'h-7 w-7 rounded-md p-0',
                    ttsStatus?.error &&
                      'text-destructive hover:text-destructive',
                  )}
                  disabled={ttsStatus?.isLoading}
                  title={
                    ttsStatus?.isLoading
                      ? 'Loading audio...'
                      : ttsStatus?.error
                        ? `TTS Error: ${ttsStatus.error}`
                        : ttsStatus?.audioUrl
                          ? 'Stop narration'
                          : 'Play narration'
                  }
                >
                  {ttsStatus?.isLoading ? (
                    <Loader2 className="h-3.5 w-3.5 animate-spin" />
                  ) : ttsStatus?.error ? (
                    <AlertCircle className="h-3.5 w-3.5 text-destructive" />
                  ) : ttsStatus?.audioUrl ? (
                    <VolumeX className="h-3.5 w-3.5" />
                  ) : (
                    <Volume2 className="h-3.5 w-3.5" />
                  )}
                </Button>
              </TooltipTrigger>
              <TooltipContent side="top">
                {ttsStatus?.isLoading
                  ? 'Loading audio...'
                  : ttsStatus?.error
                    ? `TTS Error: ${ttsStatus.error}`
                    : ttsStatus?.audioUrl
                      ? 'Stop narration'
                      : 'Play narration'}
              </TooltipContent>
            </Tooltip>
          )}
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                size="sm"
                onClick={handleCopy}
                className={cn(
                  'h-7 w-7 rounded-md p-0',
                  copyError && 'text-destructive hover:text-destructive',
                )}
                title={
                  copyError
                    ? 'Failed to copy'
                    : copied
                      ? 'Copied!'
                      : 'Copy message'
                }
              >
                {copyError ? (
                  <AlertCircle className="h-3.5 w-3.5" />
                ) : copied ? (
                  <Check className="h-3.5 w-3.5 text-success" />
                ) : (
                  <Copy className="h-3.5 w-3.5" />
                )}
              </Button>
            </TooltipTrigger>
            <TooltipContent side="top">
              {copyError
                ? 'Failed to copy'
                : copied
                  ? 'Copied!'
                  : 'Copy message'}
            </TooltipContent>
          </Tooltip>

          {onFork && (
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={handleFork}
                  className="h-7 w-7 rounded-md p-0"
                  title="Fork session"
                >
                  <GitFork className="h-3.5 w-3.5" />
                </Button>
              </TooltipTrigger>
              <TooltipContent side="top">Fork session from here</TooltipContent>
            </Tooltip>
          )}

          {onDelete && (
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={handleDelete}
                  className="h-7 w-7 rounded-md p-0 text-destructive"
                  title="Delete message"
                >
                  <Trash2 className="h-3.5 w-3.5" />
                </Button>
              </TooltipTrigger>
              <TooltipContent side="top">Delete message</TooltipContent>
            </Tooltip>
          )}
        </div>
      </div>
    </TooltipProvider>
  )
}
