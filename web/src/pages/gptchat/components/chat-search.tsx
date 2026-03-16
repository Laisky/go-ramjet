/**
 * ChatSearch component for fuzzy searching messages across sessions.
 */
import { Check, ChevronDown, Command, Filter, Search } from 'lucide-react'
import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { createPortal } from 'react-dom'

import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { TooltipWrapper } from '@/components/ui/tooltip-wrapper'
import { cn } from '@/utils/cn'
import { kvGet } from '@/utils/storage'
import { sanitizeChatMessageData } from '../hooks/chat-storage'
import type { ChatMessageData, SessionHistoryItem } from '../types'
import { getChatDataKey, getSessionHistoryKey } from '../utils/chat-storage'

interface SearchResultItem {
  message: ChatMessageData
  sessionId: number
  sessionName: string
}

interface ChatSearchProps {
  messages: ChatMessageData[]
  sessions: { id: number; name: string; visible: boolean }[]
  currentSessionId: number
  onSelectMessage: (chatId: string, role: string) => void
  onSwitchAndSelect?: (sessionId: number, chatId: string, role: string) => void
  onClose?: () => void
}

/**
 * Load all messages for a session directly from storage.
 */
async function loadSessionMessagesFromStorage(
  sessionId: number,
): Promise<ChatMessageData[]> {
  const key = getSessionHistoryKey(sessionId)
  const history = await kvGet<SessionHistoryItem[]>(key)
  if (!history || history.length === 0) return []

  const msgs: ChatMessageData[] = []
  const seenChatIds = new Set<string>()

  for (const item of history) {
    if (seenChatIds.has(item.chatID)) continue
    seenChatIds.add(item.chatID)

    const userKey = getChatDataKey(item.chatID, 'user')
    const assistantKey = getChatDataKey(item.chatID, 'assistant')

    const userData = await kvGet<ChatMessageData>(userKey)
    const assistantData = await kvGet<ChatMessageData>(assistantKey)

    if (userData && typeof userData === 'object' && userData.content) {
      msgs.push(sanitizeChatMessageData(userData))
    }
    if (
      assistantData &&
      typeof assistantData === 'object' &&
      assistantData.content
    ) {
      msgs.push(sanitizeChatMessageData(assistantData))
    }
  }

  return msgs
}

/**
 * ChatSearch provides a command-palette style search interface for messages
 * with optional multi-session search scope.
 */
