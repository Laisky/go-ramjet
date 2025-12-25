/**
 * Chat message component for displaying user and assistant messages.
 */
import {
  Bot,
  Check,
  Copy,
  Edit2,
  RotateCcw,
  Trash2,
  User,
  Volume2,
  VolumeX,
} from 'lucide-react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'

import { Markdown } from '@/components/markdown'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { splitReasoningContent } from '@/utils/chat-parser'
import { cn } from '@/utils/cn'
import type { ChatMessageData } from '../types'

function stripMarkdownText(input: string): string {
  if (typeof input !== 'string') {
    return ''
  }
  return input
    .replace(/```[\s\S]*?```/g, ' ')
    .replace(/`[^`]*`/g, ' ')
    .replace(/!\[[^\]]*\]\([^)]*\)/g, '')
    .replace(/\[([^\]]+)\]\([^)]*\)/g, '$1')
    .replace(/[>*_~`#]/g, '')
    .replace(/\s+/g, ' ')
    .trim()
}

export interface ChatMessageProps {
  message: ChatMessageData
  onDelete?: (chatId: string) => void
  isStreaming?: boolean
  onRegenerate?: (chatId: string) => void
  onEditResend?: (payload: { chatId: string; content: string }) => void
  pairedUserMessage?: ChatMessageData
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
}: ChatMessageProps) {
  const [copied, setCopied] = useState(false)
  const [copiedCitation, setCopiedCitation] = useState<number | null>(null)
  const [isSpeaking, setIsSpeaking] = useState(false)
  const speechRef = useRef<SpeechSynthesisUtterance | null>(null)
  const isUser = message.role === 'user'
  const isAssistant = message.role === 'assistant'
  const userHasCode = useMemo(
    () => /```/.test(message.content) || /`[^`]/.test(message.content),
    [message.content],
  )
  const supportsSpeech = useMemo(
    () =>
      typeof window !== 'undefined' &&
      'speechSynthesis' in window &&
      'SpeechSynthesisUtterance' in window,
    [],
  )
  const pairedUserContent = isUser
    ? message.content
    : pairedUserMessage?.content || ''
  const canEditMessage = Boolean(onEditResend && pairedUserContent)

  const stopSpeaking = useCallback(() => {
    if (!supportsSpeech || !isSpeaking) return
    window.speechSynthesis.cancel()
    speechRef.current = null
    setIsSpeaking(false)
  }, [isSpeaking, supportsSpeech])

  useEffect(() => {
    return () => {
      stopSpeaking()
    }
  }, [stopSpeaking])

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

  const showSpeechButton = Boolean(
    supportsSpeech && isAssistant && message.content,
  )
  const actionDisabled = Boolean(isStreaming && isAssistant)

  const handleToggleSpeech = useCallback(() => {
    if (!supportsSpeech || !message.content) {
      return
    }
    if (isSpeaking) {
      stopSpeaking()
      return
    }
    if (
      typeof window === 'undefined' ||
      typeof window.SpeechSynthesisUtterance === 'undefined'
    ) {
      return
    }
    const plain = stripMarkdownText(message.content)
    if (!plain) {
      return
    }
    stopSpeaking()
    const utterance = new window.SpeechSynthesisUtterance(plain)
    utterance.onend = () => setIsSpeaking(false)
    utterance.onerror = () => setIsSpeaking(false)
    speechRef.current = utterance
    window.speechSynthesis.speak(utterance)
    setIsSpeaking(true)
  }, [isSpeaking, message.content, stopSpeaking, supportsSpeech])

  const handleRegenerate = useCallback(() => {
    if (onRegenerate) {
      onRegenerate(message.chatID)
    }
  }, [message.chatID, onRegenerate])

  const handleEditClick = useCallback(() => {
    if (canEditMessage && onEditResend) {
      onEditResend({ chatId: message.chatID, content: pairedUserContent })
    }
  }, [canEditMessage, message.chatID, onEditResend, pairedUserContent])

  return (
    <div
      className={cn(
        'w-full',
        isUser ? 'flex justify-end' : 'flex justify-start',
      )}
    >
      <Card
        className={cn(
          'group/message relative w-full max-w-full rounded-md border px-2 py-1.5 transition-all sm:w-fit sm:max-w-[92%] sm:px-2.5 sm:py-2 md:max-w-[880px]',
          isUser
            ? 'ml-auto rounded-br-sm border-primary/20 bg-primary/10 text-foreground'
            : 'bg-card text-card-foreground border-border mr-auto rounded-bl-sm',
          isStreaming && 'animate-pulse',
        )}
      >
        <div className="flex items-start gap-2">
          <div
            className={cn(
              'flex h-6 w-6 shrink-0 items-center justify-center rounded-md text-xs',
              isUser
                ? 'bg-primary text-primary-foreground'
                : 'bg-muted text-muted-foreground',
            )}
          >
            {isUser ? (
              <User className="h-4 w-4" />
            ) : (
              <Bot className="h-4 w-4" />
            )}
          </div>

          <div className="min-w-0 flex-1 space-y-1">
            <div className="flex flex-wrap items-center gap-2 text-xs">
              <span
                className={cn(
                  'font-semibold',
                  'text-foreground',
                )}
              >
                {isUser ? 'You' : 'Assistant'}
              </span>
              {isAssistant && message.model && (
                <Badge
                  variant="secondary"
                  className="h-6 rounded-md bg-muted px-2 text-[11px] text-muted-foreground"
                >
                  {message.model}
                </Badge>
              )}
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
                    title={isSpeaking ? 'Stop narration' : 'Play narration'}
                  >
                    {isSpeaking ? (
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
              <pre
                className={cn(
                  'whitespace-pre-wrap break-words text-[15px] leading-relaxed sm:text-base',
                  userHasCode ? 'font-mono' : 'font-sans',
                )}
              >
                {message.content}
              </pre>
            ) : message.content ? (
              <Markdown className="prose prose-sm max-w-none break-words leading-relaxed dark:prose-invert sm:prose-base">
                {message.content}
              </Markdown>
            ) : (
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
            )}

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
                          onClick={() =>
                            handleCopyReference(ref.url, ref.index)
                          }
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
          </div>
        </div>
      </Card>
    </div>
  )
}
