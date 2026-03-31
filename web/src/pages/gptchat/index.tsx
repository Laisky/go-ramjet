/**
 * GPTChat page - main chat interface.
 */
import { ArrowDown, ArrowUp, Settings } from 'lucide-react'
import {
  Suspense,
  lazy,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react'

import { ThemeToggle } from '@/components/theme-toggle'
import { Button } from '@/components/ui/button'
import { TooltipWrapper } from '@/components/ui/tooltip-wrapper'
import { cn } from '@/utils/cn'
import { setPageFavicon, setPageTitle } from '@/utils/dom'
import {
  ChatInput,
  ChatMessage,
  ChatSearch,
  FloatingMessageHeader,
  ModelSelector,
  SelectionTTSPlayer,
  SelectionToolbar,
  SessionDock,
  UpgradeNotification,
} from './components'
import { useChat } from './hooks/use-chat'
import { useChatScroll } from './hooks/use-chat-scroll'
import { useConfig } from './hooks/use-config'
import { useDraft } from './hooks/use-draft'
import { useFloatingHeader } from './hooks/use-floating-header'
import { useMcpSync } from './hooks/use-mcp-sync'
import { useMessageNavigation } from './hooks/use-message-navigation'
import { usePromptShortcuts } from './hooks/use-prompt-shortcuts'
import { useSelection } from './hooks/use-selection'
import { useTTS } from './hooks/use-tts'
import { useUser } from './hooks/use-user'
import { useVersionCheck } from './hooks/use-version-check'
import { ImageModelFluxDev, isImageModel } from './models'
import type {
  ChatAttachment,
  ChatMessageData,
  SelectionData,
  SessionConfig,
} from './types'
import { DefaultSessionConfig } from './types'
import { exportSessionToXml } from './utils/session-export'

// Lazy load heavy sidebar & modal components - only needed on user interaction
const ConfigSidebar = lazy(() =>
  import('./components/config-sidebar').then((m) => ({
    default: m.ConfigSidebar,
  })),
)
const EditMessageModal = lazy(() =>
  import('./components/edit-message-modal').then((m) => ({
    default: m.EditMessageModal,
  })),
)

const MESSAGE_PAGE_SIZE = 40

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
    forkSession,
    purgeAllSessions,
    exportAllData,
    importAllData,
  } = useConfig()
  const {
    messages,
    isLoading: chatLoading,
    loadingChatId,
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
  const {
    promptShortcuts,
    handleSavePrompt,
    handleEditPrompt,
    handleDeletePrompt,
  } = usePromptShortcuts(configLoading)
  const messagesContainerRef = useRef<HTMLDivElement>(null)
  const footerRef = useRef<HTMLDivElement>(null)
  const [footerHeight, setFooterHeight] = useState(112) // Default pb-28 is 112px
  const {
    messagesEndRef,
    showScrollButton,
    visibleCount,
    scrollModeRef,
    lockViewport,
    scrollToBottom,
    scrollToTop,
    resetScroll,
    handleLoadOlder,
    scrollToMessage,
  } = useChatScroll({
    messages,
    pageSize: MESSAGE_PAGE_SIZE,
    sessionId,
    contentRef: messagesContainerRef,
    footerHeight,
  })

  // Cross-session search: when a result from another session is selected,
  // switch session and then scroll to the message once it loads.
  const [pendingScrollTarget, setPendingScrollTarget] = useState<{
    chatId: string
    role: string
  } | null>(null)

  useEffect(() => {
    if (!pendingScrollTarget) return
    const { chatId, role } = pendingScrollTarget
    const exists = messages.some((m) => m.chatID === chatId && m.role === role)
    if (exists) {
      setPendingScrollTarget(null)
      scrollToMessage(chatId, role)
    }
  }, [messages, pendingScrollTarget, scrollToMessage])

  const handleSearchSwitchAndSelect = useCallback(
    (targetSessionId: number, chatId: string, role: string) => {
      setPendingScrollTarget({ chatId, role })
      switchSession(targetSessionId)
    },
    [switchSession],
  )

  const { upgradeInfo, setUpgradeInfo, ignoreVersion } = useVersionCheck()
  const [prefillDraft, setPrefillDraft] = useState<
    | {
        id: string
        text: string
      }
    | undefined
  >(undefined)
  const { globalDraft, setGlobalDraft } = useDraft()

  const [editingMessage, setEditingMessage] = useState<{
    chatId: string
    content: string
    attachments?: ChatAttachment[]
  } | null>(null)

  // Monitor footer height to adjust main content padding
  useEffect(() => {
    const footer = footerRef.current
    if (!footer) return
    if (typeof ResizeObserver === 'undefined') {
      return
    }

    const observer = new ResizeObserver((entries) => {
      for (const entry of entries) {
        if (entry.target instanceof HTMLElement) {
          setFooterHeight(entry.target.offsetHeight)
        }
      }
    })

    observer.observe(footer)
    return () => observer.disconnect()
  }, [])

  const displayedMessages = useMemo(() => {
    if (messages.length <= visibleCount) {
      return messages
    }
    return messages.slice(-visibleCount)
  }, [messages, visibleCount])

  const { selectedMessageIndex, handleMessageSelect, navigateMessageUp } =
    useMessageNavigation({
      displayedMessages,
      sessionId,
      onNavigate: lockViewport,
    })

  const { selectionData, setSelectionData } = useSelection(messagesContainerRef)
  const [inputSelectionData, setInputSelectionData] =
    useState<SelectionData | null>(null)
  const activeSelectionData = inputSelectionData || selectionData

  const {
    requestTTS,
    stopTTS,
    isLoading: isTtsLoading,
    error: ttsError,
    audioUrl: ttsAudioUrl,
  } = useTTS({
    apiToken: config.api_token || '',
  })
  const { user } = useUser(config.api_token)

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

  useMcpSync(config, configLoading, updateConfig)

  // Scroll to bottom when footer height changes (e.g. typing long prompt)
  // to prevent messages from being covered.
  useEffect(() => {
    if (scrollModeRef.current === 'auto-follow') {
      scrollToBottom({ force: false, behavior: 'auto' })
    }
  }, [footerHeight, scrollToBottom, scrollModeRef])

  const currentDraftMessage = globalDraft

  const userMessageByChatId = useMemo(() => {
    const map = new Map<string, ChatMessageData>()
    messages.forEach((msg) => {
      if (msg.role === 'user') {
        map.set(msg.chatID, msg)
      }
    })
    return map
  }, [messages])

  // Track which message's header should appear in the floating header
  const floatingHeaderState = useFloatingHeader({
    messages: displayedMessages,
    containerRef: messagesContainerRef,
    topOffset: 48, // Height of the fixed header (top-12 = 48px)
  })

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
    // Reset all config except sync_key
    const { sync_key: __unused, ...rest } = DefaultSessionConfig // eslint-disable-line @typescript-eslint/no-unused-vars
    await updateConfig(rest)
  }, [updateConfig])

  const handleSend = useCallback(
    async (content: string, attachments?: ChatAttachment[]) => {
      scrollModeRef.current = 'auto-follow'
      await sendMessage(content, attachments)
      requestAnimationFrame(() => scrollToBottom({ force: true }))
    },
    [scrollToBottom, sendMessage, scrollModeRef],
  )

  const handleRegenerate = useCallback(
    async (chatId: string) => {
      // Lock viewport so streaming response doesn't auto-scroll.
      lockViewport()
      await regenerateMessage(chatId)
    },
    [regenerateMessage, lockViewport],
  )

  const handleFork = useCallback(
    async (chatId: string, role: string) => {
      const newSessionId = await forkSession(
        sessionId,
        chatId,
        role as 'user' | 'assistant',
      )
      if (newSessionId) {
        switchSession(newSessionId)
      }
    },
    [forkSession, sessionId, switchSession],
  )

  const handleEditResend = useCallback(
    (payload: {
      chatId: string
      content: string
      attachments?: ChatAttachment[]
    }) => {
      lockViewport()
      setEditingMessage({
        chatId: payload.chatId,
        content: payload.content,
        attachments: payload.attachments,
      })
    },
    [lockViewport],
  )

  const handleConfirmEdit = useCallback(
    async (newContent: string, attachments?: ChatAttachment[]) => {
      if (!editingMessage) return
      setEditingMessage(null)
      lockViewport()
      await editAndRetry(editingMessage.chatId, newContent, attachments)
    },
    [editAndRetry, editingMessage, lockViewport],
  )

  const handleClearChats = useCallback(async () => {
    await clearMessages()
    resetScroll()
  }, [clearMessages, resetScroll])

  const handlePurgeAllSessions = useCallback(async () => {
    await purgeAllSessions()
    await clearMessages()
    resetScroll()
  }, [purgeAllSessions, clearMessages, resetScroll])

  const handleImportData = useCallback(
    async (data: unknown, mode: 'merge' | 'download' = 'merge') => {
      // Accept any shape but ensure we pass an object map to storage importer
      await importAllData((data as Record<string, unknown>) || {}, mode)
    },
    [importAllData],
  )

  const handleDraftChange = useCallback(
    (value: string) => {
      setGlobalDraft(value)
    },
    [setGlobalDraft],
  )

  const handleQuote = useCallback(
    (text: string) => {
      setPrefillDraft({ id: Date.now().toString(), text })
      setSelectionData(null)
      setInputSelectionData(null)
    },
    [setSelectionData],
  )

  /**
   * handleInputSelectionChange updates selection state from the input textarea.
   */
  const handleInputSelectionChange = useCallback(
    (data: SelectionData | null) => {
      setInputSelectionData(data)
      if (data) {
        setSelectionData(null)
      }
    },
    [setSelectionData],
  )

  /**
   * clearSelection resets selection toolbar state.
   */
  const clearSelection = useCallback(() => {
    setSelectionData(null)
    setInputSelectionData(null)
  }, [setSelectionData])

  const handleSelectionCopy = useCallback(async () => {
    if (activeSelectionData) {
      try {
        await navigator.clipboard.writeText(
          activeSelectionData.copyText || activeSelectionData.text,
        )
      } catch (err) {
        console.error('Failed to copy selection:', err)
      }
    }
  }, [activeSelectionData])

  const handleSelectionTTS = useCallback(() => {
    if (activeSelectionData) {
      requestTTS(activeSelectionData.text)
    }
  }, [activeSelectionData, requestTTS])

  return (
    <div className="theme-bg min-h-dvh w-full max-w-full overflow-x-hidden">
      {/* Session Dock (Fixed Left Sidebar) */}
      <aside className="fixed left-0 top-0 z-40 flex h-dvh w-12 shrink-0 flex-col border-r border-primary/20 bg-primary/5 dark:bg-primary/8">
        {/* Dock header area */}
        <div className="flex h-12 shrink-0 items-center justify-center border-b border-primary/20">
          <span className="text-base">💬</span>
        </div>
        {/* Session buttons */}
        <SessionDock
          sessions={sessions}
          activeSessionId={sessionId}
          onSwitchSession={switchSession}
          onCreateSession={() => createSession()}
          onClearChats={handleClearChats}
          onReorderSessions={reorderSessions}
          onRenameSession={renameSession}
        />
      </aside>

      {/* Main Content Area - offset by sidebar width */}
      <div className="ml-12 flex min-h-dvh min-w-0 flex-col">
        {/* Header - fixed at top */}
        <header
          className="theme-surface theme-border fixed left-12 right-0 top-0 z-30 flex h-12 shrink-0 items-center gap-2 border-b px-2 sm:px-3"
          onClick={(e) => {
            if (e.target !== e.currentTarget) {
              return
            }
            scrollToTop()
          }}
        >
          <div
            className="flex min-w-0 items-center gap-2 sm:gap-2.5"
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
              allowedModels={user?.allowed_models}
              selectedModel={chatModel}
              active={!isDrawActive}
              onModelChange={handleChatModelChange}
              className="h-9 shrink-0 min-w-[3.9rem] rounded-xl bg-background/70 px-2.5 shadow-sm shadow-primary/5 sm:min-w-[4.2rem]"
              compact
              tone="ghost"
            />
            <ModelSelector
              label="Draw"
              categories={['Image']}
              allowedModels={user?.allowed_models}
              selectedModel={drawModel}
              active={isDrawActive}
              onModelChange={handleDrawModelChange}
              className="h-9 shrink-0 min-w-[3.9rem] rounded-xl bg-background/70 px-2.5 shadow-sm shadow-primary/5 sm:min-w-[4.2rem]"
              compact
              tone="ghost"
            />
            <TooltipWrapper content="Pay / Top up">
              <Button
                variant="ghost"
                size="sm"
                className="h-9 shrink-0 rounded-xl bg-background/70 px-2.5 text-sm font-semibold shadow-sm shadow-primary/5 sm:min-w-[4.5rem]"
                onClick={() =>
                  window.open(
                    'https://wiki.laisky.com/projects/gpt/pay/#page_gpt_pay',
                    '_blank',
                    'noopener,noreferrer',
                  )
                }
              >
                Pay
              </Button>
            </TooltipWrapper>
          </div>

          <div className="ml-auto flex shrink-0 items-center gap-1 sm:gap-1.5">
            <ChatSearch
              messages={messages}
              sessions={sessions}
              currentSessionId={sessionId}
              onSelectMessage={scrollToMessage}
              onSwitchAndSelect={handleSearchSwitchAndSelect}
            />
            <ThemeToggle className="h-9 w-9" />
            <TooltipWrapper content="Settings">
              <Button
                variant="ghost"
                size="sm"
                onClick={() => setConfigOpen(true)}
                className="h-9 w-9 rounded-lg px-0"
                aria-label="Settings"
              >
                <Settings className="h-4 w-4" />
              </Button>
            </TooltipWrapper>
          </div>
        </header>

        {/* Floating message header - appears when a message's inline header scrolls out of view */}
        <FloatingMessageHeader
          messages={messages}
          chatId={floatingHeaderState.chatId}
          role={floatingHeaderState.role}
          visible={floatingHeaderState.visible}
          onDelete={deleteMessage}
          onRegenerate={handleRegenerate}
          onEditResend={handleEditResend}
          onFork={handleFork}
          pairedUserMessage={
            floatingHeaderState.chatId
              ? userMessageByChatId.get(floatingHeaderState.chatId)
              : undefined
          }
          apiToken={config.api_token}
          messageIndex={floatingHeaderState.index}
          onSelect={handleMessageSelect}
          isStreaming={
            chatLoading &&
            floatingHeaderState.role === 'assistant' &&
            floatingHeaderState.chatId === loadingChatId
          }
        />

        {/* Scrollable chat area - uses window scroll with padding for fixed header/footer */}
        <section
          className="relative pt-12"
          style={{ paddingBottom: `${footerHeight}px` }}
        >
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
            key={sessionId}
            ref={messagesContainerRef}
            className="min-h-0 overflow-x-hidden px-1 pt-1 sm:px-2 sm:pt-1.5 md:px-4"
          >
            {messages.length === 0 ? (
              <div className="flex min-h-[calc(100dvh-10rem)] flex-col items-center justify-center text-center">
                <div className="mb-4 flex h-14 w-14 items-center justify-center rounded-2xl bg-primary/10 text-3xl ring-1 ring-primary/20">
                  💬
                </div>
                <h2 className="mt-2 text-lg font-semibold text-primary">
                  Start a conversation
                </h2>
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
                    onFork={handleFork}
                    pairedUserMessage={userMessageByChatId.get(msg.chatID)}
                    isSelected={idx === selectedMessageIndex}
                    onSelect={handleMessageSelect}
                    messageIndex={idx}
                    apiToken={config.api_token}
                    isStreaming={
                      chatLoading &&
                      msg.role === 'assistant' &&
                      msg.chatID === loadingChatId
                    }
                  />
                ))}
                <div ref={messagesEndRef} />
              </div>
            )}
          </div>
        </section>

        {/* Input (fixed to bottom of viewport) */}
        <footer
          ref={footerRef}
          className="theme-surface theme-border fixed bottom-0 left-12 right-0 z-30 border-t p-0"
        >
          {/* Scroll up button – always visible */}
          <button
            onClick={() => {
              lockViewport()
              navigateMessageUp()
            }}
            className="absolute bottom-full right-2 mb-14 z-40 flex h-9 w-9 items-center justify-center rounded-md bg-primary/10 text-primary shadow-lg ring-1 ring-primary/30 backdrop-blur transition-all hover:bg-primary/20"
            aria-label="Scroll up by message"
          >
            <ArrowUp className="h-4 w-4" />
          </button>

          {/* Scroll to bottom button */}
          <button
            onClick={() => scrollToBottom({ force: true })}
            className={cn(
              'absolute bottom-full right-2 mb-4 z-40 flex h-9 w-9 items-center justify-center rounded-md bg-primary/10 text-primary shadow-lg ring-1 ring-primary/30 backdrop-blur transition-all hover:bg-primary/20',
              showScrollButton
                ? 'translate-y-0 opacity-100'
                : 'translate-y-4 opacity-0 pointer-events-none',
            )}
            aria-label="Scroll to bottom"
          >
            <ArrowDown className="h-4 w-4" />
          </button>
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
            onSelectionChange={handleInputSelectionChange}
          />
        </footer>
      </div>

      {/* Config Sidebar - lazy loaded */}
      {configOpen && (
        <Suspense fallback={null}>
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
            onExportSession={exportSessionToXml}
          />
        </Suspense>
      )}

      {upgradeInfo && (
        <div
          className="fixed z-50"
          style={{ bottom: `${footerHeight + 12}px`, right: '1rem' }}
        >
          <UpgradeNotification
            from={upgradeInfo.from}
            to={upgradeInfo.to}
            onClose={() => setUpgradeInfo(null)}
            onIgnore={() => ignoreVersion(upgradeInfo.to)}
          />
        </div>
      )}

      {/* Edit Message Modal - lazy loaded */}
      {editingMessage && (
        <Suspense fallback={null}>
          <EditMessageModal
            content={editingMessage.content}
            attachments={editingMessage.attachments}
            apiToken={config.api_token}
            onClose={() => setEditingMessage(null)}
            onConfirm={handleConfirmEdit}
          />
        </Suspense>
      )}

      {/* Selection Toolbar */}
      {activeSelectionData && (
        <SelectionToolbar
          text={activeSelectionData.text}
          position={activeSelectionData.position}
          onCopy={handleSelectionCopy}
          onTTS={handleSelectionTTS}
          onQuote={
            activeSelectionData.source === 'message' ? handleQuote : undefined
          }
          onClose={clearSelection}
        />
      )}

      {/* Selection TTS Player */}
      {(ttsAudioUrl || isTtsLoading || ttsError) && (
        <div
          className="fixed left-1/2 z-50 -translate-x-1/2"
          style={{ bottom: `${footerHeight + 12}px` }}
        >
          <SelectionTTSPlayer
            audioUrl={ttsAudioUrl}
            isLoading={isTtsLoading}
            error={ttsError}
            onClose={stopTTS}
          />
        </div>
      )}
    </div>
  )
}
