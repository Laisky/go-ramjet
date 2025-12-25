/**
 * PouchDB-based key-value storage for chat data.
 * Maintains compatibility with legacy chat.js storage keys.
 */
import PouchDB from 'pouchdb-browser'

// Storage key constants - must match legacy keys from chat.js
export const StorageKeys = {
  PINNED_MATERIALS: 'config_api_pinned_materials',
  ALLOWED_MODELS: 'config_chat_models',
  CUSTOM_DATASET_PASSWORD: 'config_chat_dataset_key',
  PROMPT_SHORTCUTS: 'config_prompt_shortcuts',
  SESSION_HISTORY_PREFIX: 'chat_user_session_',
  SESSION_CONFIG_PREFIX: 'chat_user_config_',
  SELECTED_SESSION: 'config_selected_session',
  SYNC_KEY: 'config_sync_key',
  VERSION_DATE: 'config_version_date',
  USER_INFO: 'config_user_info',
  SESSION_DRAFTS: 'chat_session_drafts',
  CHAT_DATA_PREFIX: 'chat_data_', // ${prefix}${role}_${chatID}
} as const

export type KvOperation = 'set' | 'del'

type KvListener = (
  key: string,
  op: KvOperation,
  oldVal: unknown,
  newVal: unknown,
) => void

interface KvListenerEntry {
  name?: string
  callback: KvListener
}

// Singleton database instance
let db: PouchDB.Database | null = null
let dbInitializing = false
let dbInitialized = false

// Listener registry
const listeners: Map<string, KvListenerEntry[]> = new Map()

/**
 * Sleep for a specified duration
 */
function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms))
}

/**
 * Execute a database operation with retry logic
 */
async function executeWithRetry<T>(
  operation: () => Promise<T>,
  maxRetries = 3,
): Promise<T> {
  for (let attempt = 0; attempt < maxRetries; attempt++) {
    try {
      return await operation()
    } catch (err: unknown) {
      const error = err as { name?: string }
      if (error.name === 'InvalidStateError' && attempt < maxRetries - 1) {
        console.warn('Database connection closing, retrying operation...')
        await sleep(300)
        dbInitialized = false
        await initDb()
      } else {
        throw err
      }
    }
  }
  throw new Error('Max retries exceeded')
}

/**
 * Initialize the PouchDB database connection
 */
async function initDb(): Promise<PouchDB.Database> {
  if (dbInitialized && db) {
    return db
  }

  if (dbInitializing) {
    // Wait for initialization to complete
    return new Promise((resolve) => {
      const checkInterval = setInterval(() => {
        if (dbInitialized && db) {
          clearInterval(checkInterval)
          resolve(db)
        }
      }, 100)
    })
  }

  dbInitializing = true

  try {
    db = new PouchDB('mydatabase')
    dbInitialized = true
    return db
  } finally {
    dbInitializing = false
  }
}

/**
 * Add a listener for a key prefix
 */
export async function kvAddListener(
  keyPrefix: string,
  callback: KvListener,
  callbackName?: string,
): Promise<void> {
  await initDb()

  if (!listeners.has(keyPrefix)) {
    listeners.set(keyPrefix, [])
  }

  const prefixListeners = listeners.get(keyPrefix)!

  if (callbackName) {
    // Check if a listener with this name already exists
    const existingIndex = prefixListeners.findIndex(
      (l) => l.name === callbackName,
    )
    if (existingIndex >= 0) {
      prefixListeners[existingIndex].callback = callback
    } else {
      prefixListeners.push({ name: callbackName, callback })
    }
  } else {
    // Check if callback already exists
    const exists = prefixListeners.some((l) => l.callback === callback)
    if (!exists) {
      prefixListeners.push({ callback })
    }
  }
}

/**
 * Remove a listener by callback name
 */
export function kvRemoveListener(
  keyPrefix: string,
  callbackName: string,
): void {
  const prefixListeners = listeners.get(keyPrefix)
  if (!prefixListeners) return

  const index = prefixListeners.findIndex((l) => l.name === callbackName)
  if (index >= 0) {
    prefixListeners.splice(index, 1)
  }
}

/**
 * Notify all listeners for a key
 */
