/**
 * deleted-chat-ids contains helpers for tracking and syncing deleted chat IDs.
 */

import { kvGet, kvSet, StorageKeys } from '@/utils/storage'

import { compareUuidV7, isUuidV7, uuidv7 } from './uuidv7'

export interface DeletedChatIdEntry {
  chat_id: string
  deleted_version: string
}

/**
 * normalizeDeletedChatIds coerces unknown persisted shapes into a canonical entry list.
 */
export function normalizeDeletedChatIds(input: unknown): DeletedChatIdEntry[] {
  if (!input) return []
  if (!Array.isArray(input)) return []

  const out: DeletedChatIdEntry[] = []
  for (const item of input) {
    if (typeof item === 'string') {
      out.push({ chat_id: item, deleted_version: '' })
      continue
    }

    if (!item || typeof item !== 'object') continue
    const obj = item as Partial<DeletedChatIdEntry>
    if (typeof obj.chat_id !== 'string' || !obj.chat_id) continue
    out.push({
      chat_id: obj.chat_id,
      deleted_version:
        typeof obj.deleted_version === 'string' ? obj.deleted_version : '',
    })
  }

  return out
}

/**
 * mergeDeletedChatIds merges two entry lists, keeping the newest deletion marker per chat id.
 */
export function mergeDeletedChatIds(
  a: DeletedChatIdEntry[],
  b: DeletedChatIdEntry[],
): DeletedChatIdEntry[] {
  const map = new Map<string, DeletedChatIdEntry>()

  const upsert = (entry: DeletedChatIdEntry) => {
    const existing = map.get(entry.chat_id)
    if (!existing) {
      map.set(entry.chat_id, entry)
      return
    }

    const ev = (existing.deleted_version || '').trim()
    const nv = (entry.deleted_version || '').trim()
    const eOk = isUuidV7(ev)
    const nOk = isUuidV7(nv)
    if (!eOk && nOk) {
      map.set(entry.chat_id, entry)
      return
    }
    if (eOk && !nOk) {
      return
    }

    const cmp = compareUuidV7(ev, nv)
    if (cmp < 0) map.set(entry.chat_id, entry)
  }

  for (const e of a) upsert(e)
  for (const e of b) upsert(e)

  return Array.from(map.values())
}

/**
 * trimDeletedChatIds keeps only the newest maxEntries entries.
 */
export function trimDeletedChatIds(
  entries: DeletedChatIdEntry[],
  maxEntries: number,
): DeletedChatIdEntry[] {
  if (entries.length <= maxEntries) return entries

  const sorted = [...entries].sort((x, y) => {
    const xv = (x.deleted_version || '').trim()
    const yv = (y.deleted_version || '').trim()
    const xOk = isUuidV7(xv)
    const yOk = isUuidV7(yv)
    if (xOk && !yOk) return 1
    if (!xOk && yOk) return -1

    const cmp = compareUuidV7(xv, yv)
    if (cmp !== 0) return cmp
    if (x.chat_id === y.chat_id) return 0
    return x.chat_id < y.chat_id ? -1 : 1
  })

  return sorted.slice(Math.max(0, sorted.length - maxEntries))
}

/**
 * buildDeletedChatIdSet extracts chat ids for fast membership checks.
 */
export function buildDeletedChatIdSet(
  entries: DeletedChatIdEntry[],
): Set<string> {
  return new Set(entries.map((e) => e.chat_id))
}

/**
 * recordDeletedChatId adds a deletion marker for a chat id to persistent storage.
 */
export async function recordDeletedChatId(chatId: string): Promise<void> {
  if (!chatId) return

  const local = normalizeDeletedChatIds(
    await kvGet<unknown>(StorageKeys.DELETED_CHAT_IDS),
  )
  const merged = mergeDeletedChatIds(local, [
    { chat_id: chatId, deleted_version: uuidv7() },
  ])
  const trimmed = trimDeletedChatIds(merged, 1000)
  await kvSet(StorageKeys.DELETED_CHAT_IDS, trimmed)
}
