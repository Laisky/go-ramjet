/**
 * Chat message component for displaying user and assistant messages.
 */
import { Trash2, User, Bot, Copy, Check } from 'lucide-react'
import { useState } from 'react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Markdown } from '@/components/markdown'
import { cn } from '@/utils/cn'
import type { ChatMessageData } from '../types'

export interface ChatMessageProps {
  message: ChatMessageData
  onDelete?: (chatId: string) => void
  isStreaming?: boolean
}

/**
 * ChatMessage renders a single chat message with markdown support.
 */
export function ChatMessage({ message, onDelete, isStreaming }: ChatMessageProps) {
  const [copied, setCopied] = useState(false)
  const isUser = message.role === 'user'
  const isAssistant = message.role === 'assistant'

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

  return (
    <div
      className={cn(
        'group flex gap-3',
        isUser ? 'flex-row-reverse' : 'flex-row'
      )}
    >
      {/* Avatar */}
      <div
        className={cn(
          'flex h-8 w-8 shrink-0 items-center justify-center rounded-full',
          isUser
            ? 'bg-blue-500 text-white'
            : 'bg-gradient-to-br from-purple-500 to-pink-500 text-white'
        )}
      >
        {isUser ? (
          <User className="h-4 w-4" />
        ) : (
          <Bot className="h-4 w-4" />
        )}
      </div>

      {/* Message content */}
      <div className={cn('flex max-w-[80%] flex-col gap-1', isUser && 'items-end')}>
        {/* Model badge for assistant */}
        {isAssistant && message.model && (
          <Badge variant="secondary" className="w-fit text-xs">
            {message.model}
          </Badge>
        )}

        {/* Reasoning content (for models like o1, deepseek-reasoner) */}
        {isAssistant && message.reasoningContent && (
          <Card className="mb-2 border-dashed bg-black/5 p-3 dark:bg-white/5">
            <details className="text-xs text-black/60 dark:text-white/60">
              <summary className="cursor-pointer font-medium">
                💭 Reasoning
              </summary>
              <pre className="mt-2 whitespace-pre-wrap break-words">
                {message.reasoningContent}
              </pre>
            </details>
          </Card>
        )}

        {/* Main content */}
        <Card
          className={cn(
            'relative p-3',
            isUser
              ? 'bg-blue-500 text-white'
              : 'bg-white dark:bg-black/50',
            isStreaming && 'animate-pulse'
          )}
        >
          {isUser ? (
            <pre className="whitespace-pre-wrap break-words text-sm">
              {message.content}
            </pre>
          ) : message.content ? (
            <Markdown className="prose prose-sm max-w-none dark:prose-invert">
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
              'absolute -bottom-8 flex gap-1 opacity-0 transition-opacity group-hover:opacity-100',
              isUser ? 'right-0' : 'left-0'
            )}
          >
            <Button
              variant="ghost"
              size="sm"
              onClick={handleCopy}
              className="h-6 px-2"
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
                className="h-6 px-2 text-red-500 hover:text-red-600"
              >
                <Trash2 className="h-3 w-3" />
              </Button>
            )}
          </div>
        </Card>

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
