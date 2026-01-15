import { kvDel, kvGet, kvList, kvSet, StorageKeys } from '@/utils/storage'
import type { SessionHistoryItem } from '../types'

const MIGRATION_FLAG_KEY = 'MIGRATE_V1_COMPLETED'

export async function migrateLegacyData() {
  try {
    const isMigrated = await kvGet<boolean>(MIGRATION_FLAG_KEY)

    if (!isMigrated) {
      console.groupCollapsed('Migrating Legacy Data...')

      // List of prefixes/keys to migrate
      const keysToMigrate: string[] = [
        StorageKeys.PINNED_MATERIALS,
        StorageKeys.ALLOWED_MODELS,
        StorageKeys.CUSTOM_DATASET_PASSWORD,
        StorageKeys.PROMPT_SHORTCUTS,
        StorageKeys.SELECTED_SESSION,
        StorageKeys.SYNC_KEY,
        StorageKeys.VERSION_DATE,
        StorageKeys.USER_INFO,
      ]

      const prefixesToMigrate = [
        StorageKeys.SESSION_HISTORY_PREFIX,
        StorageKeys.SESSION_CONFIG_PREFIX,
        StorageKeys.CHAT_DATA_PREFIX,
      ]

      // Iterate over localStorage
      for (let i = 0; i < localStorage.length; i++) {
        const key = localStorage.key(i)
        if (!key) continue

        let shouldMigrate = false
        if (keysToMigrate.includes(key)) {
          shouldMigrate = true
        } else {
          for (const prefix of prefixesToMigrate) {
            if (key.startsWith(prefix)) {
              shouldMigrate = true
              break
            }
          }
        }

        if (shouldMigrate) {
          try {
            const rawVal = localStorage.getItem(key)
            if (rawVal) {
              const existing = await kvGet(key)
              if (!existing) {
                const parsed = JSON.parse(rawVal)
                await kvSet(key, parsed)
                console.debug(
                  `[migration] Migrated key from localStorage: ${key}`,
                )
              } else {
                console.debug(
                  `[migration] Skip migrating key ${key}, already exists in KV`,
                )
              }
            }
          } catch (err) {
            console.error(`[migration] Failed to migrate key ${key}:`, err)
          }
        }
      }

      await kvSet(MIGRATION_FLAG_KEY, true)
      console.log('Legacy migration completed.')
      console.groupEnd()
    } else {
      console.debug('Legacy migration already completed.')
    }
  } catch (error) {
    console.error('Migration failed:', error)
  } finally {
    try {
      await cleanupOrphanChatData()
    } catch (cleanupErr) {
      console.warn('Failed to clean orphan chat data:', cleanupErr)
    }
  }
}

export async function cleanupOrphanChatData(): Promise<void> {
  const allKeys = await kvList()
  const historyKeys = allKeys.filter((key) =>
    key.startsWith(StorageKeys.SESSION_HISTORY_PREFIX),
  )
  const chatDataKeys = allKeys.filter((key) =>
    key.startsWith(StorageKeys.CHAT_DATA_PREFIX),
  )

  if (historyKeys.length === 0 || chatDataKeys.length === 0) {
    return
  }

  const validChatIds = new Set<string>()

  for (const historyKey of historyKeys) {
    try {
      const history = (await kvGet<SessionHistoryItem[]>(historyKey)) || []
      history.forEach((item) => {
        if (item?.chatID) {
          validChatIds.add(item.chatID)
        }
      })
    } catch (err) {
      console.warn(`Failed to read history for ${historyKey}:`, err)
    }
  }

  if (validChatIds.size === 0) {
    // Nothing references chat data, so skip destructive cleanup.
    return
  }

  for (const dataKey of chatDataKeys) {
    const suffix = dataKey.substring(StorageKeys.CHAT_DATA_PREFIX.length)
    const parts = suffix.split('_')
    if (parts.length < 2) {
      continue
    }
    const chatId = parts.slice(1).join('_')
    if (!validChatIds.has(chatId)) {
      try {
        await kvDel(dataKey)
        console.debug(`Removed orphan chat data: ${dataKey}`)
      } catch (err) {
        console.warn(`Failed to delete orphan chat data ${dataKey}:`, err)
      }
    }
  }
}
