/**
 * Native IndexedDB key-value storage backed by the `idb` v8 library.
 *
 * - Database name: `ramjet_kv_v2`, version 1.
 * - Two object stores: `kv` (user data, external keys) and `meta` (internal sentinels).
 * - Values are stored via structured clone (no JSON.stringify).
 */
import { openDB, type IDBPDatabase } from 'idb'

const DB_NAME = 'ramjet_kv_v2'
const DB_VERSION = 1
const KV_STORE = 'kv'
const META_STORE = 'meta'

export type KvOperation = 'set' | 'del'
export type KvListener = (
  key: string,
  op: KvOperation,
  oldVal: unknown,
  newVal: unknown,
) => void

interface KvListenerEntry {
  name?: string
  callback: KvListener
}

interface RamjetKvSchema {
  kv: { key: string; value: unknown }
  meta: { key: string; value: unknown }
}

// Singleton database handle
let dbPromise: Promise<IDBPDatabase<RamjetKvSchema>> | null = null
let persistRequested = false

// Listener registry — keyed by prefix
const listeners: Map<string, KvListenerEntry[]> = new Map()

/**
 * Sleep helper.
 */
function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms))
}

/**
 * Fire-and-forget navigator.storage.persist() — ignore failures.
 */
function requestPersistOnce(): void {
  if (persistRequested) return
  persistRequested = true
  try {
    if (
      typeof navigator !== 'undefined' &&
      navigator.storage &&
      typeof navigator.storage.persist === 'function'
    ) {
      void navigator.storage
        .persist()
        .catch((err) =>
          console.debug('navigator.storage.persist() rejected', err),
        )
    }
  } catch (err) {
    console.debug('navigator.storage.persist() threw', err)
  }
}

/**
 * Open the DB (lazy singleton). Handles blocking/terminated events by
 * closing and nulling the cached handle.
 */
async function openHandle(): Promise<IDBPDatabase<RamjetKvSchema>> {
  const handle = await openDB<RamjetKvSchema>(DB_NAME, DB_VERSION, {
    upgrade(db) {
      if (!db.objectStoreNames.contains(KV_STORE)) {
        db.createObjectStore(KV_STORE)
      }
      if (!db.objectStoreNames.contains(META_STORE)) {
        db.createObjectStore(META_STORE)
      }
    },
    blocking() {
      // Another tab is requesting an upgrade — close and drop our handle.
      try {
        handle.close()
      } catch (err) {
        console.debug('Error closing DB on blocking', err)
      }
      dbPromise = null
    },
    terminated() {
      dbPromise = null
    },
  })
  requestPersistOnce()
  return handle
}

/**
 * Get (or create) the shared DB handle.
 */
async function getDB(): Promise<IDBPDatabase<RamjetKvSchema>> {
  if (!dbPromise) {
    dbPromise = openHandle().catch((err) => {
      dbPromise = null
      throw err
    })
  }
  return dbPromise
}

/**
 * Decide whether an IndexedDB error is worth retrying by reopening the handle.
 *
 * - `InvalidStateError` — classic Safari/iOS idle-handle death.
 * - `UnknownError` with message `"Connection to Indexed Database server lost"` —
 *   Safari/iOS 17.4+ raises this during idle handle death instead of the
 *   older `InvalidStateError`.
 * - `TransactionInactiveError` — a related iOS quirk class; cheap to include.
 */
function isRetryableError(err: unknown): boolean {
  const name = (err as { name?: string } | null)?.name
  if (name === 'InvalidStateError') return true
  if (name === 'TransactionInactiveError') return true
  if (name === 'UnknownError') {
    const message = (err as { message?: unknown } | null)?.message
    if (
      typeof message === 'string' &&
      message.includes('Connection to Indexed Database server lost')
    ) {
      return true
    }
  }
  return false
}

/**
 * Execute an op with retry on transient IndexedDB handle errors. Safari/iOS
 * sometimes closes idle IndexedDB handles; we close + reopen and try again.
 * See `isRetryableError` for the exact predicate.
 */
