/**
 * Shared helpers for chat storage keys and identifiers.
 */
import { StorageKeys } from '@/utils/storage'

export type SupportedChatRole = 'user' | 'assistant'

/**
 * generateChatId creates a unique chat identifier preserving chronological order.
 */
export function generateChatId(): string {
  const timestamp = Date.now()
  const random = Math.random().toString(36).substring(2, 8)
  return `chat-${timestamp}-${random}`
}

/**
 * getSessionHistoryKey derives the stored history key for a session id.
 */
export function getSessionHistoryKey(sessionId: number): string {
  return `${StorageKeys.SESSION_HISTORY_PREFIX}${sessionId}`
}

/**
 * getChatDataKey derives the persisted chat payload key for a chat id and role.
 */
export function getChatDataKey(
  chatId: string,
  role: SupportedChatRole,
): string {
  return `${StorageKeys.CHAT_DATA_PREFIX}${role}_${chatId}`
}
