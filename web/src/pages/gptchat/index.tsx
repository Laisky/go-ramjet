/**
 * GPTChat page - main chat interface.
 */
import { ArrowDown, Settings } from 'lucide-react'
import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react'

import { ThemeToggle } from '@/components/theme-toggle'
import { Button } from '@/components/ui/button'
import { cn } from '@/utils/cn'
import { kvGet, kvSet, StorageKeys } from '@/utils/storage'
import {
  ChatInput,
  ChatMessage,
  ConfigSidebar,
  ModelSelector,
  SessionDock,
} from './components'
import { useChat } from './hooks/use-chat'
import { useConfig } from './hooks/use-config'
import { ImageModelFluxDev, isImageModel } from './models'
import type { ChatMessageData, PromptShortcut, SessionConfig } from './types'
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
    renameSession,
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
  const [sessionDrafts, setSessionDrafts] = useState<Record<number, string>>({})
  const [editingMessage, setEditingMessage] = useState<{
    chatId: string
    content: string
  } | null>(null)

  const chatModel = config.selected_chat_model || config.selected_model
  const drawModel = config.selected_draw_model || ImageModelFluxDev
  const isDrawActive = isImageModel(config.selected_model)
  const activeModelName = isDrawActive ? drawModel : chatModel
  const messagePlaceholder = config.api_token
    ? activeModelName || 'Message...'
    : 'Enter your API key in Settings to start chatting'

  // Load messages and shortcuts on mount
  useEffect(() => {
    if (!configLoading) {
      loadMessages()
      loadPromptShortcuts()
    }
  }, [configLoading, loadMessages])

  useEffect(() => {
    let cancelled = false

    const checkUpgrade = async () => {
      try {
        const resp = await fetch('/gptchat/version', { cache: 'no-cache' })
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

  const isNearBottom = useCallback((container?: HTMLElement | null) => {
    const el = container ?? messagesContainerRef.current
    if (!el) return true
    const { scrollTop, scrollHeight, clientHeight } = el
    return scrollHeight - scrollTop - clientHeight < 120
  }, [])

  // Track whether we should keep auto-following the bottom during streaming
  const autoScrollRef = useRef(true)
  const suppressAutoScrollOnceRef = useRef(false)

  const scrollToBottom = useCallback(
    (options?: { force?: boolean }) => {
      const container = messagesContainerRef.current
      if (!options?.force && !isNearBottom(container)) {
        return
      }
      messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' })
    },
    [isNearBottom],
  )

  const scrollToTop = useCallback(() => {
    const container = messagesContainerRef.current
    if (container) {
      container.scrollTo({ top: 0, behavior: 'smooth' })
    }
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

  // Track scroll position for scroll-to-bottom button
  useEffect(() => {
    const container = messagesContainerRef.current
    if (!container) return

    const handleScroll = () => {
      const near = isNearBottom(container)
      setShowScrollButton(!near)
      // Disable auto-follow as soon as user scrolls away
      if (!near) {
        autoScrollRef.current = false
      } else {
        autoScrollRef.current = true
      }
    }

    container.addEventListener('scroll', handleScroll)
    return () => container.removeEventListener('scroll', handleScroll)
  }, [])

  const handleLoadOlder = useCallback(() => {
    const container = messagesContainerRef.current
    const prevScrollHeight = container?.scrollHeight ?? 0
    const prevScrollTop = container?.scrollTop ?? 0

    setVisibleCount((prev) =>
      Math.min(prev + MESSAGE_PAGE_SIZE, messages.length),
    )

    // Keep the viewport anchored after older messages are prepended.
    requestAnimationFrame(() => {
      const nextContainer = messagesContainerRef.current
      if (!nextContainer) return
      const nextScrollHeight = nextContainer.scrollHeight
      const delta = nextScrollHeight - prevScrollHeight
      nextContainer.scrollTop = prevScrollTop + Math.max(delta, 0)
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

  const lastMessage = messages[messages.length - 1]
  const currentDraftMessage = sessionDrafts[sessionId] ?? ''

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
      const updated = [...promptShortcuts, newShortcut]
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
    (payload: { chatId: string; content: string }) => {
      autoScrollRef.current = false
      suppressAutoScrollOnceRef.current = true
      setEditingMessage({
        chatId: payload.chatId,
        content: payload.content,
      })
    },
    [],
  )

  const handleConfirmEdit = useCallback(
    async (newContent: string) => {
      if (!editingMessage) return
      setEditingMessage(null)
      autoScrollRef.current = false
      suppressAutoScrollOnceRef.current = true
      await editAndRetry(editingMessage.chatId, newContent)
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

  const handleDraftChange = useCallback(
    (value: string) => {
      setSessionDrafts((prev) => {
        const existing = prev[sessionId] ?? ''
        if (existing === value) {
          return prev
        }
        if (!value) {
          if (!(sessionId in prev)) {
            return prev
          }
          const updated = { ...prev }
          delete updated[sessionId]
          return updated
        }
        return { ...prev, [sessionId]: value }
      })
    },
    [sessionId],
  )

  if (configLoading) {
    return (
      <div className="flex h-screen items-center justify-center">
        <div className="text-center">
          <div className="mb-2 h-8 w-8 animate-spin rounded-full border-2 border-black/20 border-t-black dark:border-white/20 dark:border-t-white" />
          <p className="text-sm text-black/50 dark:text-white/50">Loading...</p>
        </div>
      </div>
    )
  }

  return (
    <div className="theme-bg flex h-screen w-full overflow-hidden">
      {/* Session Dock (Integrated Left Sidebar) */}
      <aside className="theme-surface theme-border flex h-full w-10 shrink-0 flex-col border-r">
        {/* Dock header area */}
        <div className="flex h-12 shrink-0 items-center justify-center border-b border-slate-200 dark:border-slate-700">
          <span className="text-base">💬</span>
        </div>
        {/* Session buttons */}
        <SessionDock
          sessions={sessions}
          activeSessionId={sessionId}
          onSwitchSession={switchSession}
          onCreateSession={() => createSession()}
          onDeleteSession={deleteSession}
        />
      </aside>

      {/* Main Content Area */}
      <div className="flex flex-1 flex-col overflow-hidden">
        {/* Header */}
        <header
          className="theme-surface theme-border flex h-12 shrink-0 items-center justify-between border-b px-1 sm:px-2"
          onClick={(e) => {
            const target = e.target as HTMLElement
            if (
              target.closest(
                'button, [role="button"], input, select, textarea, a',
              )
            ) {
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

        {/* Scrollable chat area */}
        <main className="relative flex-1 overflow-hidden">
        {/* Error display */}
        {error && (
          <div className="mx-4 mt-2 rounded-md bg-red-100 p-3 text-sm text-red-700 dark:bg-red-900/20 dark:text-red-400">
            {error}
          </div>
        )}

          <div
            ref={messagesContainerRef}
            className="h-full overflow-y-auto overflow-x-hidden px-1 pb-[80px] pt-1 sm:px-2 sm:pb-[90px] sm:pt-1.5 md:px-4"
          >
          {messages.length === 0 ? (
            <div className="flex h-full flex-col items-center justify-center text-center">
              <div className="mb-4 text-4xl">💬</div>
              <h2 className="text-lg font-medium">Start a conversation</h2>
              <p className="mt-1 max-w-sm text-sm text-black/50 dark:text-white/50">
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
              {displayedMessages.map((msg) => (
                <ChatMessage
                  key={`${msg.chatID}-${msg.role}`}
                  message={msg}
                  onDelete={deleteMessage}
                  onRegenerate={handleRegenerate}
                  onEditResend={handleEditResend}
                  pairedUserMessage={userMessageByChatId.get(msg.chatID)}
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
              'absolute bottom-8 right-4 z-40 flex h-9 w-9 items-center justify-center rounded-md bg-indigo-500 text-white shadow-lg ring-1 ring-black/10 backdrop-blur transition-all hover:bg-indigo-600',
              showScrollButton
                ? 'translate-y-0 opacity-100'
                : 'translate-y-0 opacity-50',
            )}
            aria-label="Scroll to bottom"
          >
            <ArrowDown className="h-4 w-4" />
          </button>
        </main>

        {/* Input (fixed to bottom of main area) */}
        <footer className="theme-surface theme-border shrink-0 border-t p-0">
        <ChatInput
          onSend={handleSend}
          onStop={stopGeneration}
          isLoading={chatLoading}
          disabled={!config.api_token}
          config={config}
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
        onDeletePrompt={handleDeletePrompt}
        onExportData={exportAllData}
        onImportData={handleImportData}
        sessions={sessions}
        activeSessionId={sessionId}
        onCreateSession={createSession}
        onDeleteSession={deleteSession}
        onSwitchSession={switchSession}
        onRenameSession={renameSession}
        onDuplicateSession={duplicateSession}
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
          onClose={() => setEditingMessage(null)}
          onConfirm={handleConfirmEdit}
        />
      )}
    </div>
  )
}

interface EditMessageModalProps {
  content: string
  onClose: () => void
  onConfirm: (newContent: string) => void
}

function EditMessageModal({
  content,
  onClose,
  onConfirm,
}: EditMessageModalProps) {
  const [editedContent, setEditedContent] = useState(content)
  const textareaRef = useRef<HTMLTextAreaElement>(null)

  useEffect(() => {
    textareaRef.current?.focus()
    textareaRef.current?.select()
  }, [])

  const handleSubmit = useCallback(() => {
    if (editedContent.trim()) {
      onConfirm(editedContent)
    }
  }, [editedContent, onConfirm])

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
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm"
      onClick={onClose}
    >
      <div
        className="mx-4 w-full max-w-2xl rounded-lg border theme-border theme-elevated p-6 shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        <h3 className="mb-4 text-lg font-semibold">Edit Message</h3>
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
          <Button onClick={handleSubmit} disabled={!editedContent.trim()}>
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
