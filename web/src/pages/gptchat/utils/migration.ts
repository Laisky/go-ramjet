import { kvGet, kvSet, StorageKeys } from '@/utils/storage'

const MIGRATION_FLAG_KEY = 'MIGRATE_V1_COMPLETED'

export async function migrateLegacyData() {
  try {
    const isMigrated = await kvGet<boolean>(MIGRATION_FLAG_KEY)
    if (isMigrated) {
      console.debug('Legacy migration already completed.')
      return
    }

    console.groupCollapsed('Migrating Legacy Data...')

    // List of prefixes/keys to migrate
    const keysToMigrate = [
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
        if (keysToMigrate.includes(key as any)) {
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
                    // Check if already exists in PouchDB to avoid overwriting new data
                    // (Though on first run, PouchDB should be empty or strictly newer)
                    // We'll trust legacy data if PouchDB is empty for this key.
                    const existing = await kvGet(key)
                    if (!existing) {
                        const parsed = JSON.parse(rawVal)
                        await kvSet(key, parsed)
                        console.debug(`Migrated key: ${key}`)
                    }
                }
            } catch (err) {
                console.error(`Failed to migrate key ${key}:`, err)
            }
        }
    }

    await kvSet(MIGRATION_FLAG_KEY, true)
    console.log('Legacy migration completed.')
    console.groupEnd()

  } catch (error) {
    console.error('Migration failed:', error)
  }
}
