/**
 * GPTChat page - main chat interface.
 */
import { ArrowDown, Settings } from 'lucide-react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'

import { ThemeToggle } from '@/components/theme-toggle'
import { Button } from '@/components/ui/button'
import { cn } from '@/utils/cn'
import { setPageFavicon, setPageTitle } from '@/utils/dom'
import {
  ChatInput,
  ChatMessage,
  ChatSearch,
  ConfigSidebar,
  EditMessageModal,
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
  const {
    messagesEndRef,
    showScrollButton,
    visibleCount,
    autoScrollRef,
    suppressAutoScrollOnceRef,
    scrollToBottom,
    scrollToTop,
    handleLoadOlder,
    scrollToMessage,
  } = useChatScroll({
    messages,
    pageSize: MESSAGE_PAGE_SIZE,
    sessionId,
    contentRef: messagesContainerRef,
  })

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

  const footerRef = useRef<HTMLDivElement>(null)
  const [footerHeight, setFooterHeight] = useState(112) // Default pb-28 is 112px

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

  const { selectedMessageIndex, handleMessageSelect } = useMessageNavigation({
    displayedMessages,
    sessionId,
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
    if (autoScrollRef.current) {
      scrollToBottom({ force: false, behavior: 'auto' })
    }
  }, [footerHeight, scrollToBottom, autoScrollRef])

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
    await updateConfig(DefaultSessionConfig)
  }, [updateConfig])

  const handleSend = useCallback(
    async (content: string, attachments?: ChatAttachment[]) => {
      autoScrollRef.current = true
      await sendMessage(content, attachments)
      requestAnimationFrame(() => scrollToBottom({ force: true }))
    },
    [scrollToBottom, sendMessage, autoScrollRef],
  )

  const handleRegenerate = useCallback(
    async (chatId: string) => {
      // Do not auto-scroll on regenerate; keep viewport stable.
      autoScrollRef.current = false
      suppressAutoScrollOnceRef.current = true
      await regenerateMessage(chatId)
    },
    [regenerateMessage, autoScrollRef, suppressAutoScrollOnceRef],
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
      autoScrollRef.current = false
      suppressAutoScrollOnceRef.current = true
      setEditingMessage({
        chatId: payload.chatId,
        content: payload.content,
        attachments: payload.attachments,
      })
    },
    [autoScrollRef, suppressAutoScrollOnceRef],
  )

  const handleConfirmEdit = useCallback(
    async (newContent: string, attachments?: ChatAttachment[]) => {
      if (!editingMessage) return
      setEditingMessage(null)
      autoScrollRef.current = false
      suppressAutoScrollOnceRef.current = true
      await editAndRetry(editingMessage.chatId, newContent, attachments)
    },
    [editAndRetry, editingMessage, autoScrollRef, suppressAutoScrollOnceRef],
  )

  const handleClearChats = useCallback(async () => {
    await clearMessages()
  }, [clearMessages])

  const handlePurgeAllSessions = useCallback(async () => {
    await purgeAllSessions()
    await clearMessages()
  }, [purgeAllSessions, clearMessages])

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
      <div className="ml-10 flex min-h-dvh min-w-0 flex-col">
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
            <ChatSearch messages={messages} onSelectMessage={scrollToMessage} />
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
        <main
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
        </main>

        {/* Input (fixed to bottom of viewport) */}
        <footer
          ref={footerRef}
          className="theme-surface theme-border fixed bottom-0 left-10 right-0 z-30 border-t p-0"
        >
          {/* Scroll to bottom button */}
          <button
            onClick={() => scrollToBottom({ force: true })}
            className={cn(
              'absolute bottom-full right-2 mb-4 z-40 flex h-9 w-9 items-center justify-center rounded-md bg-muted text-muted-foreground shadow-lg ring-1 ring-border backdrop-blur transition-all hover:bg-muted/80',
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
        onExportSession={exportSessionToXml}
      />

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

      {/* Edit Message Modal */}
      {editingMessage && (
        <EditMessageModal
          content={editingMessage.content}
          attachments={editingMessage.attachments}
          apiToken={config.api_token}
          onClose={() => setEditingMessage(null)}
          onConfirm={handleConfirmEdit}
        />
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
