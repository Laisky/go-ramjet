/**
 * React hook for managing chat state and interactions.
 */
import { useCallback, useRef, useState } from 'react'

import {
  sendStreamingChatRequest,
  type ChatMessage as ApiChatMessage,
} from '@/utils/api'
import { kvDel, kvGet, kvSet, StorageKeys } from '@/utils/storage'
import { isImageModel } from '../models'
import type { ChatMessageData, SessionConfig, SessionHistoryItem } from '../types'

/**
 * Generate a unique chat ID
 */
function generateChatId(): string {
  const timestamp = Date.now()
  const random = Math.random().toString(36).substring(2, 8)
  return `chat-${timestamp}-${random}`
}

/**
 * Get session history key for a session ID
 */
function getSessionHistoryKey(sessionId: number): string {
  return `${StorageKeys.SESSION_HISTORY_PREFIX}${sessionId}`
}

/**
 * Get chat data key
 */
function getChatDataKey(chatId: string, role: 'user' | 'assistant'): string {
  return `${StorageKeys.CHAT_DATA_PREFIX}${role}_${chatId}`
}

export interface UseChatOptions {
  sessionId: number
  config: SessionConfig
}

export interface UseChatReturn {
  messages: ChatMessageData[]
  isLoading: boolean
  error: string | null
  sendMessage: (content: string, attachments?: File[]) => Promise<void>
  stopGeneration: () => void
  clearMessages: () => Promise<void>
  deleteMessage: (chatId: string) => Promise<void>
  loadMessages: () => Promise<void>
}

/**
 * Hook for managing chat messages and interactions
 */
