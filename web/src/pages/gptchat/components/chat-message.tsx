/**
 * Chat message component for displaying user and assistant messages.
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

import { Markdown } from '@/components/markdown'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { splitReasoningContent } from '@/utils/chat-parser'
import { cn } from '@/utils/cn'
import { useTTS } from '../hooks/use-tts'
import type { ChatAttachment, ChatMessageData } from '../types'
import { formatCostUsd } from '../utils/format'
import { TTSAudioPlayer } from './tts-audio-player'

export interface ChatMessageProps {
  message: ChatMessageData
  onDelete?: (chatId: string) => void
  isStreaming?: boolean
  onRegenerate?: (chatId: string) => void
  onEditResend?: (payload: {
    chatId: string
    content: string
    attachments?: ChatAttachment[]
  }) => void
  pairedUserMessage?: ChatMessageData
  isSelected?: boolean
  /** Called when user clicks the message to toggle selection */
  onSelect?: (index: number) => void
  /** The index of this message in the list (used for selection) */
  messageIndex?: number
  /** API token for TTS functionality */
  apiToken?: string
}

function ReasoningBlock({ content }: { content: string }) {
  const { thinking, toolEvents } = splitReasoningContent(content)

  return (
    <Card className="mb-2 border-dashed bg-muted p-3">
      <details
        className="text-xs text-muted-foreground"
        open={!!toolEvents.length}
      >
        <summary className="cursor-pointer font-medium hover:text-foreground transition-colors">
          💭 Reasoning & Tools
        </summary>

        <div className="mt-2 space-y-3">
          {/* Tool Events */}
          {toolEvents.length > 0 && (
            <div className="space-y-1 rounded bg-muted p-2 font-mono text-[10px]">
              {toolEvents.map((evt, i) => (
                <div key={i} className="flex gap-2">
                  <span className="shrink-0 opacity-50">🔧</span>
                  <span>{evt}</span>
                </div>
              ))}
            </div>
          )}

          {/* Thinking Content */}
          {thinking && (
            <pre className="whitespace-pre-wrap break-words font-sans text-muted-foreground">
              {thinking}
            </pre>
          )}
        </div>
      </details>
    </Card>
  )
}

/**
 * ChatMessage renders a single chat message with markdown support.
 */