async function executeWithRetry<T>(
  operation: () => Promise<T>,
  maxRetries = 3,
): Promise<T> {
  let lastErr: unknown
  for (let attempt = 0; attempt < maxRetries; attempt++) {
    try {
      return await operation()
    } catch (err: unknown) {
      lastErr = err
      const name = (err as { name?: string } | null)?.name
      if (name === 'QuotaExceededError') {
        const message =
          (err as { message?: string })?.message ?? 'quota exceeded'
        throw new Error(`Storage quota exceeded: ${message}`)
      }
      if (isRetryableError(err) && attempt < maxRetries - 1) {
        console.warn(
          `IDB retryable error ${name ?? 'unknown'} (attempt ${attempt + 1}), retrying...`,
        )
        try {
          const db = await dbPromise
          db?.close()
        } catch {
          // ignore
        }
        dbPromise = null
        await sleep(300)
        continue
      }
      throw err
    }
  }
  throw lastErr ?? new Error('Max retries exceeded')
}

/**
 * Notify listeners for `key`. Each listener is wrapped in try/catch so that a
 * throwing listener does not interfere with siblings or the caller.
 */
function notifyListeners(
  key: string,
  op: KvOperation,
  oldVal: unknown,
  newVal: unknown,
): void {
  listeners.forEach((prefixListeners, prefix) => {
    if (!key.startsWith(prefix)) return
    for (const entry of prefixListeners) {
      try {
        entry.callback(key, op, oldVal, newVal)
      } catch (err) {
        console.error('kv listener error', err)
      }
    }
  })
}

/**
 * Get a value from the kv store.
 *
 * @returns the stored value, or `null` if the key is missing OR if `undefined` was stored.
 * Callers that need to distinguish "missing" from "stored null" should use `kvExists`.
 * Note: storing `undefined` is discouraged — it is indistinguishable from a missing key via kvGet.
 */
export async function kvGet<T>(key: string): Promise<T | null> {
  return executeWithRetry(async () => {
    const db = await getDB()
    const tx = db.transaction(KV_STORE, 'readonly')
    const store = tx.objectStore(KV_STORE)
    const [val] = await Promise.all([store.get(key), tx.done])
    if (val === undefined) return null
    return val as T | null
  })
}

/**
 * Set a value. Fires 'set' listener after the transaction commits.
 */
export async function kvSet<T>(key: string, val: T): Promise<void> {
  let oldVal: unknown = null
  await executeWithRetry(async () => {
    const db = await getDB()
    const tx = db.transaction(KV_STORE, 'readwrite')
    const store = tx.objectStore(KV_STORE)
    const getPromise = store.get(key)
    // Queue the put without awaiting get separately — both share the tx.
    const putPromise = store.put(val as unknown, key)
    const [prev] = await Promise.all([getPromise, putPromise, tx.done])
    oldVal = prev === undefined ? null : prev
  })
  notifyListeners(key, 'set', oldVal, val)
}

/**
 * Delete a key. No-op if missing (no listener fired).
 */
export async function kvDel(key: string): Promise<void> {
  let existed = false
  let oldVal: unknown = null
  await executeWithRetry(async () => {
    const db = await getDB()
    const tx = db.transaction(KV_STORE, 'readwrite')
    const store = tx.objectStore(KV_STORE)
    const getKeyPromise = store.getKey(key)
    const getPromise = store.get(key)
    const delPromise = store.delete(key)
    const [foundKey, prev] = await Promise.all([
      getKeyPromise,
      getPromise,
      delPromise,
      tx.done,
    ])
    if (foundKey !== undefined) {
      existed = true
      oldVal = prev === undefined ? null : prev
    }
  })
  if (existed) {
    notifyListeners(key, 'del', oldVal, null)
  }
}

/**
 * Check if a key exists (true even if stored value is null/undefined).
 */