export function ChatSearch({
  messages,
  sessions,
  currentSessionId,
  onSelectMessage,
  onSwitchAndSelect,
  onClose,
}: ChatSearchProps) {
  const [isOpen, setIsOpen] = useState(false)
  const [query, setQuery] = useState('')
  const [results, setResults] = useState<SearchResultItem[]>([])
  const [selectedIndex, setSelectedIndex] = useState(0)
  const [selectedSessionIds, setSelectedSessionIds] = useState<Set<number>>(
    () => new Set(sessions.map((s) => s.id)),
  )
  const [showSessionFilter, setShowSessionFilter] = useState(false)
  const [otherSessionMessages, setOtherSessionMessages] = useState<
    Map<number, ChatMessageData[]>
  >(new Map())
  const [isLoadingSessions, setIsLoadingSessions] = useState(false)
  const inputRef = useRef<HTMLInputElement>(null)
  const filterRef = useRef<HTMLDivElement>(null)

  // Keep selectedSessionIds in sync with sessions list changes
  useEffect(() => {
    setSelectedSessionIds((prev) => {
      const validIds = new Set(sessions.map((s) => s.id))
      const next = new Set<number>()
      for (const id of prev) {
        if (validIds.has(id)) next.add(id)
      }
      // Include newly added sessions by default
      for (const s of sessions) {
        if (!next.has(s.id)) next.add(s.id)
      }
      if (next.size === 0) return new Set(sessions.map((s) => s.id))
      return next
    })
  }, [sessions])

  // Open on Ctrl/Cmd + K or Ctrl/Cmd + F
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && (e.key === 'k' || e.key === 'f')) {
        e.preventDefault()
        setIsOpen(true)
      } else if (e.key === 'Escape') {
        setIsOpen(false)
      }
    }
    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [])

  // Load messages from other sessions when the modal opens
  const loadOtherSessions = useCallback(async () => {
    const others = sessions.filter((s) => s.id !== currentSessionId)
    if (others.length === 0) return

    setIsLoadingSessions(true)
    try {
      const entries = await Promise.all(
        others.map(async (s) => {
          const msgs = await loadSessionMessagesFromStorage(s.id)
          return [s.id, msgs] as const
        }),
      )
      setOtherSessionMessages(new Map(entries))
    } finally {
      setIsLoadingSessions(false)
    }
  }, [sessions, currentSessionId])

  // Focus input when opened, reset state when closed
  const prevIsOpenRef = useRef(isOpen)
  useEffect(() => {
    if (isOpen && !prevIsOpenRef.current) {
      setTimeout(() => inputRef.current?.focus(), 10)
      loadOtherSessions()
    }
    if (!isOpen && prevIsOpenRef.current) {
      setQuery('') // eslint-disable-line react-hooks/set-state-in-effect -- reset on close
      setResults([])
      setShowSessionFilter(false)
    }
    prevIsOpenRef.current = isOpen
  }, [isOpen, loadOtherSessions])

  // Close filter dropdown on outside click
  useEffect(() => {
    if (!showSessionFilter) return
    const handleClick = (e: MouseEvent) => {
      if (filterRef.current && !filterRef.current.contains(e.target as Node)) {
        setShowSessionFilter(false)
      }
    }
    document.addEventListener('mousedown', handleClick)
    return () => document.removeEventListener('mousedown', handleClick)
  }, [showSessionFilter])

  // Session name lookup
  const sessionNameMap = useMemo(() => {
    const map = new Map<number, string>()
    for (const s of sessions) map.set(s.id, s.name)
    return map
  }, [sessions])

  // Combine messages from all selected sessions
  const allSearchableItems = useMemo(() => {
    const items: SearchResultItem[] = []

    if (selectedSessionIds.has(currentSessionId)) {
      const name =
        sessionNameMap.get(currentSessionId) || `Session ${currentSessionId}`
      for (const msg of messages) {
        items.push({
          message: msg,
          sessionId: currentSessionId,
          sessionName: name,
        })
      }
    }

    for (const [sid, msgs] of otherSessionMessages) {
      if (!selectedSessionIds.has(sid)) continue
      const name = sessionNameMap.get(sid) || `Session ${sid}`
      for (const msg of msgs) {
        items.push({ message: msg, sessionId: sid, sessionName: name })
      }
    }

    return items
  }, [
    messages,
    otherSessionMessages,
    selectedSessionIds,
    currentSessionId,
    sessionNameMap,
  ])

  // Per-session hit counts (computed against ALL sessions, not just selected)
  const hitCountBySession = useMemo(() => {
    const counts = new Map<number, number>()
    if (!query.trim()) return counts

    const words = query.toLowerCase().trim().split(/\s+/)
    const match = (msg: ChatMessageData) => {
      const content = msg.content.toLowerCase()
      return words.every((w) => content.includes(w))
    }

    counts.set(currentSessionId, messages.filter(match).length)
    for (const [sid, msgs] of otherSessionMessages) {
      counts.set(sid, msgs.filter(match).length)
    }
    return counts
  }, [query, messages, otherSessionMessages, currentSessionId])

  // Fuzzy search with debouncing
  useEffect(() => {
    if (!query.trim()) {
      if (results.length > 0) setResults([]) // eslint-disable-line react-hooks/set-state-in-effect -- clear when query empty
      return
    }

    const timer = setTimeout(() => {
      const words = query.toLowerCase().trim().split(/\s+/)
      const filtered = allSearchableItems
        .filter((item) => {
          const content = item.message.content.toLowerCase()
          return words.every((word) => content.includes(word))
        })
        .reverse() // Newest results first
        .slice(0, 50) // Limit results

      setResults(filtered)
      setSelectedIndex(0)
    }, 200)

    return () => clearTimeout(timer)
  }, [query, allSearchableItems]) // eslint-disable-line react-hooks/exhaustive-deps -- results.length guard prevents loop

  const handleSelect = useCallback(
    (item: SearchResultItem) => {
      if (item.sessionId === currentSessionId) {
        onSelectMessage(item.message.chatID, item.message.role)
      } else {
        onSwitchAndSelect?.(
          item.sessionId,
          item.message.chatID,
          item.message.role,
        )
      }
      setIsOpen(false)
      onClose?.()
    },
    [onSelectMessage, onSwitchAndSelect, onClose, currentSessionId],
  )

  const handleKeyDown = (e: React.KeyboardEvent) => {
    // Ignore keyboard events when composition is in progress (IME)
    if (e.nativeEvent.isComposing) return

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

  const toggleSession = (id: number) => {
    setSelectedSessionIds((prev) => {
      const next = new Set(prev)
      if (next.has(id)) {
        next.delete(id)
      } else {
        next.add(id)
      }
      return next
    })
  }

  const toggleAll = () => {
    const allIds = sessions.map((s) => s.id)
    const allSelected = allIds.every((id) => selectedSessionIds.has(id))
    if (allSelected) {
      setSelectedSessionIds(new Set([currentSessionId]))
    } else {
      setSelectedSessionIds(new Set(allIds))
    }
  }

  const allSelected = sessions.every((s) => selectedSessionIds.has(s.id))
  const filterLabel = allSelected
    ? 'All sessions'
    : `${selectedSessionIds.size} session${selectedSessionIds.size > 1 ? 's' : ''}`

  return (
    <>
      <TooltipWrapper content="Search messages (Ctrl+K / Ctrl+F)">
        <Button
          variant="ghost"
          size="sm"
          onClick={() => setIsOpen(true)}
          className="h-9 w-9 rounded-lg px-0"
          aria-label="Search messages"
        >
          <Search className="h-4 w-4" />
        </Button>
      </TooltipWrapper>

      {isOpen && createPortal(
        <div className="fixed inset-0 z-50 flex items-start justify-center bg-black/40 p-4 pt-[10dvh] backdrop-blur-[2px]">
          <div className="fixed inset-0" onClick={() => setIsOpen(false)} />
          <Card
            className="theme-surface relative z-10 w-full max-w-xl overflow-hidden rounded-xl border border-primary/25 shadow-2xl shadow-primary/10"
            style={{ borderTopColor: 'var(--primary)', borderTopWidth: '2px' }}
          >
            {/* Search input */}
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

            {/* Session filter (only when multiple sessions exist) */}
            {sessions.length > 1 && (
              <div
                className="relative flex items-center border-b px-3 py-1.5"
                ref={filterRef}
              >
                <Filter className="mr-2 h-3.5 w-3.5 shrink-0 opacity-40" />
                <button
                  onClick={() => setShowSessionFilter((prev) => !prev)}
                  className="flex items-center gap-1 rounded-md px-2 py-1 text-xs text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
                >
                  <span>{filterLabel}</span>
                  <ChevronDown
                    className={cn(
                      'h-3 w-3 transition-transform',
                      showSessionFilter && 'rotate-180',
                    )}
                  />
                </button>
                {isLoadingSessions && (
                  <div className="ml-2 h-3 w-3 animate-spin rounded-full border border-primary/20 border-t-primary" />
                )}

                {showSessionFilter && (
                  <div className="absolute left-0 top-full z-20 w-full rounded-b-lg border border-t-0 bg-popover p-1 shadow-lg">
                    <button
                      onClick={toggleAll}
                      className="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-xs transition-colors hover:bg-muted"
                    >
                      <div
                        className={cn(
                          'flex h-4 w-4 shrink-0 items-center justify-center rounded border',
                          allSelected
                            ? 'border-primary bg-primary text-primary-foreground'
                            : 'border-muted-foreground/30',
                        )}
                      >
                        {allSelected && <Check className="h-3 w-3" />}
                      </div>
                      <span className="font-medium">Select All</span>
                    </button>
                    <div className="my-1 border-t" />
                    <div className="max-h-40 overflow-y-auto">
                      {sessions.map((s) => {
                        const isChecked = selectedSessionIds.has(s.id)
                        const hits = hitCountBySession.get(s.id)
                        return (
                          <button
                            key={s.id}
                            onClick={() => toggleSession(s.id)}
                            className="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-xs transition-colors hover:bg-muted"
                          >
                            <div
                              className={cn(
                                'flex h-4 w-4 shrink-0 items-center justify-center rounded border',
                                isChecked
                                  ? 'border-primary bg-primary text-primary-foreground'
                                  : 'border-muted-foreground/30',
                              )}
                            >
                              {isChecked && <Check className="h-3 w-3" />}
                            </div>
                            <span className="min-w-0 flex-1 truncate text-left">
                              {s.name}
                              {s.id === currentSessionId && (
                                <span className="ml-1 text-[10px] opacity-50">
                                  (current)
                                </span>
                              )}
                            </span>
                            {hits !== undefined && (
                              <span
                                className={cn(
                                  'shrink-0 rounded-full px-1.5 py-0.5 text-[10px] font-medium tabular-nums',
                                  hits > 0
                                    ? 'bg-primary/15 text-primary'
                                    : 'bg-muted text-muted-foreground/60',
                                )}
                              >
                                {hits}
                              </span>
                            )}
                          </button>
                        )
                      })}
                    </div>
                  </div>
                )}
              </div>
            )}

            {/* Results */}
            <div className="max-h-[60dvh] overflow-y-auto p-2">
              {results.length > 0 ? (
                <div className="space-y-1">
                  {results.map((item, i) => (
                    <div
                      key={`${item.message.chatID}-${item.message.role}-${item.sessionId}`}
                      className={cn(
                        'flex cursor-pointer flex-col rounded-md px-3 py-2 transition-colors',
                        i === selectedIndex
                          ? 'bg-accent text-accent-foreground'
                          : 'hover:bg-muted/50',
                      )}
                      onClick={() => handleSelect(item)}
                    >
                      <div className="mb-1 flex items-center gap-2">
                        {sessions.length > 1 && (
                          <span className="max-w-[120px] shrink-0 truncate rounded-[4px] bg-muted px-1.5 py-0.5 text-[10px] font-medium text-muted-foreground">
                            {item.sessionName}
                          </span>
                        )}
                        <span
                          className={cn(
                            'shrink-0 rounded-[4px] px-1.5 py-0.5 text-[10px] font-bold uppercase tracking-wider',
                            item.message.role === 'user'
                              ? 'bg-primary/20 text-primary'
                              : 'bg-muted text-muted-foreground',
                          )}
                        >
                          {item.message.role}
                        </span>
                        {item.message.timestamp && (
                          <span className="text-[10px] opacity-50">
                            {new Date(item.message.timestamp).toLocaleString()}
                          </span>
                        )}
                      </div>
                      <div className="line-clamp-2 text-xs leading-relaxed opacity-90">
                        {item.message.content}
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
                  Start typing to search
                  {sessions.length > 1
                    ? allSelected
                      ? ' across all sessions'
                      : ` in ${selectedSessionIds.size} session${selectedSessionIds.size > 1 ? 's' : ''}`
                    : ''}
                  ...
                </div>
              )}
            </div>

            {/* Footer */}
            <div className="flex items-center justify-between border-t bg-muted/30 px-3 py-2 text-[10px] text-muted-foreground">
              <div className="flex items-center gap-3">
                <span className="flex items-center gap-1">
                  <kbd className="rounded border bg-background px-1">↑↓</kbd>{' '}
                  navigate
                </span>
                <span className="flex items-center gap-1">
                  <kbd className="rounded border bg-background px-1">Enter</kbd>{' '}
                  select
                </span>
              </div>
              <div className="flex items-center gap-1">
                <Command className="h-3 w-3" />
                <span>Total {allSearchableItems.length} messages</span>
              </div>
            </div>
          </Card>
        </div>,
        document.body,
      )}
    </>
  )
}