export function useChat({ sessionId, config }: UseChatOptions): UseChatReturn {
  const [messages, setMessages] = useState<ChatMessageData[]>([])
  const [isLoading, setIsLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const abortControllerRef = useRef<AbortController | null>(null)
  const currentChatIdRef = useRef<string | null>(null)

  /**
   * Load messages from storage
   */
  const loadMessages = useCallback(async () => {
    try {
      const key = getSessionHistoryKey(sessionId)
      const history = await kvGet<SessionHistoryItem[]>(key)

      if (!history || history.length === 0) {
        setMessages([])
        return
      }

      // Load full message data for each history item
      const loadedMessages: ChatMessageData[] = []

      for (const item of history) {
        // Try to load from individual storage first
        const userKey = getChatDataKey(item.chatID, 'user')
        const assistantKey = getChatDataKey(item.chatID, 'assistant')

        const userData = await kvGet<ChatMessageData>(userKey)
        const assistantData = await kvGet<ChatMessageData>(assistantKey)

        if (userData) {
          loadedMessages.push(userData)
        }
        if (assistantData) {
          loadedMessages.push(assistantData)
        }
      }

      // Sort by chatID (which contains timestamp)
      loadedMessages.sort((a, b) => a.chatID.localeCompare(b.chatID))

      setMessages(loadedMessages)
    } catch (err) {
      console.error('Failed to load messages:', err)
      setError('Failed to load messages')
    }
  }, [sessionId])

  /**
   * Save a message to storage
   */
  const saveMessage = useCallback(
    async (message: ChatMessageData) => {
      // Save individual message data
      const key = getChatDataKey(message.chatID, message.role as 'user' | 'assistant')
      await kvSet(key, message)

      // Update session history
      const historyKey = getSessionHistoryKey(sessionId)
      const history = (await kvGet<SessionHistoryItem[]>(historyKey)) || []

      // Check if this chatID already exists
      const existingIndex = history.findIndex(
        (h) => h.chatID === message.chatID && h.role === message.role
      )

      const historyItem: SessionHistoryItem = {
        chatID: message.chatID,
        role: message.role as 'user' | 'assistant',
        content: message.content.substring(0, 100), // Preview only
        model: message.model,
        timestamp: message.timestamp,
      }

      if (existingIndex >= 0) {
        history[existingIndex] = historyItem
      } else {
        history.push(historyItem)
      }

      await kvSet(historyKey, history)
    },
    [sessionId]
  )

  /**
   * Send a message and get AI response
   */
  const sendMessage = useCallback(
    async (content: string, _attachments?: File[]) => {
      if (!content.trim()) return

      const chatId = generateChatId()
      currentChatIdRef.current = chatId
      setError(null)

      // Create user message
      const userMessage: ChatMessageData = {
        chatID: chatId,
        role: 'user',
        content: content.trim(),
        timestamp: Date.now(),
      }

      // Add user message to state
      setMessages((prev) => [...prev, userMessage])

      // Save user message
      await saveMessage(userMessage)

      // Check if this is an image model
      if (isImageModel(config.selected_model)) {
        // TODO: Handle image generation
        setError('Image generation not yet implemented in new UI')
        return
      }

      // Prepare assistant message placeholder
      const assistantMessage: ChatMessageData = {
        chatID: chatId,
        role: 'assistant',
        content: '',
        model: config.selected_model,
        timestamp: Date.now(),
      }

      setMessages((prev) => [...prev, assistantMessage])
      setIsLoading(true)

      // Build messages for API
      const apiMessages: ApiChatMessage[] = []

      // Add system prompt
      if (config.system_prompt) {
        apiMessages.push({
          role: 'system',
          content: config.system_prompt,
        })
      }

      // Add context messages (last n_contexts)
      const contextMessages = messages.slice(-config.n_contexts * 2)
      for (const msg of contextMessages) {
        apiMessages.push({
          role: msg.role,
          content: msg.content,
        })
      }

      // Add current user message
      apiMessages.push({
        role: 'user',
        content: content.trim(),
      })

      // Stream response
      let fullContent = ''
      let fullReasoning = ''

      abortControllerRef.current = sendStreamingChatRequest(
        {
          model: config.selected_model,
          messages: apiMessages,
          max_tokens: config.max_tokens,
          temperature: config.temperature,
          presence_penalty: config.presence_penalty,
          frequency_penalty: config.frequency_penalty,
          stream: true,
          enable_mcp: config.chat_switch.enable_mcp,
          mcp_servers: config.mcp_servers?.filter((s: { enabled: boolean }) => s.enabled).map((s: { name: string; url: string; api_key?: string }) => ({
            name: s.name,
            url: s.url,
            api_key: s.api_key,
          })),
        },
        config.api_token,
        {
          onContent: (chunk) => {
            fullContent += chunk
            setMessages((prev) =>
              prev.map((m) =>
                m.chatID === chatId && m.role === 'assistant'
                  ? { ...m, content: fullContent }
                  : m
              )
            )
          },
          onReasoning: (chunk) => {
            fullReasoning += chunk
            setMessages((prev) =>
              prev.map((m) =>
                m.chatID === chatId && m.role === 'assistant'
                  ? { ...m, reasoningContent: fullReasoning }
                  : m
              )
            )
          },
          onDone: async () => {
            setIsLoading(false)
            abortControllerRef.current = null
            currentChatIdRef.current = null

            // Save final assistant message
            const finalMessage: ChatMessageData = {
              chatID: chatId,
              role: 'assistant',
              content: fullContent,
              model: config.selected_model,
              reasoningContent: fullReasoning || undefined,
              timestamp: Date.now(),
            }
            await saveMessage(finalMessage)
          },
          onError: (err) => {
            setIsLoading(false)
            setError(err.message)
            abortControllerRef.current = null
            currentChatIdRef.current = null
          },
        },
        config.api_base !== 'https://api.openai.com' ? config.api_base : undefined
      )
    },
    [config, messages, saveMessage]
  )

  /**
   * Stop the current generation
   */
  const stopGeneration = useCallback(() => {
    if (abortControllerRef.current) {
      abortControllerRef.current.abort()
      abortControllerRef.current = null
      setIsLoading(false)
    }
  }, [])

  /**
   * Clear all messages in the current session
   */
  const clearMessages = useCallback(async () => {
    // Get all message IDs from history
    const historyKey = getSessionHistoryKey(sessionId)
    const history = await kvGet<SessionHistoryItem[]>(historyKey)

    if (history) {
      // Delete individual message data
      const chatIds = new Set(history.map((h) => h.chatID))
      for (const chatId of chatIds) {
        await kvDel(getChatDataKey(chatId, 'user'))
        await kvDel(getChatDataKey(chatId, 'assistant'))
      }
    }

    // Clear history
    await kvSet(historyKey, [])

    setMessages([])
  }, [sessionId])

  /**
   * Delete a specific message pair (user + assistant)
   */
  const deleteMessage = useCallback(
    async (chatId: string) => {
      // Delete from storage
      await kvDel(getChatDataKey(chatId, 'user'))
      await kvDel(getChatDataKey(chatId, 'assistant'))

      // Update history
      const historyKey = getSessionHistoryKey(sessionId)
      const history = await kvGet<SessionHistoryItem[]>(historyKey)
      if (history) {
        const newHistory = history.filter((h) => h.chatID !== chatId)
        await kvSet(historyKey, newHistory)
      }

      // Update state
      setMessages((prev) => prev.filter((m) => m.chatID !== chatId))
    },
    [sessionId]
  )

  return {
    messages,
    isLoading,
    error,
    sendMessage,
    stopGeneration,
    clearMessages,
    deleteMessage,
    loadMessages,
  }
}
