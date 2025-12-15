/**
 * GPTChat page - main chat interface.
 */
import { ArrowDown, Settings } from 'lucide-react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'

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

  const scrollToBottom = useCallback(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' })
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

  // Auto-scroll on new messages
  useEffect(() => {
    scrollToBottom()
  }, [messages, scrollToBottom])

  useEffect(() => {
    if (messages.length === 0) {
      setVisibleCount(MESSAGE_PAGE_SIZE)
      return
    }

    setVisibleCount((prev) => {
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
      const { scrollTop, scrollHeight, clientHeight } = container
      const isNearBottom = scrollHeight - scrollTop - clientHeight < 100
      setShowScrollButton(!isNearBottom)
    }

    container.addEventListener('scroll', handleScroll)
    return () => container.removeEventListener('scroll', handleScroll)
  }, [])

  const handleLoadOlder = useCallback(() => {
    setVisibleCount((prev) =>
      Math.min(prev + MESSAGE_PAGE_SIZE, messages.length),
    )
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

  const handleReset = useCallback(async () => {
    await updateConfig(DefaultSessionConfig)
  }, [updateConfig])

  const handleEditResend = useCallback(
    (payload: { chatId: string; content: string }) => {
      setPrefillDraft({ id: payload.chatId, text: payload.content })
      // Scroll to bottom so the input is visible for edit
      requestAnimationFrame(scrollToBottom)
    },
    [scrollToBottom],
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

  if (configLoading) {
    return (
      <div className="flex h-[calc(100vh-100px)] items-center justify-center">
        <div className="text-center">
          <div className="mb-2 h-8 w-8 animate-spin rounded-full border-2 border-black/20 border-t-black dark:border-white/20 dark:border-t-white" />
          <p className="text-sm text-black/50 dark:text-white/50">Loading...</p>
        </div>
      </div>
    )
  }

  return (
    <div className="flex h-[calc(100vh-100px)] flex-row bg-white dark:bg-black overflow-hidden relative">
      {/* Session Dock (Left Sidebar) */}
      <SessionDock
        sessions={sessions}
        activeSessionId={sessionId}
        onSwitchSession={switchSession}
        onCreateSession={() => createSession()}
        onDeleteSession={deleteSession}
      />

      <div className="flex-1 flex flex-col min-w-0 h-full relative">
        {/* Header */}
        <div className="flex items-center justify-between border-b border-black/10 pb-3 dark:border-white/10">
          <div className="flex items-center gap-3">
            <h1 className="text-xl font-semibold">Chat</h1>
            <ModelSelector
              selectedModel={config.selected_model}
              onModelChange={(model) =>
                handleConfigChange({ selected_model: model })
              }
            />
          </div>
          <Button
            variant="ghost"
            size="sm"
            onClick={() => setConfigOpen(true)}
            className="flex items-center gap-1"
          >
            <Settings className="h-4 w-4" />
            <span className="hidden sm:inline">Settings</span>
          </Button>
        </div>

        {/* Error display */}
        {error && (
          <div className="mt-2 rounded-md bg-red-100 p-3 text-sm text-red-700 dark:bg-red-900/20 dark:text-red-400">
            {error}
          </div>
        )}

        {/* Messages */}
        <div
          ref={messagesContainerRef}
          className="relative flex-1 overflow-y-auto py-4"
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
            <div className="space-y-6 pb-4">
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
                  onRegenerate={regenerateMessage}
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

          {/* Scroll to bottom button */}
          <button
            onClick={scrollToBottom}
            className={cn(
              'fixed bottom-32 right-8 flex h-10 w-10 items-center justify-center rounded-full bg-black/80 text-white shadow-lg transition-all dark:bg-white/80 dark:text-black',
              showScrollButton
                ? 'translate-y-0 opacity-100'
                : 'translate-y-4 opacity-0 pointer-events-none',
            )}
          >
            <ArrowDown className="h-5 w-5" />
          </button>
        </div>

        {/* Input */}
        <div className="border-t border-black/10 pt-3 dark:border-white/10">
          <ChatInput
            onSend={sendMessage}
            onStop={stopGeneration}
            isLoading={chatLoading}
            disabled={!config.api_token}
            config={config}
            onConfigChange={handleChatSwitchChange}
            prefillDraft={prefillDraft}
            onPrefillUsed={() => setPrefillDraft(undefined)}
            placeholder={
              config.api_token
                ? 'Type a message...'
                : 'Enter your API key in Settings to start chatting'
            }
          />
        </div>
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
        <div className="fixed bottom-4 right-4 z-50 max-w-sm rounded-lg border border-black/10 bg-white p-4 shadow-lg dark:border-white/10 dark:bg-black">
          <p className="text-sm font-medium">New version available</p>
          <p className="text-xs text-black/60 dark:text-white/60">
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
    </div>
  )
}
