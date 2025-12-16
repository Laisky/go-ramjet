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
    <Card className="mb-2 border-dashed bg-black/5 p-3 dark:bg-white/5">
      <details
        className="text-xs text-black/60 dark:text-white/60"
        open={!!toolEvents.length}
      >
        <summary className="cursor-pointer font-medium hover:text-black dark:hover:text-white transition-colors">
          💭 Reasoning & Tools
        </summary>

        <div className="mt-2 space-y-3">
          {/* Tool Events */}
          {toolEvents.length > 0 && (
            <div className="space-y-1 rounded bg-black/5 p-2 font-mono text-[10px] dark:bg-white/5">
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
            <pre className="whitespace-pre-wrap break-words font-sans text-black/70 dark:text-white/70">
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
        'group flex w-full gap-3 sm:gap-4',
        isUser ? 'flex-row-reverse' : 'flex-row',
      )}
    >
      {/* Avatar */}
      <div
        className={cn(
          'flex h-7 w-7 shrink-0 items-center justify-center rounded-full sm:h-8 sm:w-8',
          isUser
            ? 'bg-blue-500 text-white'
            : 'bg-gradient-to-br from-purple-500 to-pink-500 text-white',
        )}
      >
        {isUser ? <User className="h-4 w-4" /> : <Bot className="h-4 w-4" />}
      </div>

      {/* Message content */}
      <div
        className={cn(
          'flex w-full max-w-full flex-col gap-1 md:max-w-[820px]',
          isUser && 'items-end',
        )}
      >
        {/* Model badge for assistant */}
        {isAssistant && message.model && (
          <Badge variant="secondary" className="w-fit text-[11px] sm:text-xs">
            {message.model}
          </Badge>
        )}

        {/* Reasoning content (for models like o1, deepseek-reasoner) */}
        {isAssistant && message.reasoningContent && (
          <ReasoningBlock content={message.reasoningContent} />
        )}

        {/* Main content */}
        <Card
          className={cn(
            'relative w-full overflow-hidden rounded-2xl border p-3 shadow-sm transition-all sm:p-4',
            isUser
              ? 'bg-gradient-to-r from-blue-600 to-blue-500 text-white shadow-blue-500/20'
              : 'bg-white/95 text-slate-900 shadow-black/5 dark:border-slate-700 dark:bg-slate-900/80 dark:text-white',
            isStreaming && 'animate-pulse',
          )}
        >
          {isUser ? (
            <pre className="whitespace-pre-wrap break-words text-[15px] leading-relaxed sm:text-base">
              {message.content}
            </pre>
          ) : message.content ? (
            <Markdown className="prose prose-sm max-w-none break-words leading-relaxed dark:prose-invert sm:prose-base">
              {message.content}
            </Markdown>
          ) : (
            <div className="flex items-center gap-2 text-sm text-black/50 dark:text-white/50">
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

          {/* Action buttons */}
          <div
            className={cn(
              'mt-3 flex flex-wrap gap-2 text-xs md:absolute md:-bottom-8 md:mt-0 md:gap-1 md:opacity-0 md:transition-opacity md:group-hover:opacity-100',
              isUser ? 'justify-end md:right-0' : 'justify-start md:left-0',
            )}
          >
            {canEditMessage && (
              <Button
                variant="ghost"
                size="sm"
                onClick={handleEditClick}
                className="h-7 px-2 sm:h-6"
                title="Edit & resend"
              >
                <Edit2 className="h-3 w-3" />
              </Button>
            )}
            {isAssistant && onRegenerate && (
              <Button
                variant="ghost"
                size="sm"
                onClick={handleRegenerate}
                className="h-7 px-2 sm:h-6"
                disabled={actionDisabled}
                title="Regenerate response"
              >
                <RotateCcw className="h-3 w-3" />
              </Button>
            )}
            {showSpeechButton && (
              <Button
                variant="ghost"
                size="sm"
                onClick={handleToggleSpeech}
                className="h-7 px-2 sm:h-6"
                title={isSpeaking ? 'Stop narration' : 'Play narration'}
              >
                {isSpeaking ? (
                  <VolumeX className="h-3 w-3" />
                ) : (
                  <Volume2 className="h-3 w-3" />
                )}
              </Button>
            )}
            <Button
              variant="ghost"
              size="sm"
              onClick={handleCopy}
              className="h-7 px-2 sm:h-6"
            >
              {copied ? (
                <Check className="h-3 w-3 text-green-500" />
              ) : (
                <Copy className="h-3 w-3" />
              )}
            </Button>
            {onDelete && (
              <Button
                variant="ghost"
                size="sm"
                onClick={handleDelete}
                className="h-7 px-2 text-red-500 hover:text-red-600 sm:h-6"
              >
                <Trash2 className="h-3 w-3" />
              </Button>
            )}
          </div>
        </Card>

        {isAssistant && message.references && message.references.length > 0 && (
          <Card className="mt-2 border-0 bg-transparent p-0 text-xs sm:rounded-xl sm:border sm:bg-black/5 sm:p-3 dark:sm:bg-white/5">
            <p className="font-semibold text-black/60 dark:text-white/60">
              References
            </p>
            <ol className="mt-2 space-y-1">
              {message.references.map((ref) => (
                <li key={ref.index} className="flex items-start gap-2">
                  <span className="text-black/50 dark:text-white/50">
                    [{ref.index}]
                  </span>
                  <a
                    href={ref.url}
                    target="_blank"
                    rel="noreferrer"
                    className="flex-1 truncate text-blue-600 hover:underline dark:text-blue-300"
                  >
                    {ref.title || ref.url}
                  </a>
                  <button
                    className="text-black/40 transition hover:text-black dark:text-white/40 dark:hover:text-white"
                    onClick={() => handleCopyReference(ref.url, ref.index)}
                    title="Copy reference URL"
                  >
                    {copiedCitation === ref.index ? (
                      <Check className="h-3 w-3 text-green-500" />
                    ) : (
                      <Copy className="h-3 w-3" />
                    )}
                  </button>
                </li>
              ))}
            </ol>
          </Card>
        )}

        {/* Timestamp */}
        {message.timestamp && (
          <span className="text-xs text-black/40 dark:text-white/40">
            {new Date(message.timestamp).toLocaleTimeString()}
          </span>
        )}
      </div>
    </div>
  )
}
