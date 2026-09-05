/**
 * Session-explicit message persistence.
 *
 * Every other writer in the chat closes over the *active* session id, which is
 * correct for text chat but wrong for a live voice call: the call is pinned to
 * the session it started in and must keep writing there even after the user
 * switches to another session. These are plain functions taking the target
 * session id, so a pinned writer can reach its own session.
 *
 * Writes are serialized per session. A message blob and the session's history
 * index are two separate keys, and the history update is a read-modify-write;
 * without a queue two concurrent commits interleave and one history entry is
 * lost, leaving a stored message that nothing indexes and nothing renders.
 */
import { kvGet, kvSet } from '@/utils/storage'

import type { ChatMessageData, SessionHistoryItem } from '../types'
import { getChatDataKey, getSessionHistoryKey } from '../utils/chat-storage'
import { uuidv7 } from '../utils/uuidv7'

/** sessionWriteQueues serializes history read-modify-writes per session. */
const sessionWriteQueues = new Map<number, Promise<void>>()

/** toSessionHistoryItem creates the compact history entry used to order persisted messages. */
function toSessionHistoryItem(message: ChatMessageData): SessionHistoryItem {
  return {
    chatID: message.chatID,
    role: message.role as 'user' | 'assistant',
    content: message.content.substring(0, 100),
    model: message.model,
    timestamp: message.timestamp,
  }
}

/** runSerialized queues work behind any in-flight write for the same session. */
function runSerialized(
  sessionId: number,
  work: () => Promise<void>,
): Promise<void> {
  const previous = sessionWriteQueues.get(sessionId) ?? Promise.resolve()
  // Swallow the predecessor's rejection so one failed write cannot poison the queue.
  const next = previous.catch(() => {}).then(work)
  sessionWriteQueues.set(
    sessionId,
    next.catch(() => {}),
  )
  return next
}

/**
 * appendMessageToSession stores one message and indexes it in the given session.
 *
 * Messages are keyed by (chatID, role), so re-saving the same pair updates it in
 * place rather than creating a second entry. That is what lets a streaming turn
 * be committed repeatedly under a stable chat id.
 */
export function appendMessageToSession(
  sessionId: number,
  message: ChatMessageData,
): Promise<void> {
  return runSerialized(sessionId, async () => {
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
      (item) => item.chatID === message.chatID && item.role === message.role,
    )
    const historyItem = toSessionHistoryItem(toSave)
    if (existingIndex >= 0) history[existingIndex] = historyItem
    else history.push(historyItem)

    await kvSet(historyKey, history)
  })
}