export async function kvExists(key: string): Promise<boolean> {
  return executeWithRetry(async () => {
    const db = await getDB()
    const tx = db.transaction(KV_STORE, 'readonly')
    const store = tx.objectStore(KV_STORE)
    const [foundKey] = await Promise.all([store.getKey(key), tx.done])
    return foundKey !== undefined
  })
}

/**
 * Rename oldKey -> newKey. No-op if oldKey missing.
 * Fires 'set' for newKey then 'del' for oldKey.
 */
export async function kvRename(oldKey: string, newKey: string): Promise<void> {
  let didMove = false
  let movedVal: unknown = null
  let newKeyOldVal: unknown = null
  await executeWithRetry(async () => {
    const db = await getDB()
    const tx = db.transaction(KV_STORE, 'readwrite')
    const store = tx.objectStore(KV_STORE)
    const getOldKeyPromise = store.getKey(oldKey)
    const getOldValPromise = store.get(oldKey)
    const getNewValPromise = store.get(newKey)
    const [oldKeyPresent, oldVal, prevNewVal] = await Promise.all([
      getOldKeyPromise,
      getOldValPromise,
      getNewValPromise,
    ])
    if (oldKeyPresent === undefined) {
      // commit empty tx
      await tx.done
      return
    }
    didMove = true
    movedVal = oldVal === undefined ? null : oldVal
    newKeyOldVal = prevNewVal === undefined ? null : prevNewVal
    const putPromise = store.put(oldVal as unknown, newKey)
    const delPromise = store.delete(oldKey)
    await Promise.all([putPromise, delPromise, tx.done])
  })
  if (didMove) {
    notifyListeners(newKey, 'set', newKeyOldVal, movedVal)
    notifyListeners(oldKey, 'del', movedVal, null)
  }
}

/**
 * List all keys in the kv store (not meta).
 */
export async function kvList(): Promise<string[]> {
  return executeWithRetry(async () => {
    const db = await getDB()
    const tx = db.transaction(KV_STORE, 'readonly')
    const store = tx.objectStore(KV_STORE)
    const [keys] = await Promise.all([store.getAllKeys(), tx.done])
    return keys as string[]
  })
}

/**
 * Clear kv store. Snapshots keys+values and issues the clear inside a single
 * readwrite transaction so no concurrent kvSet can slip in between the
 * snapshot and the wipe (would otherwise get its 'set' notification skipped
 * or observe a stale oldVal). Listeners are notified after the transaction
 * commits. Meta store is preserved.
 */
export async function kvClear(): Promise<void> {
  const snapshot: Array<[string, unknown]> = []
  await executeWithRetry(async () => {
    // Reset on retry — a previous attempt may have partially populated it.
    snapshot.length = 0
    const db = await getDB()
    const tx = db.transaction(KV_STORE, 'readwrite')
    const store = tx.objectStore(KV_STORE)
    // getAllKeys() + getAll() share the same ordering per IDB spec.
    const keys = (await store.getAllKeys()) as string[]
    const values = await store.getAll()
    for (let i = 0; i < keys.length; i++) {
      snapshot.push([String(keys[i]), values[i]])
    }
    await store.clear()
    await tx.done
  })
  // Notify only after commit.
  for (const [key, oldVal] of snapshot) {
    notifyListeners(key, 'del', oldVal, null)
  }
}

/**
 * Register a prefix listener. If `callbackName` is provided and already
 * registered on this prefix, the existing callback is replaced. Without a
 * name, listeners are de-duped by function identity.
 */
export async function kvAddListener(
  keyPrefix: string,
  callback: KvListener,
  callbackName?: string,
): Promise<void> {
  // Ensure DB is at least opened — matches the old contract.
  await getDB().catch(() => {
    // Swallow — adding a listener shouldn't fail just because the DB open
    // is transiently broken; the next op will retry.
  })
  if (!listeners.has(keyPrefix)) {
    listeners.set(keyPrefix, [])
  }
  const bucket = listeners.get(keyPrefix)!
  if (callbackName) {
    const existing = bucket.findIndex((l) => l.name === callbackName)
    if (existing >= 0) {
      bucket[existing].callback = callback
    } else {
      bucket.push({ name: callbackName, callback })
    }
  } else {
    if (!bucket.some((l) => l.callback === callback)) {
      bucket.push({ callback })
    }
  }
}

