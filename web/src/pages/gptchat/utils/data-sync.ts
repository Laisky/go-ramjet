/**
 * Utility functions for exporting and importing chat data.
 */
import { kvDel, kvGet, kvList, kvSet, StorageKeys } from '@/utils/storage'
import type { ChatMessageData, SessionHistoryItem } from '../types'
import { getChatDataKey } from './chat-storage'
import {
  buildDeletedChatIdSet,
  mergeDeletedChatIds,
  normalizeDeletedChatIds,
  trimDeletedChatIds,
  type DeletedChatIdEntry,
} from './deleted-chat-ids'
import { compareUuidV7, isUuidV7 } from './uuidv7'

/**
 * Export all data (sessions, configs, shortcuts) for sync
 */
export async function exportAllData(): Promise<Record<string, unknown>> {
  const keys = await kvList()
  const data: Record<string, unknown> = {}

  // keys to exclude
  const excludeKeys = ['MIGRATE_V1_COMPLETED']

  for (const key of keys) {
    if (excludeKeys.includes(key)) continue
    data[key] = await kvGet(key)
  }

  return data
}

/**
 * pickNewerMessage chooses the message to keep when local and cloud differ.
 *
 * Preference order:
 * 1) Non-empty edited_version (UUIDv7) wins; larger UUIDv7 is newer.
 * 2) Higher timestamp wins.
 * 3) If still tied/unknown, keep local.
 */
function pickNewerMessage(
  localMsg: ChatMessageData,
  cloudMsg: ChatMessageData,
) {
  const localVer = (localMsg.edited_version || '').trim()
  const cloudVer = (cloudMsg.edited_version || '').trim()

  if (localVer && cloudVer) {
    const cmp = compareUuidV7(localVer, cloudVer)
    if (cmp < 0) return cloudMsg
    if (cmp > 0) return localMsg
  } else if (!localVer && cloudVer) {
    return cloudMsg
  } else if (localVer && !cloudVer) {
    return localMsg
  }

  const localTs =
    typeof localMsg.timestamp === 'number' ? localMsg.timestamp : 0
  const cloudTs =
    typeof cloudMsg.timestamp === 'number' ? cloudMsg.timestamp : 0
  if (cloudTs > localTs) return cloudMsg
  if (localTs > cloudTs) return localMsg
  return localMsg
}

function parseChatDataKey(
  key: string,
): { role: 'user' | 'assistant'; chatId: string } | null {
  if (!key.startsWith(StorageKeys.CHAT_DATA_PREFIX)) return null
  const suffix = key.substring(StorageKeys.CHAT_DATA_PREFIX.length)
  const idx = suffix.indexOf('_')
  if (idx <= 0) return null
  const role = suffix.slice(0, idx)
  const chatId = suffix.slice(idx + 1)
  if (role !== 'user' && role !== 'assistant') return null
  if (!chatId) return null
  return { role, chatId }
}

async function applyDeletions(deletedChatIds: Set<string>): Promise<void> {
  if (deletedChatIds.size === 0) return

  for (const chatId of deletedChatIds) {
    await kvDel(getChatDataKey(chatId, 'user'))
    await kvDel(getChatDataKey(chatId, 'assistant'))
  }

  const keys = await kvList()
  const historyKeys = keys.filter((k) =>
    k.startsWith(StorageKeys.SESSION_HISTORY_PREFIX),
  )

  for (const historyKey of historyKeys) {
    const history = (await kvGet<SessionHistoryItem[]>(historyKey)) || []
    const filtered = history.filter((h) => !deletedChatIds.has(h.chatID))
    if (filtered.length !== history.length) {
      await kvSet(historyKey, filtered)
    }
  }
}

function buildHistoryItemFromMessage(msg: ChatMessageData): SessionHistoryItem {
  return {
    chatID: msg.chatID,
    role: msg.role as 'user' | 'assistant',
    content: String(msg.content || '').substring(0, 100),
    model: msg.model,
    timestamp: msg.timestamp,
  }
}

