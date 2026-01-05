import { useCallback, useRef } from 'react'

import { kvDel, kvGet, kvSet } from '@/utils/storage'

import type { ChatMessageData, SessionHistoryItem } from '../types'
import { getChatDataKey, getSessionHistoryKey } from '../utils/chat-storage'
import { recordDeletedChatId } from '../utils/deleted-chat-ids'
import { uuidv7 } from '../utils/uuidv7'

/**
 * Sanitizes a ChatMessageData object to ensure correct types.
 * This handles backward compatibility for stored data that may have
 * incorrect types (e.g., costUsd stored as string instead of number).
 *
 * @param data - The raw message data from storage
 * @returns Sanitized ChatMessageData with correct types
 */
export function sanitizeChatMessageData(
  data: ChatMessageData,
): ChatMessageData {
  const sanitized = { ...data }

  // Ensure content is a string
  if (typeof sanitized.content !== 'string') {
    sanitized.content = String(sanitized.content ?? '')
  }

  // Clean up old image markdown from user messages to avoid double rendering
  // and broken image icons in the markdown view.
  if (sanitized.role === 'user' && sanitized.content) {
    sanitized.content = sanitized.content
      .replace(/!\[.*?\]\(data:image\/.*?;base64,.*?\)/g, '')
      .trim()
  }

  // Ensure costUsd is a number or undefined (null becomes undefined)
  if (sanitized.costUsd === null) {
    sanitized.costUsd = undefined
  } else if (sanitized.costUsd !== undefined) {
    const numValue =
      typeof sanitized.costUsd === 'number'
        ? sanitized.costUsd
        : Number(sanitized.costUsd)
    sanitized.costUsd = Number.isNaN(numValue) ? undefined : numValue
  }

  // Ensure timestamp is a number or undefined (null becomes undefined)
  if (sanitized.timestamp === null) {
    sanitized.timestamp = undefined
  } else if (sanitized.timestamp !== undefined) {
    const numValue =
      typeof sanitized.timestamp === 'number'
        ? sanitized.timestamp
        : Number(sanitized.timestamp)
    sanitized.timestamp = Number.isNaN(numValue) ? undefined : numValue
  }

  if (sanitized.edited_version === null) {
    sanitized.edited_version = undefined
  } else if (sanitized.edited_version !== undefined) {
    sanitized.edited_version = String(sanitized.edited_version)
  }

  return sanitized
}

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
  const sessionIdRef = useRef(sessionId)
  sessionIdRef.current = sessionId

  const loadMessages = useCallback(async () => {
    const loadingSessionId = sessionId
    try {
      const key = getSessionHistoryKey(sessionId)
      const history = await kvGet<SessionHistoryItem[]>(key)

      if (loadingSessionId !== sessionIdRef.current) {
        return
      }

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

        if (userData && typeof userData === 'object' && userData.content) {
          loadedMessages.push(sanitizeChatMessageData(userData))
        }
        if (
          assistantData &&
          typeof assistantData === 'object' &&
          assistantData.content
        ) {
          loadedMessages.push(sanitizeChatMessageData(assistantData))
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

      const existing = await kvGet<ChatMessageData>(key)
      const needsBump =
        existing &&
        typeof existing === 'object' &&
        existing.content !== undefined &&
        String(existing.content) !== String(message.content)

      const toSave: ChatMessageData = needsBump
        ? { ...message, edited_version: uuidv7() }
        : message

      await kvSet(key, toSave)

      const historyKey = getSessionHistoryKey(sessionId)
      const history = (await kvGet<SessionHistoryItem[]>(historyKey)) || []

      const existingIndex = history.findIndex(
        (h) => h.chatID === message.chatID && h.role === message.role,
      )

      const historyItem: SessionHistoryItem = {
        chatID: toSave.chatID,
        role: toSave.role as 'user' | 'assistant',
        content: toSave.content.substring(0, 100),
        model: toSave.model,
        timestamp: toSave.timestamp,
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
        await recordDeletedChatId(chatId)
        await kvDel(getChatDataKey(chatId, 'user'))
        await kvDel(getChatDataKey(chatId, 'assistant'))
      }
    }

    await kvSet(historyKey, [])
    setMessages([])
  }, [sessionId, setMessages])

  const deleteMessage = useCallback(
    async (chatId: string) => {
      await recordDeletedChatId(chatId)
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
