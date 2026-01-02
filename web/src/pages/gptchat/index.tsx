/**
 * GPTChat page - main chat interface.
 */
import { ArrowDown, Paperclip, Settings, X } from 'lucide-react'
import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react'

import { ThemeToggle } from '@/components/theme-toggle'
import { Button } from '@/components/ui/button'
import { API_BASE } from '@/utils/api'
import { cn } from '@/utils/cn'
import { setPageFavicon, setPageTitle } from '@/utils/dom'
import { kvGet, kvSet, StorageKeys } from '@/utils/storage'
import {
  ChatInput,
  ChatMessage,
  ConfigSidebar,
  FloatingMessageHeader,
  ModelSelector,
  SelectionToolbar,
  SessionDock,
  TTSAudioPlayer,
} from './components'
import { useChat } from './hooks/use-chat'
import { useConfig } from './hooks/use-config'
import { useFloatingHeader } from './hooks/use-floating-header'
import { useTTS } from './hooks/use-tts'
import { ImageModelFluxDev, isImageModel } from './models'
import type {
  ChatAttachment,
  ChatMessageData,
  PromptShortcut,
  SessionConfig,
} from './types'
import { DefaultSessionConfig } from './types'
import { syncMCPServerTools } from './utils/mcp'

const MESSAGE_PAGE_SIZE = 40
type VersionSetting = { Key: string; Value: string }
type VersionResponse = { Settings?: VersionSetting[] }

/**
 * GPTChatPage provides a full-featured chat interface.
 */