async function rebuildSessionHistory(
  sessionHistoryKey: string,
  chatIds: string[],
): Promise<void> {
  const items: SessionHistoryItem[] = []

  const chatMeta: {
    chatId: string
    ts: number
    user?: ChatMessageData
    assistant?: ChatMessageData
  }[] = []

  for (const chatId of chatIds) {
    const user = await kvGet<ChatMessageData>(getChatDataKey(chatId, 'user'))
    const assistant = await kvGet<ChatMessageData>(
      getChatDataKey(chatId, 'assistant'),
    )

    let ts = Math.max(
      typeof user?.timestamp === 'number' ? user.timestamp : 0,
      typeof assistant?.timestamp === 'number' ? assistant.timestamp : 0,
    )
    if (!ts) {
      ts = inferChatTimestampMs(chatId)
    }

    chatMeta.push({
      chatId,
      ts,
      user: user || undefined,
      assistant: assistant || undefined,
    })
  }

  chatMeta.sort((a, b) => {
    if (a.ts !== b.ts) return a.ts - b.ts
    if (a.chatId === b.chatId) return 0
    return a.chatId < b.chatId ? -1 : 1
  })

  for (const meta of chatMeta) {
    if (meta.user) items.push(buildHistoryItemFromMessage(meta.user))
    if (meta.assistant) items.push(buildHistoryItemFromMessage(meta.assistant))
  }

  await kvSet(sessionHistoryKey, items)
}

function inferChatTimestampMs(chatId: string): number {
  if (!chatId) return 0

  // Legacy format: chat-${timestamp}-${random}
  const legacy = /^chat-(\d+)-/.exec(chatId)
  if (legacy?.[1]) {
    const ts = Number(legacy[1])
    return Number.isFinite(ts) ? ts : 0
  }

  // v2 format: v2@{uuidv7}
  const v2 = /^v2@([0-9a-fA-F-]{36})$/.exec(chatId)
  if (v2?.[1] && isUuidV7(v2[1])) {
    const hex = v2[1].replaceAll('-', '').slice(0, 12)
    const ts = Number.parseInt(hex, 16)
    return Number.isFinite(ts) ? ts : 0
  }

  return 0
}

/**
 * Import data.
 *
 * @param mode 'merge' means incremental merge (default).
 *             'download' means overwrite all configs, and merge messages
 *             only for sessions that exist in both cloud and local.
 */