function notifyListeners(
  key: string,
  op: KvOperation,
  oldVal: unknown,
  newVal: unknown,
): void {
  listeners.forEach((prefixListeners, keyPrefix) => {
    if (key.startsWith(keyPrefix)) {
      prefixListeners.forEach((entry) => {
        try {
          entry.callback(key, op, oldVal, newVal)
        } catch (err) {
          console.error('Listener error:', err)
        }
      })
    }
  })
}

/**
 * Set a value in the database
 */
export async function kvSet<T>(key: string, val: T): Promise<void> {
  await initDb()
  console.debug(`kvSet: ${key}`)

  const marshaledVal = JSON.stringify(val)
  let oldVal: unknown = null

  try {
    await executeWithRetry(async () => {
      let oldDoc: PouchDB.Core.ExistingDocument<{ val: string }> | null = null

      try {
        oldDoc = await db!.get<{ val: string }>(key)
        oldVal = oldDoc ? JSON.parse(oldDoc.val) : null
      } catch (error: unknown) {
        const pouchError = error as { status?: number }
        if (pouchError.status !== 404) {
          throw error
        }
      }

      await db!.put({
        _id: key,
        _rev: oldDoc?._rev,
        val: marshaledVal,
      })
    })
  } catch (error: unknown) {
    const pouchError = error as { status?: number }
    if (pouchError.status === 409) {
      console.warn(`Conflict detected for key ${key}, ignoring`)
      return
    }
    console.error(`kvSet for key ${key} failed:`, error)
    throw error
  }

  notifyListeners(key, 'set', oldVal, val)
}

/**
 * Get a value from the database
 */
export async function kvGet<T>(key: string): Promise<T | null> {
  await initDb()
  console.debug(`kvGet: ${key}`)

  return executeWithRetry(async () => {
    try {
      const doc = await db!.get<{ val: string }>(key)
      if (!doc || !doc.val) {
        return null
      }
      return JSON.parse(doc.val) as T
    } catch (error: unknown) {
      const pouchError = error as { status?: number }
      if (pouchError.status === 404) {
        return null
      }
      throw error
    }
  })
}

/**
 * Check if a key exists
 */
export async function kvExists(key: string): Promise<boolean> {
  await initDb()
  console.debug(`kvExists: ${key}`)

  return executeWithRetry(async () => {
    try {
      await db!.get(key)
      return true
    } catch (error: unknown) {
      const pouchError = error as { status?: number }
      if (pouchError.status === 404) {
        return false
      }
      throw error
    }
  })
}

/**
 * Rename a key
 */
export async function kvRename(oldKey: string, newKey: string): Promise<void> {
  await initDb()
  console.debug(`kvRename: ${oldKey} -> ${newKey}`)

  const oldVal = await kvGet(oldKey)
  if (oldVal === null) {
    return
  }

  await kvSet(newKey, oldVal)
  await kvDel(oldKey)
}

/**
 * Delete a key from the database
 */
export async function kvDel(key: string): Promise<void> {
  await initDb()
  console.debug(`kvDel: ${key}`)

  return executeWithRetry(async () => {
    let oldVal: unknown = null

    try {
      const doc = await db!.get<{ val: string }>(key)
      oldVal = JSON.parse(doc.val)
      await db!.remove(doc)

      notifyListeners(key, 'del', oldVal, null)
    } catch (error: unknown) {
      const pouchError = error as { status?: number }
      if (pouchError.status !== 404) {
        throw error
      }
    }
  })
}

/**
 * List all keys in the database
 */
export async function kvList(): Promise<string[]> {
  await initDb()
  console.debug('kvList')

  const docs = await db!.allDocs({ include_docs: true })
  return docs.rows.map((row) => row.id)
}

/**
 * Clear all data from the database
 */
export async function kvClear(): Promise<void> {
  if (!dbInitialized || !db) return

  console.debug('kvClear')
  dbInitialized = false

  try {
    const keys = await kvList()

    // Notify listeners before destroying
    for (const key of keys) {
      try {
        const oldVal = await kvGet(key)
        notifyListeners(key, 'del', oldVal, null)
      } catch (error) {
        console.warn(`Failed to notify listeners for key ${key}:`, error)
      }
    }

    await db.destroy()
    db = null

    await sleep(500)
    await initDb()
  } finally {
    if (!dbInitialized) {
      await initDb()
    }
  }
}