export function ChatMessage({
  message,
  onDelete,
  isStreaming,
  onRegenerate,
  onEditResend,
  pairedUserMessage,
  isSelected,
  onSelect,
  messageIndex,
  apiToken,
}: ChatMessageProps) {
  const [copied, setCopied] = useState(false)
  const [copiedCitation, setCopiedCitation] = useState<number | null>(null)
  const isUser = message.role === 'user'
  const isAssistant = message.role === 'assistant'

  // TTS hook - uses server-side Azure TTS
  const {
    isLoading: ttsLoading,
    audioUrl: ttsAudioUrl,
    error: ttsError,
    requestTTS,
    stopTTS,
  } = useTTS({
    apiToken: apiToken || '',
  })

  // Cleanup TTS when message changes
  useEffect(() => {
    return () => {
      stopTTS()
    }
  }, [message.chatID]) // eslint-disable-line react-hooks/exhaustive-deps

  const pairedUserContent = isUser
    ? message.content
    : pairedUserMessage?.content || ''
  const canEditMessage = Boolean(onEditResend && pairedUserContent)

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(message.content)
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    } catch (err) {
      console.error('Failed to copy:', err)
    }
  }

  const handleDelete = () => {
    if (onDelete) {
      onDelete(message.chatID)
    }
  }

  const handleCopyReference = async (url: string, index: number) => {
    try {
      await navigator.clipboard.writeText(url)
      setCopiedCitation(index)
      setTimeout(() => setCopiedCitation(null), 2000)
    } catch (err) {
      console.error('Failed to copy reference:', err)
    }
  }

  // Show TTS button for assistant messages when API token is available
  const showSpeechButton = Boolean(apiToken && isAssistant && message.content)
  const actionDisabled = Boolean(isStreaming && isAssistant)

  const handleToggleSpeech = useCallback(() => {
    if (!apiToken || !message.content) {
      console.debug('[ChatMessage] TTS not available:', {
        hasApiToken: !!apiToken,
        hasContent: !!message.content,
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
  }, [apiToken, message.content, ttsAudioUrl, stopTTS, requestTTS])

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

  const handleCardClick = useCallback(
    (e: React.MouseEvent) => {
      // Don't toggle selection if clicking on interactive elements
      // Note: We allow clicks on <pre> (user message content) and inline <code>
      // Only block clicks on code blocks that might have copy buttons, etc.
      const target = e.target as HTMLElement
      if (
        target.closest('button') ||
        target.closest('a') ||
        target.closest('details')
      ) {
        return
      }

      // If there's a selection, don't toggle message selection
      const selection = window.getSelection()
      if (selection && selection.toString().trim().length > 0) {
        return
      }

      if (onSelect !== undefined && messageIndex !== undefined) {
        onSelect(messageIndex)
      }
    },
    [onSelect, messageIndex],
  )

  return (
    <div
      id={`chat-message-${message.chatID}-${message.role}`}
      className={cn(
        'w-full',
        isUser ? 'flex justify-end' : 'flex justify-start',
      )}
    >
      <Card
        onClick={handleCardClick}
        className={cn(
          'group/message relative w-full max-w-full rounded-md border px-2 py-1.5 transition-all sm:w-fit sm:max-w-[92%] sm:px-2.5 sm:py-2 md:max-w-[880px]',
          isUser
            ? 'ml-auto rounded-br-sm border-primary/20 bg-primary/10 text-foreground'
            : 'bg-card text-card-foreground border-border mr-auto rounded-bl-sm',
          isStreaming && 'animate-pulse',
          isSelected &&
            'ring-2 ring-primary ring-offset-2 dark:ring-offset-background',
          onSelect && 'cursor-pointer',
        )}
      >
        <div className="space-y-1">
          <div
            className={cn(
              'sticky top-12 z-10 -mx-2 -mt-1.5 mb-1 flex flex-wrap items-center gap-2 rounded-t-md bg-inherit px-2 py-1.5 text-xs backdrop-blur-sm transition-all',
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
              {isUser ? (
                <User className="h-3 w-3" />
              ) : (
                <Bot className="h-3 w-3" />
              )}
            </div>
            <span className={cn('font-semibold', 'text-foreground')}>
              {isUser ? 'You' : 'Assistant'}
            </span>
            {message.timestamp && (
              <span className="text-[11px] text-muted-foreground">
                {new Date(message.timestamp).toLocaleTimeString()}
              </span>
            )}

            <div className="ml-auto flex flex-wrap items-center gap-1 text-[11px] opacity-100 transition-opacity md:opacity-0 md:group-hover/message:opacity-100 md:group-focus-within/message:opacity-100">
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

          {isAssistant && message.reasoningContent && (
            <ReasoningBlock content={message.reasoningContent} />
          )}

          {isUser ? (
            <div className="space-y-2">
              {message.content && (
                <Markdown className="prose prose-sm max-w-none break-words leading-relaxed dark:prose-invert sm:prose-base">
                  {message.content}
                </Markdown>
              )}
              {message.attachments && message.attachments.length > 0 && (
                <div className="flex flex-wrap gap-2 mt-2">
                  {message.attachments.map((att, i) =>
                    att.type === 'image' && att.contentB64 ? (
                      <div
                        key={i}
                        className="relative group/img max-w-[300px] rounded-lg overflow-hidden border border-border bg-muted"
                      >
                        <img
                          src={att.contentB64}
                          alt={att.filename || 'image'}
                          className="w-full h-auto object-contain max-h-[300px]"
                          onError={(e) => {
                            console.error('Image failed to load', att.filename)
                            e.currentTarget.style.display = 'none'
                          }}
                        />
                      </div>
                    ) : null,
                  )}
                </div>
              )}
            </div>
          ) : message.error ? (
            <div className="space-y-3">
              <div className="rounded-md bg-destructive/10 p-3 text-sm text-destructive border border-destructive/20">
                <p className="font-semibold mb-1">Error</p>
                {message.error}
              </div>
              {onRegenerate && (
                <Button
                  variant="outline"
                  size="sm"
                  onClick={handleRegenerate}
                  className="flex items-center gap-2"
                >
                  <RotateCcw className="h-4 w-4" />
                  Retry
                </Button>
              )}
            </div>
          ) : message.content ? (
            <Markdown className="prose prose-sm max-w-none break-words leading-relaxed dark:prose-invert sm:prose-base">
              {message.content}
            </Markdown>
          ) : isStreaming ? (
            <div className="flex items-center gap-2 text-sm text-muted-foreground">
              <div className="h-2 w-2 animate-bounce rounded-full bg-current" />
              <div
                className="h-2 w-2 animate-bounce rounded-full bg-current"
                style={{ animationDelay: '0.1s' }}
              />
              <div
                className="h-2 w-2 animate-bounce rounded-full bg-current"
                style={{ animationDelay: '0.2s' }}
              />
            </div>
          ) : null}

          {isAssistant &&
            message.references &&
            message.references.length > 0 && (
              <Card className="mt-2 border-0 bg-transparent p-0 text-xs sm:rounded-xl sm:border sm:bg-muted sm:p-3">
                <p className="font-semibold text-muted-foreground">
                  References
                </p>
                <ol className="mt-2 space-y-1">
                  {message.references.map((ref) => (
                    <li key={ref.index} className="flex items-start gap-2">
                      <span className="text-muted-foreground">
                        [{ref.index}]
                      </span>
                      <a
                        href={ref.url}
                        target="_blank"
                        rel="noreferrer"
                        className="flex-1 truncate text-primary hover:underline"
                      >
                        {ref.title || ref.url}
                      </a>
                      <button
                        className="text-muted-foreground/50 transition hover:text-foreground"
                        onClick={() => handleCopyReference(ref.url, ref.index)}
                        title="Copy reference URL"
                      >
                        {copiedCitation === ref.index ? (
                          <Check className="h-3 w-3 text-success" />
                        ) : (
                          <Copy className="h-3 w-3" />
                        )}
                      </button>
                    </li>
                  ))}
                </ol>
              </Card>
            )}

          {/* TTS Audio Player - shown when audio is loaded */}
          {isAssistant && ttsAudioUrl && (
            <TTSAudioPlayer audioUrl={ttsAudioUrl} onClose={stopTTS} />
          )}

          {/* TTS Error Message */}
          {isAssistant && ttsError && (
            <div className="mt-1 text-xs text-destructive">
              TTS Error: {ttsError}
            </div>
          )}

          {isAssistant && (
            <div className="mt-1 flex items-center justify-end gap-2 text-[10px] text-muted-foreground/60">
              {message.model && <span>{message.model}</span>}
              {(() => {
                const formattedCost = formatCostUsd(message.costUsd)
                return formattedCost ? <span>${formattedCost}</span> : null
              })()}
            </div>
          )}
        </div>
      </Card>
    </div>
  )
}