export async function importAllData(
  data: Record<string, unknown>,
  sessionId: number,
  mode: 'merge' | 'download' = 'merge',
): Promise<void> {
  const incoming = (data && typeof data === 'object' ? data : {}) as Record<
    string,
    unknown
  >

  const localKeys = await kvList()
  const localSessionConfigKeys = new Set(
    localKeys.filter((k) => k.startsWith(StorageKeys.SESSION_CONFIG_PREFIX)),
  )
  const cloudSessionConfigKeys = new Set(
    Object.keys(incoming).filter((k) =>
      k.startsWith(StorageKeys.SESSION_CONFIG_PREFIX),
    ),
  )

  // 1) Merge deleted ids and apply deletions BEFORE merging messages.
  const localDeleted = normalizeDeletedChatIds(
    await kvGet<unknown>(StorageKeys.DELETED_CHAT_IDS),
  )
  const cloudDeleted = normalizeDeletedChatIds(
    incoming[StorageKeys.DELETED_CHAT_IDS],
  )
  const mergedDeletedFull: DeletedChatIdEntry[] = mergeDeletedChatIds(
    localDeleted,
    cloudDeleted,
  )
  const deletedSet = buildDeletedChatIdSet(mergedDeletedFull)

  await applyDeletions(deletedSet)

  const trimmedDeleted = trimDeletedChatIds(mergedDeletedFull, 1000)
  await kvSet(StorageKeys.DELETED_CHAT_IDS, trimmedDeleted)

  // 2) Merge chat payloads.
  for (const [key, val] of Object.entries(incoming)) {
    const parsed = parseChatDataKey(key)
    if (!parsed) continue
    if (deletedSet.has(parsed.chatId)) continue
    if (!val || typeof val !== 'object') continue

    const cloudMsg = val as ChatMessageData
    const normalizedCloud: ChatMessageData = {
      ...cloudMsg,
      chatID: cloudMsg.chatID || parsed.chatId,
      role: (cloudMsg.role as any) || parsed.role,
    }

    const localMsg = await kvGet<ChatMessageData>(key)
    if (!localMsg || typeof localMsg !== 'object') {
      await kvSet(key, normalizedCloud)
      continue
    }

    const chosen = pickNewerMessage(localMsg, normalizedCloud)
    if (chosen === normalizedCloud) {
      await kvSet(key, chosen)
    }
  }

  // 3) Rebuild session histories using merged chat payloads.
  const historyKeys = new Set<string>()

  for (const k of localKeys) {
    if (k.startsWith(StorageKeys.SESSION_HISTORY_PREFIX)) historyKeys.add(k)
  }
  for (const k of Object.keys(incoming)) {
    if (k.startsWith(StorageKeys.SESSION_HISTORY_PREFIX)) historyKeys.add(k)
  }

  for (const historyKey of historyKeys) {
    // Extract session ID to check matching
    const sessIdSuffix = historyKey.substring(
      StorageKeys.SESSION_HISTORY_PREFIX.length,
    )
    const configKey = StorageKeys.SESSION_CONFIG_PREFIX + sessIdSuffix

    const localHistory =
      (await kvGet<SessionHistoryItem[]>(historyKey)) ||
      ([] as SessionHistoryItem[])
    const cloudHistory = normalizeHistoryList(incoming[historyKey])

    if (mode === 'download') {
      const isMatched =
        localSessionConfigKeys.has(configKey) &&
        cloudSessionConfigKeys.has(configKey)

      if (isMatched) {
        // Match -> attempt to merge
        const chatIds = new Set<string>()
        for (const item of localHistory) {
          if (item?.chatID) chatIds.add(item.chatID)
        }
        for (const item of cloudHistory) {
          if (item?.chatID) chatIds.add(item.chatID)
        }
        for (const id of deletedSet) chatIds.delete(id)
        await rebuildSessionHistory(historyKey, Array.from(chatIds))
      } else {
        // Not matched -> do nothing for history in download mode
        continue
      }
    } else {
      // mode === 'merge' (default behavior)
      const chatIds = new Set<string>()
      for (const item of localHistory) {
        if (item?.chatID) chatIds.add(item.chatID)
      }
      for (const item of cloudHistory) {
        if (item?.chatID) chatIds.add(item.chatID)
      }
      for (const id of deletedSet) chatIds.delete(id)
      await rebuildSessionHistory(historyKey, Array.from(chatIds))
    }
  }

  // 4) Overwrite non-chat keys from cloud (settings, session configs, shortcuts, etc).
  //    Do this last so chat merges don't get clobbered.
  for (const [key, val] of Object.entries(incoming)) {
    if (key === StorageKeys.DELETED_CHAT_IDS) continue
    if (key.startsWith(StorageKeys.CHAT_DATA_PREFIX)) continue
    if (key.startsWith(StorageKeys.SESSION_HISTORY_PREFIX)) continue
    if (key === StorageKeys.SELECTED_SESSION) continue

    if (mode === 'download') {
      // Download mode: unconditional overwrite
      await kvSet(key, val)
    } else {
      // Merge mode: conditional overwrite based on updated_at
      const localVal = await kvGet<any>(key)
      if (
        localVal &&
        typeof localVal === 'object' &&
        val &&
        typeof val === 'object'
      ) {
        const localTs = localVal.updated_at || 0
        const cloudTs = (val as any).updated_at || 0
        if (cloudTs >= localTs) {
          await kvSet(key, val)
        }
      } else {
        await kvSet(key, val)
      }
    }
  }

  // No automatic reload here; callers can choose when to refresh the UI.

  // Keep the active session stable on this device.
  await kvSet(StorageKeys.SELECTED_SESSION, sessionId)
}

function normalizeHistoryList(input: unknown): SessionHistoryItem[] {
  if (!Array.isArray(input)) return []
  const out: SessionHistoryItem[] = []
  for (const item of input) {
    if (!item || typeof item !== 'object') continue
    const obj = item as Partial<SessionHistoryItem>
    if (typeof obj.chatID !== 'string' || !obj.chatID) continue
    if (obj.role !== 'user' && obj.role !== 'assistant') continue
    out.push({
      chatID: obj.chatID,
      role: obj.role,
      content: typeof obj.content === 'string' ? obj.content : '',
      model: typeof obj.model === 'string' ? obj.model : undefined,
      timestamp: typeof obj.timestamp === 'number' ? obj.timestamp : undefined,
    })
  }
  return out
}