/**
 * Remove a named listener. Silent no-op if name not registered.
 */
export function kvRemoveListener(
  keyPrefix: string,
  callbackName: string,
): void {
  const bucket = listeners.get(keyPrefix)
  if (!bucket) return
  const idx = bucket.findIndex((l) => l.name === callbackName)
  if (idx >= 0) bucket.splice(idx, 1)
}

/**
 * Return the storage usage/quota estimate or null if unavailable.
 */
export async function kvEstimate(): Promise<{
  usage: number
  quota: number
} | null> {
  try {
    if (
      typeof navigator === 'undefined' ||
      !navigator.storage ||
      typeof navigator.storage.estimate !== 'function'
    ) {
      return null
    }
    const est = await navigator.storage.estimate()
    return {
      usage: typeof est.usage === 'number' ? est.usage : 0,
      quota: typeof est.quota === 'number' ? est.quota : 0,
    }
  } catch (err) {
    console.debug('kvEstimate failed', err)
    return null
  }
}

// -----------------------------------------------------------------------------
// Meta store helpers — used by migration.ts
// -----------------------------------------------------------------------------

export async function metaGet<T>(key: string): Promise<T | null> {
  return executeWithRetry(async () => {
    const db = await getDB()
    const tx = db.transaction(META_STORE, 'readonly')
    const store = tx.objectStore(META_STORE)
    const [val] = await Promise.all([store.get(key), tx.done])
    if (val === undefined) return null
    return val as T | null
  })
}

export async function metaSet<T>(key: string, val: T): Promise<void> {
  await executeWithRetry(async () => {
    const db = await getDB()
    const tx = db.transaction(META_STORE, 'readwrite')
    const store = tx.objectStore(META_STORE)
    const putPromise = store.put(val as unknown, key)
    await Promise.all([putPromise, tx.done])
  })
}

/**
 * Internal: write directly to the kv store without firing listeners. Used
 * only during migration to bulk-import legacy data silently.
 */
export async function __kvSetSilent<T>(key: string, val: T): Promise<void> {
  await executeWithRetry(async () => {
    const db = await getDB()
    const tx = db.transaction(KV_STORE, 'readwrite')
    const store = tx.objectStore(KV_STORE)
    const putPromise = store.put(val as unknown, key)
    await Promise.all([putPromise, tx.done])
  })
}

/**
 * Internal: batch-write many entries into the kv store in a single
 * readwrite transaction without firing listeners. Used during migration so
 * thousands of legacy rows don't each open their own tx. No-op for an empty
 * input.
 */
export async function __kvSetManySilent(
  entries: Array<[string, unknown]>,
): Promise<void> {
  if (entries.length === 0) return
  await executeWithRetry(async () => {
    const db = await getDB()
    const tx = db.transaction(KV_STORE, 'readwrite')
    const store = tx.objectStore(KV_STORE)
    const puts: Array<Promise<unknown>> = []
    for (const [key, val] of entries) {
      puts.push(store.put(val, key))
    }
    await Promise.all([...puts, tx.done])
  })
}

// -----------------------------------------------------------------------------
// Test helpers
// -----------------------------------------------------------------------------

/**
 * Close the cached handle and clear listener state. Test-only.
 */
export async function __resetForTests(): Promise<void> {
  try {
    const db = await dbPromise
    db?.close()
  } catch {
    // ignore
  }
  dbPromise = null
  listeners.clear()
  persistRequested = false
}

/**
 * Exposed purely for the retry test — forcibly close the cached handle so
 * the next op must reopen.
 */
export function __forceCloseForTests(): void {
  void (async () => {
    try {
      const db = await dbPromise
      db?.close()
    } catch {
      // ignore
    }
    dbPromise = null
  })()
}
