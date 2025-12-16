import { useCallback } from 'react'

import { kvDel, kvGet, kvSet } from '@/utils/storage'

import type { ChatMessageData, SessionHistoryItem } from '../types'
import { getChatDataKey, getSessionHistoryKey } from '../utils/chat-storage'

export interface ChatStorageOptions {
  sessionId: number
  setMessages: React.Dispatch<React.SetStateAction<ChatMessageData[]>>
  setError: (value: string | null) => void
}

export interface ChatStorageApi {
  loadMessages: () => Promise<void>
  saveMessage: (message: ChatMessageData) => Promise<void>
  clearMessages: () => Promise<void>
  deleteMessage: (chatId: string) => Promise<void>
}

/**
 * useChatStorage provides persistence helpers for chat sessions.
 * It handles loading history, saving messages, and removing chat pairs.
 */
export function useChatStorage({
  sessionId,
  setMessages,
  setError,
}: ChatStorageOptions): ChatStorageApi {
  const loadMessages = useCallback(async () => {
    try {
      const key = getSessionHistoryKey(sessionId)
      const history = await kvGet<SessionHistoryItem[]>(key)

      if (!history || history.length === 0) {
        setMessages([])
        return
      }

      const loadedMessages: ChatMessageData[] = []
      const seenChatIds = new Set<string>()

      for (const item of history) {
        if (seenChatIds.has(item.chatID)) {
          continue
        }
        seenChatIds.add(item.chatID)

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

      setMessages(loadedMessages)
    } catch (err) {
      console.error('Failed to load messages:', err)
      setError('Failed to load messages')
    }
  }, [sessionId, setMessages, setError])

  const saveMessage = useCallback(
    async (message: ChatMessageData) => {
      const key = getChatDataKey(
        message.chatID,
        message.role as 'user' | 'assistant',
      )
      await kvSet(key, message)

      const historyKey = getSessionHistoryKey(sessionId)
      const history = (await kvGet<SessionHistoryItem[]>(historyKey)) || []

      const existingIndex = history.findIndex(
        (h) => h.chatID === message.chatID && h.role === message.role,
      )

      const historyItem: SessionHistoryItem = {
        chatID: message.chatID,
        role: message.role as 'user' | 'assistant',
        content: message.content.substring(0, 100),
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
    [sessionId],
  )

  const clearMessages = useCallback(async () => {
    const historyKey = getSessionHistoryKey(sessionId)
    const history = await kvGet<SessionHistoryItem[]>(historyKey)

    if (history) {
      const chatIds = new Set(history.map((h) => h.chatID))
      for (const chatId of chatIds) {
        await kvDel(getChatDataKey(chatId, 'user'))
        await kvDel(getChatDataKey(chatId, 'assistant'))
      }
    }

    await kvSet(historyKey, [])
    setMessages([])
  }, [sessionId, setMessages])

  const deleteMessage = useCallback(
    async (chatId: string) => {
      await kvDel(getChatDataKey(chatId, 'user'))
      await kvDel(getChatDataKey(chatId, 'assistant'))

      const historyKey = getSessionHistoryKey(sessionId)
      const history = await kvGet<SessionHistoryItem[]>(historyKey)
      if (history) {
        const newHistory = history.filter((h) => h.chatID !== chatId)
        await kvSet(historyKey, newHistory)
      }

      setMessages((prev) => prev.filter((m) => m.chatID !== chatId))
    },
    [sessionId, setMessages],
  )

  return { loadMessages, saveMessage, clearMessages, deleteMessage }
}
