/**
 * GPTChat page - main chat interface.
 */
import { Settings, ArrowDown } from 'lucide-react'
import { useCallback, useEffect, useRef, useState } from 'react'

import { Button } from '@/components/ui/button'
import { cn } from '@/utils/cn'
import { kvGet, kvSet, StorageKeys } from '@/utils/storage'
import { ChatMessage, ChatInput, ConfigSidebar, ModelSelector } from './components'
import { useChat } from './hooks/use-chat'
import { useConfig } from './hooks/use-config'
import type { PromptShortcut, SessionConfig } from './types'
import { DefaultSessionConfig } from './types'

/**
 * GPTChatPage provides a full-featured chat interface.
 */
export function GPTChatPage() {
  const { config, sessionId, isLoading: configLoading, updateConfig } = useConfig()
  const {
    messages,
    isLoading: chatLoading,
    error,
    sendMessage,
    stopGeneration,
    clearMessages,
    deleteMessage,
    loadMessages,
  } = useChat({ sessionId, config })

  const [configOpen, setConfigOpen] = useState(false)
  const [promptShortcuts, setPromptShortcuts] = useState<PromptShortcut[]>([])
  const messagesEndRef = useRef<HTMLDivElement>(null)
  const messagesContainerRef = useRef<HTMLDivElement>(null)
  const [showScrollButton, setShowScrollButton] = useState(false)

  // Load messages and shortcuts on mount
  useEffect(() => {
    if (!configLoading) {
      loadMessages()
      loadPromptShortcuts()
    }
  }, [configLoading, loadMessages])

  // Auto-scroll on new messages
  useEffect(() => {
    scrollToBottom()
  }, [messages])

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

  const scrollToBottom = useCallback(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [])

  const loadPromptShortcuts = async () => {
    const shortcuts = await kvGet<PromptShortcut[]>(StorageKeys.PROMPT_SHORTCUTS)
    if (shortcuts) {
      setPromptShortcuts(shortcuts)
    }
  }

  const handleSavePrompt = useCallback(
    async (name: string, prompt: string) => {
      const newShortcut: PromptShortcut = { name, prompt }
      const updated = [...promptShortcuts, newShortcut]
      setPromptShortcuts(updated)
      await kvSet(StorageKeys.PROMPT_SHORTCUTS, updated)
    },
    [promptShortcuts]
  )

  const handleConfigChange = useCallback(
    (updates: Partial<SessionConfig>) => {
      updateConfig(updates)
    },
    [updateConfig]
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
    [config.chat_switch, updateConfig]
  )

  const handleReset = useCallback(async () => {
    if (window.confirm('Reset all settings to defaults? This will not delete your chat history.')) {
      await updateConfig(DefaultSessionConfig)
    }
  }, [updateConfig])

  const handleClearChats = useCallback(async () => {
    if (window.confirm('Clear all chat history in this session?')) {
      await clearMessages()
    }
  }, [clearMessages])

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
    <div className="flex h-[calc(100vh-100px)] flex-col">
      {/* Header */}
      <div className="flex items-center justify-between border-b border-black/10 pb-3 dark:border-white/10">
        <div className="flex items-center gap-3">
          <h1 className="text-xl font-semibold">Chat</h1>
          <ModelSelector
            selectedModel={config.selected_model}
            onModelChange={(model) => handleConfigChange({ selected_model: model })}
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
              Type a message below to begin chatting with the AI. You can change
              the model and settings using the button above.
            </p>
          </div>
        ) : (
          <div className="space-y-6 pb-4">
            {messages.map((msg, idx) => (
              <ChatMessage
                key={`${msg.chatID}-${msg.role}`}
                message={msg}
                onDelete={deleteMessage}
                isStreaming={
                  chatLoading &&
                  msg.role === 'assistant' &&
                  idx === messages.length - 1
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
              : 'translate-y-4 opacity-0 pointer-events-none'
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
          placeholder={
            config.api_token
              ? 'Type a message...'
              : 'Enter your API key in Settings to start chatting'
          }
        />
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
      />
    </div>
  )
}
