/**
 * ChatSearch component for fuzzy searching messages in the current session.
 */
import { Command, Search } from 'lucide-react'
import React, { useCallback, useEffect, useRef, useState } from 'react'

import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { cn } from '@/utils/cn'
import type { ChatMessageData } from '../types'

interface ChatSearchProps {
  messages: ChatMessageData[]
  onSelectMessage: (chatId: string, role: string) => void
  onClose?: () => void
}

/**
 * ChatSearch provides a command-palette style search interface for messages.
 */
export function ChatSearch({ messages, onSelectMessage, onClose }: ChatSearchProps) {
  const [isOpen, setIsOpen] = useState(false)
  const [query, setQuery] = useState('')
  const [results, setResults] = useState<ChatMessageData[]>([])
  const [selectedIndex, setSelectedIndex] = useState(0)
  const inputRef = useRef<HTMLInputElement>(null)

  // Open on Ctrl+K/Cmd+K
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key === 'k') {
        e.preventDefault()
        setIsOpen(true)
      } else if (e.key === 'Escape') {
        setIsOpen(false)
      }
    }
    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [])

  // Focus input when opened
  useEffect(() => {
    if (isOpen) {
      setTimeout(() => inputRef.current?.focus(), 10)
    } else {
      setQuery('')
      setResults([])
    }
  }, [isOpen])

  // Simple fuzzy search (matches all words)
  useEffect(() => {
    if (!query.trim()) {
      setResults([])
      return
    }

    const words = query.toLowerCase().trim().split(/\s+/)
    const filtered = messages
      .filter((m) => {
        const content = m.content.toLowerCase()
        return words.every((word) => content.includes(word))
      })
      .reverse() // Newest results first
      .slice(0, 50) // Limit results

    setResults(filtered)
    setSelectedIndex(0)
  }, [query, messages])

  const handleSelect = useCallback(
    (msg: ChatMessageData) => {
      onSelectMessage(msg.chatID, msg.role)
      setIsOpen(false)
      onClose?.()
    },
    [onSelectMessage, onClose],
  )

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      setSelectedIndex((prev) => (prev + 1) % results.length)
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      setSelectedIndex((prev) => (prev - 1 + results.length) % results.length)
    } else if (e.key === 'Enter' && results.length > 0) {
      e.preventDefault()
      handleSelect(results[selectedIndex])
    }
  }

  return (
    <>
      <Button
        variant="ghost"
        size="sm"
        onClick={() => setIsOpen(true)}
        className="h-9 w-9 rounded-md px-0"
        title="Search messages (Ctrl+K)"
      >
        <Search className="h-4 w-4" />
      </Button>

      {isOpen && (
        <div className="fixed inset-0 z-50 flex items-start justify-center bg-black/40 p-4 pt-[10dvh] backdrop-blur-[2px]">
          <div
            className="fixed inset-0"
            onClick={() => setIsOpen(false)}
          />
          <Card className="theme-surface theme-border relative z-10 w-full max-w-xl overflow-hidden rounded-xl border shadow-2xl">
            <div className="flex items-center border-b px-3">
              <Search className="mr-2 h-4 w-4 shrink-0 opacity-50" />
              <input
                ref={inputRef}
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                onKeyDown={handleKeyDown}
                placeholder="Search messages..."
                className="flex h-12 w-full rounded-md bg-transparent py-3 text-sm outline-none placeholder:text-muted-foreground disabled:cursor-not-allowed disabled:opacity-50"
              />
              <div className="flex items-center gap-1.5 rounded border bg-muted px-1.5 py-0.5 font-mono text-[10px] font-medium text-muted-foreground opacity-100">
                <span className="text-xs">Esc</span>
              </div>
            </div>

            <div className="max-h-[60dvh] overflow-y-auto p-2">
              {results.length > 0 ? (
                <div className="space-y-1">
                  {results.map((msg, i) => (
                    <div
                      key={`${msg.chatID}-${msg.role}`}
                      className={cn(
                        'flex cursor-pointer flex-col rounded-md px-3 py-2 transition-colors',
                        i === selectedIndex
                          ? 'bg-accent text-accent-foreground'
                          : 'hover:bg-muted/50',
                      )}
                      onClick={() => handleSelect(msg)}
                    >
                      <div className="flex items-center gap-2 mb-1">
                        <span className={cn(
                          "px-1.5 py-0.5 rounded-[4px] text-[10px] font-bold uppercase tracking-wider",
                          msg.role === 'user' ? "bg-primary/20 text-primary" : "bg-muted text-muted-foreground"
                        )}>
                          {msg.role}
                        </span>
                        {msg.timestamp && (
                          <span className="text-[10px] opacity-50">
                            {new Date(msg.timestamp).toLocaleString()}
                          </span>
                        )}
                      </div>
                      <div className="line-clamp-2 text-xs leading-relaxed opacity-90">
                        {msg.content}
                      </div>
                    </div>
                  ))}
                </div>
              ) : query.trim() ? (
                <div className="py-6 text-center text-sm text-muted-foreground">
                  No messages found.
                </div>
              ) : (
                <div className="py-6 text-center text-sm text-muted-foreground">
                  Start typing to search in this conversation...
                </div>
              )}
            </div>

            <div className="flex items-center justify-between border-t bg-muted/30 px-3 py-2 text-[10px] text-muted-foreground">
              <div className="flex items-center gap-3">
                <span className="flex items-center gap-1">
                  <kbd className="rounded border bg-background px-1">↑↓</kbd> navigate
                </span>
                <span className="flex items-center gap-1">
                  <kbd className="rounded border bg-background px-1">Enter</kbd> select
                </span>
              </div>
              <div className="flex items-center gap-1">
                <Command className="h-3 w-3" />
                <span>Total {messages.length} messages</span>
              </div>
            </div>
          </Card>
        </div>
      )}
    </>
  )
}