export function GPTChatPage() {
  const {
    config,
    sessionId,
    sessions,
    isLoading: configLoading,
    updateConfig,
    createSession,
    deleteSession,
    switchSession,
    reorderSessions,
    renameSession,
    updateSessionVisibility,
    duplicateSession,
    purgeAllSessions,
    exportAllData,
    importAllData,
  } = useConfig()
  const {
    messages,
    isLoading: chatLoading,
    error,
    sendMessage,
    stopGeneration,
    clearMessages,
    deleteMessage,
    loadMessages,
    regenerateMessage,
    editAndRetry,
  } = useChat({ sessionId, config })

  // Update page title and favicon
  useEffect(() => {
    setPageTitle('Chat')
    setPageFavicon('/gptchat/favicon.ico')
  }, [])

  const [configOpen, setConfigOpen] = useState(false)
  const [promptShortcuts, setPromptShortcuts] = useState<PromptShortcut[]>([])
  const messagesEndRef = useRef<HTMLDivElement>(null)
  const messagesContainerRef = useRef<HTMLDivElement>(null)
  const [showScrollButton, setShowScrollButton] = useState(false)
  const [visibleCount, setVisibleCount] = useState(MESSAGE_PAGE_SIZE)
  const [upgradeInfo, setUpgradeInfo] = useState<{
    from: string
    to: string
  } | null>(null)
  const [prefillDraft, setPrefillDraft] = useState<
    | {
        id: string
        text: string
      }
    | undefined
  >(undefined)
  const [globalDraft, setGlobalDraft] = useState<string>('')

  // Load global draft on mount
  useEffect(() => {
    const loadDraft = async () => {
      const draft = await kvGet<unknown>(StorageKeys.SESSION_DRAFTS)
      if (draft) {
        if (typeof draft === 'string') {
          setGlobalDraft(draft)
        } else if (typeof draft === 'object' && draft !== null) {
          // Migrate from old Record<number, string> format
          const values = Object.values(draft as Record<string, unknown>)
          const firstVal = values.find((v) => typeof v === 'string')
          if (firstVal) {
            setGlobalDraft(firstVal as string)
          }
        }
      }
    }
    loadDraft()
  }, [])

  // Persist global draft when it changes (debounced)
  useEffect(() => {
    const timer = setTimeout(() => {
      kvSet(StorageKeys.SESSION_DRAFTS, globalDraft)
    }, 1000)
    return () => clearTimeout(timer)
  }, [globalDraft])

  const [editingMessage, setEditingMessage] = useState<{
    chatId: string
    content: string
    attachments?: ChatAttachment[]
  } | null>(null)
  const [selectedMessageIndex, setSelectedMessageIndex] = useState<number>(-1)
  const [selectionData, setSelectionData] = useState<{
    text: string
    position: { top: number; left: number }
  } | null>(null)

  const {
    requestTTS,
    stopTTS,
    audioUrl: ttsAudioUrl,
  } = useTTS({
    apiToken: config.api_token || '',
  })

  const chatModel = config.selected_chat_model || config.selected_model
  const drawModel = config.selected_draw_model || ImageModelFluxDev
  const isDrawActive = isImageModel(config.selected_model)
  const activeModelName = isDrawActive ? drawModel : chatModel
  const messagePlaceholder = config.api_token
    ? activeModelName || 'Message...'
    : 'Enter your API key in Settings to start chatting'

  // Load messages when session changes
  useEffect(() => {
    if (!configLoading) {
      loadMessages()
    }
  }, [sessionId, loadMessages, configLoading])

  // Load shortcuts on mount or when config finishes loading
  useEffect(() => {
    if (!configLoading) {
      loadPromptShortcuts()
    }
  }, [configLoading])

  // Global selection listener
  useEffect(() => {
    const handleGlobalMouseUp = (e: MouseEvent) => {
      const { clientX, clientY } = e
      // Small delay to allow selection to be finalized
      setTimeout(() => {
        const selection = window.getSelection()
        if (selection && selection.toString().trim().length > 0) {
          const range = selection.getRangeAt(0)
          const rect = range.getBoundingClientRect()

          // Check if selection is within the messages container
          const container = messagesContainerRef.current
          if (container && container.contains(selection.anchorNode)) {
            setSelectionData({
              text: selection.toString(),
              position: {
                top: clientY || rect.top,
                left: clientX || rect.left + rect.width / 2,
              },
            })
          }
        } else {
          setSelectionData(null)
        }
      }, 10)
    }

    document.addEventListener('mouseup', handleGlobalMouseUp)
    return () => document.removeEventListener('mouseup', handleGlobalMouseUp)
  }, [])

  useEffect(() => {
    let cancelled = false

    const checkUpgrade = async () => {
      try {
        const resp = await fetch(`${API_BASE}/version`, { cache: 'no-cache' })
        if (!resp.ok) return
        const data = (await resp.json()) as VersionResponse
        const serverVer = data.Settings?.find(
          (item) => item.Key === 'vcs.time',
        )?.Value
        if (!serverVer) return
        const localVer = await kvGet<string>(StorageKeys.VERSION_DATE)
        if (cancelled) return
        await kvSet(StorageKeys.VERSION_DATE, serverVer)
        if (localVer && localVer !== serverVer) {
          setUpgradeInfo({ from: localVer, to: serverVer })
        }
      } catch (err) {
        console.warn('Failed to check version:', err)
      }
    }

    checkUpgrade()
    return () => {
      cancelled = true
    }
  }, [])

  const isNearBottom = useCallback(() => {
    const { scrollTop, scrollHeight, clientHeight } = document.documentElement
    return scrollHeight - scrollTop - clientHeight < 120
  }, [])

  // Track whether we should keep auto-following the bottom during streaming
  const autoScrollRef = useRef(true)
  const suppressAutoScrollOnceRef = useRef(false)

  const scrollToBottom = useCallback(
    (options?: { force?: boolean }) => {
      if (!options?.force && !isNearBottom()) {
        return
      }
      messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' })
    },
    [isNearBottom],
  )

  const scrollToTop = useCallback(() => {
    window.scrollTo({ top: 0, behavior: 'smooth' })
  }, [])

  // Auto-sync MCP tools in background
  useEffect(() => {
    if (configLoading || !config.mcp_servers) return

    const syncMetrics = async () => {
      let hasUpdates = false
      const updatedServers = [...(config.mcp_servers || [])]

      for (let i = 0; i < updatedServers.length; i++) {
        const srv = updatedServers[i]
        // If enabled and no tools, try to sync
        if (srv.enabled && (!srv.tools || srv.tools.length === 0)) {
          try {
            // We clone logic from McpServerManager somewhat,
            // but we want to do it silently in background.
            const { updatedServer } = await syncMCPServerTools(srv)
            updatedServers[i] = updatedServer
            hasUpdates = true
            console.log(`[MCP] Auto-synced tools for ${srv.name}`)
          } catch (e) {
            console.warn(`[MCP] Failed to auto-sync ${srv.name}:`, e)
            // Should we disable it to avoid retry loops? Maybe not.
          }
        }
      }

      if (hasUpdates) {
        updateConfig({ mcp_servers: updatedServers })
      }
    }

    // Debounce/Check only once per mount/session?
    // We can use a simple check: if we blindly run this, and it updates config,
    // it triggers effect again. config.mcp_servers changes.
    // But if (srv.tools.length === 0) checks prevents loop if sync succeeds.
    // If sync fails, it might loop.
    // We should probably safeguard this.
    // Let's rely on "if tools.length === 0".
    // If sync fails, it remains 0. It will retry. That's bad.
    // We should maybe mark it as "attempted"?
    // Or just run it once on mount/config load?
    // Let's run it once whenever `configLoading` flips to false.
    syncMetrics()

    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [configLoading]) // Only run when loading finishes

  // Auto-scroll only when auto-follow is enabled (e.g., new send) or near bottom
  useEffect(() => {
    if (suppressAutoScrollOnceRef.current) {
      suppressAutoScrollOnceRef.current = false
      return
    }
    if (autoScrollRef.current || isNearBottom()) {
      scrollToBottom({ force: true })
    }
  }, [messages, scrollToBottom, isNearBottom])

  useEffect(() => {
    setVisibleCount((prev) => {
      if (messages.length === 0) {
        return MESSAGE_PAGE_SIZE
      }

      const desired = Math.min(MESSAGE_PAGE_SIZE, messages.length)

      if (prev < desired) {
        return desired
      }

      if (prev > messages.length) {
        return messages.length
      }

      return prev
    })
  }, [messages.length])

  // Reset selection when messages change (e.g. new message sent) or session changes
  useEffect(() => {
    setSelectedMessageIndex(-1)
  }, [messages.length, sessionId])

  // Track scroll position for scroll-to-bottom button (using window scroll)
  useEffect(() => {
    const handleScroll = () => {
      const near = isNearBottom()
      setShowScrollButton(!near)
      // Disable auto-follow as soon as user scrolls away
      if (!near) {
        autoScrollRef.current = false
      } else {
        autoScrollRef.current = true
      }
    }

    window.addEventListener('scroll', handleScroll, { passive: true })
    return () => window.removeEventListener('scroll', handleScroll)
  }, [isNearBottom])

  const handleLoadOlder = useCallback(() => {
    const prevScrollHeight = document.documentElement.scrollHeight
    const prevScrollTop = window.scrollY

    setVisibleCount((prev) =>
      Math.min(prev + MESSAGE_PAGE_SIZE, messages.length),
    )

    // Keep the viewport anchored after older messages are prepended.
    requestAnimationFrame(() => {
      const nextScrollHeight = document.documentElement.scrollHeight
      const delta = nextScrollHeight - prevScrollHeight
      window.scrollTo({ top: prevScrollTop + Math.max(delta, 0) })
    })
  }, [messages.length])

  const userMessageByChatId = useMemo(() => {
    const map = new Map<string, ChatMessageData>()
    messages.forEach((msg) => {
      if (msg.role === 'user') {
        map.set(msg.chatID, msg)
      }
    })
    return map
  }, [messages])

  const displayedMessages = useMemo(() => {
    if (messages.length <= visibleCount) {
      return messages
    }
    return messages.slice(-visibleCount)
  }, [messages, visibleCount])

  // Track which message's header should appear in the floating header
  const floatingHeaderState = useFloatingHeader({
    messages: displayedMessages,
    containerRef: messagesContainerRef,
    topOffset: 48, // Height of the fixed header (top-12 = 48px)
  })

  /**
   * findFirstVisibleMessageIndex finds the index of the first message
   * that is currently visible in the viewport.
   *
   * @returns The index of the first visible message, or -1 if none found.
   */
  const findFirstVisibleMessageIndex = useCallback((): number => {
    if (displayedMessages.length === 0) return -1

    // Use the viewport bounds since this page uses window scroll
    const viewportTop = 0
    const viewportBottom = window.innerHeight

    for (let i = 0; i < displayedMessages.length; i++) {
      const msg = displayedMessages[i]
      const el = document.getElementById(
        `chat-message-${msg.chatID}-${msg.role}`,
      )
      if (el) {
        const rect = el.getBoundingClientRect()
        // Check if element is at least partially visible in viewport
        if (rect.bottom > viewportTop && rect.top < viewportBottom) {
          return i
        }
      }
    }
    return 0 // Default to first message if none found
  }, [displayedMessages])

  /**
   * handleMessageSelect toggles selection for a message at the given index.
   * If the message is already selected, it deselects it.
   *
   * @param index - The index of the message to toggle selection for.
   */
  const handleMessageSelect = useCallback((index: number) => {
    setSelectedMessageIndex((prev) => (prev === index ? -1 : index))
  }, [])

  // Keyboard shortcuts for message navigation
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      const isInput =
        e.target instanceof HTMLInputElement ||
        e.target instanceof HTMLTextAreaElement

      if (e.key === 'ArrowUp') {
        // If in input, only navigate if cursor is at the top or Alt is pressed
        if (isInput && !e.altKey) {
          if (e.target instanceof HTMLTextAreaElement) {
            if (e.target.selectionStart !== 0) return
          } else {
            return
          }
        }

        e.preventDefault()
        setSelectedMessageIndex((prev) => {
          if (prev === -1) {
            // Start from first visible message
            const visibleIdx = findFirstVisibleMessageIndex()
            return visibleIdx >= 0 ? visibleIdx : displayedMessages.length - 1
          }
          return Math.max(0, prev - 1)
        })
      } else if (e.key === 'ArrowDown') {
        // If in input, only navigate if cursor is at the bottom or Alt is pressed
        if (isInput && !e.altKey) {
          if (e.target instanceof HTMLTextAreaElement) {
            if (e.target.selectionStart !== e.target.value.length) return
          } else {
            return
          }
        }

        e.preventDefault()
        setSelectedMessageIndex((prev) => {
          if (prev === -1) {
            // Start from first visible message
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
  }, [displayedMessages, selectedMessageIndex, findFirstVisibleMessageIndex])

  // Scroll selected message into view
  useEffect(() => {
    if (
      selectedMessageIndex >= 0 &&
      selectedMessageIndex < displayedMessages.length
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

  const lastMessage = messages[messages.length - 1]
  const currentDraftMessage = globalDraft

  const loadPromptShortcuts = async () => {
    let shortcuts = await kvGet<PromptShortcut[]>(StorageKeys.PROMPT_SHORTCUTS)

    // If no shortcuts found (or empty array), use defaults
    if (!shortcuts || shortcuts.length === 0) {
      const { DefaultPrompts } = await import('./data/prompts')
      shortcuts = DefaultPrompts
      // Optionally save defaults to storage so user can edit them
      // await kvSet(StorageKeys.PROMPT_SHORTCUTS, DefaultPrompts)
      // Decision: Don't save defaults immediately to keep storage clean?
      // Actually, legacy behavior likely just read them.
      // Let's just set them in state.
    }

    setPromptShortcuts(shortcuts)
  }

  const handleSavePrompt = useCallback(
    async (name: string, prompt: string) => {
      const newShortcut: PromptShortcut = { name, prompt }
      // Check if already exists, if so update it, else append
      const index = promptShortcuts.findIndex((s) => s.name === name)
      let updated: PromptShortcut[]
      if (index >= 0) {
        updated = [...promptShortcuts]
        updated[index] = newShortcut
      } else {
        updated = [...promptShortcuts, newShortcut]
      }
      setPromptShortcuts(updated)
      await kvSet(StorageKeys.PROMPT_SHORTCUTS, updated)
    },
    [promptShortcuts],
  )

  const handleEditPrompt = useCallback(
    async (oldName: string, newName: string, newPrompt: string) => {
      const updated = promptShortcuts.map((s) =>
        s.name === oldName ? { name: newName, prompt: newPrompt } : s,
      )
      setPromptShortcuts(updated)
      await kvSet(StorageKeys.PROMPT_SHORTCUTS, updated)
    },
    [promptShortcuts],
  )

  const handleDeletePrompt = useCallback(
    async (name: string) => {
      const updated = promptShortcuts.filter((s) => s.name !== name)
      setPromptShortcuts(updated)
      await kvSet(StorageKeys.PROMPT_SHORTCUTS, updated)
    },
    [promptShortcuts],
  )

  const handleConfigChange = useCallback(
    (updates: Partial<SessionConfig>) => {
      updateConfig(updates)
    },
    [updateConfig],
  )

  const handleChatSwitchChange = useCallback(
    (updates: Partial<SessionConfig['chat_switch']>) => {
      updateConfig({
        chat_switch: {
          ...config.chat_switch,
          ...updates,
        },
      })
    },
    [config.chat_switch, updateConfig],
  )

  const handleChatModelChange = useCallback(
    (model: string) => {
      handleConfigChange({
        selected_model: model,
        selected_chat_model: model,
      })
    },
    [handleConfigChange],
  )

  const handleDrawModelChange = useCallback(
    (model: string) => {
      handleConfigChange({
        selected_model: model,
        selected_draw_model: model,
      })
    },
    [handleConfigChange],
  )

  const handleReset = useCallback(async () => {
    await updateConfig(DefaultSessionConfig)
  }, [updateConfig])

  const handleSend = useCallback(
    async (content: string, files?: File[]) => {
      autoScrollRef.current = true
      await sendMessage(content, files)
      requestAnimationFrame(() => scrollToBottom({ force: true }))
    },
    [scrollToBottom, sendMessage],
  )

  const handleRegenerate = useCallback(
    async (chatId: string) => {
      // Do not auto-scroll on regenerate; keep viewport stable.
      autoScrollRef.current = false
      suppressAutoScrollOnceRef.current = true
      await regenerateMessage(chatId)
    },
    [regenerateMessage],
  )

  const handleEditResend = useCallback(
    (payload: {
      chatId: string
      content: string
      attachments?: ChatAttachment[]
    }) => {
      autoScrollRef.current = false
      suppressAutoScrollOnceRef.current = true
      setEditingMessage({
        chatId: payload.chatId,
        content: payload.content,
        attachments: payload.attachments,
      })
    },
    [],
  )

  const handleConfirmEdit = useCallback(
    async (newContent: string, attachments?: ChatAttachment[]) => {
      if (!editingMessage) return
      setEditingMessage(null)
      autoScrollRef.current = false
      suppressAutoScrollOnceRef.current = true
      await editAndRetry(editingMessage.chatId, newContent, attachments)
    },
    [editAndRetry, editingMessage],
  )

  const handleClearChats = useCallback(async () => {
    await clearMessages()
  }, [clearMessages])

  const handlePurgeAllSessions = useCallback(async () => {
    await purgeAllSessions()
    await clearMessages()
  }, [purgeAllSessions, clearMessages])

  const handleImportData = useCallback(
    async (data: unknown) => {
      // Accept any shape but ensure we pass an object map to storage importer
      await importAllData((data as Record<string, unknown>) || {})
    },
    [importAllData],
  )

  const handleDraftChange = useCallback((value: string) => {
    setGlobalDraft(value)
  }, [])

  const handleQuote = useCallback((text: string) => {
    setPrefillDraft({ id: Date.now().toString(), text })
    setSelectionData(null)
  }, [])

  const handleSelectionCopy = useCallback(async () => {
    if (selectionData) {
      try {
        await navigator.clipboard.writeText(selectionData.text)
      } catch (err) {
        console.error('Failed to copy selection:', err)
      }
    }
  }, [selectionData])

  const handleSelectionTTS = useCallback(() => {
    if (selectionData) {
      requestTTS(selectionData.text)
    }
  }, [selectionData, requestTTS])

  return (
    <div className="theme-bg min-h-dvh w-full max-w-full overflow-x-hidden">
      {/* Session Dock (Fixed Left Sidebar) */}
      <aside className="theme-surface theme-border fixed left-0 top-0 z-40 flex h-dvh w-10 shrink-0 flex-col border-r">
        {/* Dock header area */}
        <div className="flex h-12 shrink-0 items-center justify-center border-b border-border">
          <span className="text-base">💬</span>
        </div>
        {/* Session buttons */}
        <SessionDock
          sessions={sessions}
          activeSessionId={sessionId}
          onSwitchSession={switchSession}
          onCreateSession={() => createSession()}
          onClearChats={handleClearChats}
        />
      </aside>

      {/* Main Content Area - offset by sidebar width */}
      <div className="ml-10 flex min-h-dvh min-w-0 flex-1 flex-col">
        {/* Header - fixed at top */}
        <header
          className="theme-surface theme-border fixed left-10 right-0 top-0 z-30 flex h-12 shrink-0 items-center justify-between border-b px-1 sm:px-2"
          onClick={(e) => {
            if (e.target !== e.currentTarget) {
              return
            }
            scrollToTop()
          }}
        >
          <div className="flex items-center gap-2">
            <div
              className="flex items-center gap-0.5"
              title={
                activeModelName
                  ? `Active model: ${activeModelName}`
                  : 'Select a model'
              }
            >
              <ModelSelector
                label="Chat"
                categories={[
                  'OpenAI',
                  'Anthropic',
                  'Google',
                  'Deepseek',
                  'Others',
                ]}
                selectedModel={chatModel}
                active={!isDrawActive}
                onModelChange={handleChatModelChange}
                className="shrink-0 min-w-[70px] rounded-md"
                compact
                tone="ghost"
              />
              <ModelSelector
                label="Draw"
                categories={['Image']}
                selectedModel={drawModel}
                active={isDrawActive}
                onModelChange={handleDrawModelChange}
                className="shrink-0 min-w-[70px] rounded-md"
                compact
                tone="ghost"
              />
            </div>
          </div>

          <div className="ml-auto flex items-center gap-2 sm:gap-3">
            <div className="hidden sm:block">
              <ThemeToggle />
            </div>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => setConfigOpen(true)}
              className="h-9 w-9 rounded-md px-0"
              title="Settings"
            >
              <Settings className="h-4 w-4" />
            </Button>
          </div>
        </header>

        {/* Floating message header - appears when a message's inline header scrolls out of view */}
        <FloatingMessageHeader
          message={floatingHeaderState.message}
          visible={floatingHeaderState.visible}
          onDelete={deleteMessage}
          onRegenerate={handleRegenerate}
          onEditResend={handleEditResend}
          pairedUserMessage={
            floatingHeaderState.message
              ? userMessageByChatId.get(floatingHeaderState.message.chatID)
              : undefined
          }
          apiToken={config.api_token}
          isStreaming={
            chatLoading &&
            floatingHeaderState.message?.role === 'assistant' &&
            lastMessage &&
            floatingHeaderState.message?.chatID === lastMessage.chatID
          }
        />

        {/* Scrollable chat area - uses window scroll with padding for fixed header/footer */}
        <main className="relative flex-1 pt-12 pb-28">
          {/* Loading overlay for session switching */}
          {configLoading && (
            <div className="fixed inset-0 z-10 flex items-center justify-center bg-background/20 backdrop-blur-[1px]">
              <div className="h-6 w-6 animate-spin rounded-full border-2 border-primary/20 border-t-primary" />
            </div>
          )}

          {/* Error display */}
          {error && (
            <div className="mx-4 mt-2 shrink-0 rounded-md bg-destructive/10 p-3 text-sm text-destructive">
              {error}
            </div>
          )}

          <div
            ref={messagesContainerRef}
            className="min-h-0 overflow-x-hidden px-1 pt-1 sm:px-2 sm:pt-1.5 md:px-4"
          >
            {messages.length === 0 ? (
              <div className="flex min-h-[calc(100dvh-10rem)] flex-col items-center justify-center text-center">
                <div className="mb-4 text-4xl">💬</div>
                <h2 className="text-lg font-medium">Start a conversation</h2>
                <p className="mt-1 max-w-sm text-sm text-muted-foreground">
                  Type a message below to begin chatting with the AI. You can
                  change the model and settings using the button above.
                </p>
              </div>
            ) : (
              <div className="space-y-2.5 pb-2 sm:space-y-4 sm:pb-3">
                {messages.length > displayedMessages.length && (
                  <div className="flex justify-center">
                    <Button variant="ghost" size="sm" onClick={handleLoadOlder}>
                      Load older messages
                    </Button>
                  </div>
                )}
                {displayedMessages.map((msg, idx) => (
                  <ChatMessage
                    key={`${msg.chatID}-${msg.role}`}
                    message={msg}
                    onDelete={deleteMessage}
                    onRegenerate={handleRegenerate}
                    onEditResend={handleEditResend}
                    pairedUserMessage={userMessageByChatId.get(msg.chatID)}
                    isSelected={idx === selectedMessageIndex}
                    onSelect={handleMessageSelect}
                    messageIndex={idx}
                    apiToken={config.api_token}
                    isStreaming={
                      chatLoading &&
                      msg.role === 'assistant' &&
                      lastMessage &&
                      msg.chatID === lastMessage.chatID &&
                      msg.role === lastMessage.role
                    }
                  />
                ))}
                <div ref={messagesEndRef} />
              </div>
            )}
          </div>

          {/* Scroll to bottom button */}
          <button
            onClick={() => scrollToBottom({ force: true })}
            className={cn(
              'fixed bottom-36 right-2 z-40 flex h-9 w-9 items-center justify-center rounded-md bg-muted text-muted-foreground shadow-lg ring-1 ring-border backdrop-blur transition-all hover:bg-muted/80',
              showScrollButton
                ? 'translate-y-0 opacity-100'
                : 'translate-y-0 opacity-50',
            )}
            aria-label="Scroll to bottom"
          >
            <ArrowDown className="h-4 w-4" />
          </button>
        </main>

        {/* Input (fixed to bottom of viewport) */}
        <footer className="theme-surface theme-border fixed bottom-0 left-10 right-0 z-30 border-t p-0">
          <ChatInput
            onSend={handleSend}
            onStop={stopGeneration}
            isLoading={chatLoading}
            disabled={!config.api_token}
            config={config}
            sessionId={sessionId}
            isSidebarOpen={configOpen}
            onConfigChange={handleChatSwitchChange}
            prefillDraft={prefillDraft}
            onPrefillUsed={() => setPrefillDraft(undefined)}
            draftMessage={currentDraftMessage}
            onDraftChange={handleDraftChange}
            placeholder={messagePlaceholder}
          />
        </footer>
      </div>

      {/* Config Sidebar */}
      <ConfigSidebar
        isOpen={configOpen}
        onClose={() => setConfigOpen(false)}
        config={config}
        onConfigChange={handleConfigChange}
        onClearChats={handleClearChats}
        onReset={handleReset}
        promptShortcuts={promptShortcuts}
        onSavePrompt={handleSavePrompt}
        onEditPrompt={handleEditPrompt}
        onDeletePrompt={handleDeletePrompt}
        onExportData={exportAllData}
        onImportData={handleImportData}
        sessions={sessions}
        activeSessionId={sessionId}
        onCreateSession={createSession}
        onDeleteSession={deleteSession}
        onSwitchSession={switchSession}
        onRenameSession={renameSession}
        onUpdateSessionVisibility={updateSessionVisibility}
        onDuplicateSession={duplicateSession}
        onReorderSessions={reorderSessions}
        onPurgeAllSessions={handlePurgeAllSessions}
      />

      {upgradeInfo && (
        <div className="fixed bottom-4 right-4 z-50 max-w-sm rounded-lg border theme-border theme-elevated p-4 shadow-lg">
          <p className="text-sm font-medium">New version available</p>
          <p className="theme-text-muted text-xs">
            {upgradeInfo.from} → {upgradeInfo.to}
          </p>
          <div className="mt-3 flex gap-2">
            <Button size="sm" onClick={() => window.location.reload()}>
              Reload now
            </Button>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => setUpgradeInfo(null)}
            >
              Later
            </Button>
          </div>
        </div>
      )}

      {/* Edit Message Modal */}
      {editingMessage && (
        <EditMessageModal
          content={editingMessage.content}
          attachments={editingMessage.attachments}
          onClose={() => setEditingMessage(null)}
          onConfirm={handleConfirmEdit}
        />
      )}

      {/* Selection Toolbar */}
      {selectionData && (
        <SelectionToolbar
          text={selectionData.text}
          position={selectionData.position}
          onCopy={handleSelectionCopy}
          onTTS={handleSelectionTTS}
          onQuote={handleQuote}
          onClose={() => setSelectionData(null)}
        />
      )}

      {/* Selection TTS Player */}
      {ttsAudioUrl && (
        <div className="fixed bottom-24 left-1/2 z-50 -translate-x-1/2">
          <TTSAudioPlayer audioUrl={ttsAudioUrl} onClose={stopTTS} />
        </div>
      )}
    </div>
  )
}

interface EditMessageModalProps {
  content: string
  attachments?: ChatAttachment[]
  onClose: () => void
  onConfirm: (newContent: string, attachments?: ChatAttachment[]) => void
}

function EditMessageModal({
  content,
  attachments,
  onClose,
  onConfirm,
}: EditMessageModalProps) {
  const [editedContent, setEditedContent] = useState(content)
  const [editedAttachments, setEditedAttachments] = useState(attachments || [])
  const textareaRef = useRef<HTMLTextAreaElement>(null)

  useEffect(() => {
    textareaRef.current?.focus()
    textareaRef.current?.select()
  }, [])

  const handleSubmit = useCallback(() => {
    const trimmed = String(editedContent || '').trim()
    if (trimmed) {
      onConfirm(trimmed, editedAttachments)
    }
  }, [editedContent, editedAttachments, onConfirm])

  const handleRemoveAttachment = useCallback((index: number) => {
    setEditedAttachments((prev) => prev.filter((_, i) => i !== index))
  }, [])

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
      if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
        e.preventDefault()
        handleSubmit()
      } else if (e.key === 'Escape') {
        e.preventDefault()
        onClose()
      }
    },
    [handleSubmit, onClose],
  )

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-background/80 backdrop-blur-sm"
      onClick={onClose}
    >
      <div
        className="mx-4 w-full max-w-2xl rounded-lg border theme-border theme-elevated p-6 shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        <h3 className="mb-4 text-lg font-semibold">Edit Message</h3>

        {editedAttachments.length > 0 && (
          <div className="mb-4 flex flex-wrap gap-2">
            {editedAttachments.map((att, i) => (
              <div
                key={i}
                className="relative group flex items-center gap-2 rounded-md border border-border bg-muted px-2 py-1 text-xs shadow-sm"
              >
                {att.type === 'image' && att.contentB64 ? (
                  <div className="h-8 w-8 shrink-0 overflow-hidden rounded border border-border bg-background">
                    <img
                      src={att.contentB64}
                      alt={att.filename}
                      className="h-full w-full object-cover"
                    />
                  </div>
                ) : (
                  <Paperclip className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                )}
                <span className="max-w-[120px] truncate font-medium">
                  {att.filename}
                </span>
                <button
                  onClick={() => handleRemoveAttachment(i)}
                  className="ml-1 rounded-full p-0.5 text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
                  title="Remove attachment"
                >
                  <X className="h-3 w-3" />
                </button>
              </div>
            ))}
          </div>
        )}

        <textarea
          ref={textareaRef}
          value={editedContent}
          onChange={(e) => setEditedContent(e.target.value)}
          onKeyDown={handleKeyDown}
          className="theme-input theme-focus-ring w-full rounded border p-3 font-mono text-sm focus:outline-none focus:ring-2"
          rows={10}
        />
        <div className="mt-4 flex justify-end gap-2">
          <Button variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button
            onClick={handleSubmit}
            disabled={!String(editedContent || '').trim()}
          >
            Retry with Edited Message
          </Button>
        </div>
        <p className="mt-2 text-xs theme-text-muted">
          Ctrl+Enter to submit • Esc to cancel
        </p>
      </div>
    </div>
  )
}
