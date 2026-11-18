/**
 * Copying a stored GPTChat session (duplicate and fork): its config, its
 * history, and the chat payloads the history points at.
 */
import { kvGet, kvList, kvSet, StorageKeys } from '@/utils/storage'
import {
  DefaultSessionConfig,
  type ChatMessageData,
  type SessionConfig,
  type SessionHistoryItem,
} from '../types'
import {
  generateChatId,
  getChatDataKey,
  getSessionHistoryKey,
} from '../utils/chat-storage'
import {
  DEFAULT_SESSION_ID,
  getSessionConfigKey,
} from '../utils/config-helpers'
import { getSessionOrderFromKv, setSessionOrderToKv } from './session-order'

/**
 * SessionCopyOptions configures copyStoredSession. nameSuffix is appended
 * to the source session name ("Copy", "Fork"). upTo, when set, ends the
 * copied history after the message with that chat id and role.
 */
export interface SessionCopyOptions {
  nameSuffix: string
  upTo?: { chatId: string; role: 'user' | 'assistant' }
}

/**
 * nextSessionId returns the id after the highest stored session id, or
 * DEFAULT_SESSION_ID when no session is stored.
 */
async function nextSessionId(): Promise<number> {
  const ids = (await kvList())
    .filter((k) => k.startsWith(StorageKeys.SESSION_CONFIG_PREFIX))
    .map((k) => parseInt(k.slice(StorageKeys.SESSION_CONFIG_PREFIX.length), 10))
    .filter((id) => !Number.isNaN(id))
  return ids.length > 0 ? Math.max(...ids) + 1 : DEFAULT_SESSION_ID
}

/**
 * copyStoredSession copies session sourceSessionId into a new session: the
 * config (renamed with options.nameSuffix and bound to the browser-wide
 * sync key), the history up to options.upTo, and every chat payload under
 * fresh chat ids. The new session is appended to the session order.
 * Callers must let pending writes of the source land first.
 * It returns the new session id.
 */
export async function copyStoredSession(
  sourceSessionId: number,
  options: SessionCopyOptions,
): Promise<number> {
  const newSessionId = await nextSessionId()
  const sourceConfig =
    (await kvGet<SessionConfig>(getSessionConfigKey(sourceSessionId))) ||
    DefaultSessionConfig
  const globalSyncKey = await kvGet<string>(StorageKeys.SYNC_KEY)
  const copiedConfig: SessionConfig = {
    ...sourceConfig,
    session_name: `${sourceConfig.session_name || `Chat Session ${sourceSessionId}`} ${options.nameSuffix}`,
    updated_at: Date.now(),
    sync_key: globalSyncKey || sourceConfig.sync_key,
  }
  await kvSet(getSessionConfigKey(newSessionId), copiedConfig)

  const sourceHistory =
    (await kvGet<SessionHistoryItem[]>(
      getSessionHistoryKey(sourceSessionId),
    )) || []
  const idMap = new Map<string, string>()
  const copiedHistory: SessionHistoryItem[] = []
  for (const item of sourceHistory) {
    if (!item.chatID) continue

    let nextChatId = idMap.get(item.chatID)
    if (!nextChatId) {
      nextChatId = generateChatId()
      idMap.set(item.chatID, nextChatId)
    }
    copiedHistory.push({ ...item, chatID: nextChatId })

    const chatData = await kvGet<ChatMessageData>(
      getChatDataKey(item.chatID, item.role),
    )
    if (chatData) {
      await kvSet(getChatDataKey(nextChatId, item.role), {
        ...chatData,
        chatID: nextChatId,
      })
    }

    if (
      options.upTo &&
      item.chatID === options.upTo.chatId &&
      item.role === options.upTo.role
    ) {
      break
    }
  }
  await kvSet(getSessionHistoryKey(newSessionId), copiedHistory)

  const sessionOrder = await getSessionOrderFromKv()
  if (!sessionOrder.includes(newSessionId)) {
    sessionOrder.push(newSessionId)
    await setSessionOrderToKv(sessionOrder)
  }
  return newSessionId
}
