/**
 * One-time migration from the legacy PouchDB-backed `_pouch_mydatabase`
 * IndexedDB to the new `ramjet_kv_v2` store.
 *
 * The migration is idempotent and resumable via the `migration_status`
 * sentinel persisted in the `meta` store. States:
 *   undefined | 'pending' -> copy docs -> 'copied' -> destroy old DB -> 'done'
 */
import { __kvSetManySilent, metaGet, metaSet } from './idb-kv'

const MIGRATION_STATUS_KEY = 'migration_status'
const LEGACY_DB_NAME = '_pouch_mydatabase'
const LEGACY_BLOB_SUPPORT_DB = '_pouch_check_blob_support'
const POUCH_DB_NAME = 'mydatabase'

type MigrationStatus = 'pending' | 'copied' | 'done'

interface PouchDocShape {
  _id: string
  _deleted?: boolean
  val?: string
}

interface PouchAllDocsRow {
  id: string
  doc?: PouchDocShape
}

interface PouchAllDocsResponse {
  rows: PouchAllDocsRow[]
}

interface PouchLike {
  allDocs(opts: { include_docs: boolean }): Promise<PouchAllDocsResponse>
  destroy(): Promise<unknown>
  info(): Promise<unknown>
  close?(): Promise<unknown>
}

type PouchCtor = new (name: string) => PouchLike

/**
 * Delete an IndexedDB by name, resolving (never rejecting) on success,
 * error, or blocked. Gives up silently if another tab holds the DB open.
 */
export function deleteDBIgnoringBlocked(
  name: string,
  timeoutMs = 3000,
): Promise<void> {
  return new Promise((resolve) => {
    let req: IDBOpenDBRequest
    try {
      req = indexedDB.deleteDatabase(name)
    } catch (err) {
      console.warn(`deleteDatabase ${name} threw`, err)
      resolve()
      return
    }
    const timer = setTimeout(() => resolve(), timeoutMs)
    req.onsuccess = () => {
      clearTimeout(timer)
      resolve()
    }
    req.onerror = () => {
      clearTimeout(timer)
      console.warn(`deleteDatabase ${name} errored`, req.error)
      resolve()
    }
    req.onblocked = () => {
      clearTimeout(timer)
      console.warn(`deleteDatabase ${name} blocked (another tab has it open?)`)
      resolve()
    }
  })
}

/**
 * Best-effort check whether the legacy PouchDB IndexedDB exists. Falls back
 * to opening it when `indexedDB.databases()` is unavailable.
 */
async function legacyPouchDBExists(): Promise<boolean> {
  try {
    if (typeof indexedDB.databases === 'function') {
      const list = await indexedDB.databases()
      return list.some((info) => info.name === LEGACY_DB_NAME)
    }
  } catch (err) {
    console.debug('indexedDB.databases() threw, falling back', err)
  }
  // Fallback: try to open and check — if it creates a fresh empty DB we
  // can still delete it harmlessly below, but err on caller-friendly side.
  return true
}

/**
 * Run the one-time migration. Safe to call on every app boot. Idempotent.
 */
export async function migrateFromPouchDB(): Promise<void> {
  const status = await metaGet<MigrationStatus>(MIGRATION_STATUS_KEY)

  if (status === 'done') {
    return
  }

  // --------------------------- copy phase ---------------------------
  if (status === undefined || status === null || status === 'pending') {
    const exists = await legacyPouchDBExists()
    if (!exists) {
      await metaSet<MigrationStatus>(MIGRATION_STATUS_KEY, 'done')
      return
    }

    await metaSet<MigrationStatus>(MIGRATION_STATUS_KEY, 'pending')

    let PouchDBCtor: PouchCtor | null = null
    try {
      const mod = await import('pouchdb-browser')
      PouchDBCtor = (mod as unknown as { default: PouchCtor }).default
    } catch (err) {
      console.error('Failed to load pouchdb-browser for migration', err)
      // If we can't load pouchdb we can't do anything — best-effort: mark
      // done so we don't block the app forever on a fresh environment.
      await metaSet<MigrationStatus>(MIGRATION_STATUS_KEY, 'done')
      return
    }

    // Wrap instantiate + allDocs + copy in try/catch so a corrupt legacy DB
    // (PouchDB #8229) does not leave migration stuck at 'pending' forever.
    // On failure we still advance to 'copied' so the destroy phase can
    // attempt to delete the bad DB on this or the next boot.
    let pdb: PouchLike | null = null
    try {
      pdb = new PouchDBCtor(POUCH_DB_NAME)
      const result = await pdb.allDocs({ include_docs: true })
      const batch: Array<[string, unknown]> = []
      for (const row of result.rows) {
        const doc = row.doc
        if (!doc || doc._deleted) continue
        if (typeof doc.val !== 'string') continue
        try {
          const parsed = JSON.parse(doc.val)
          batch.push([row.id, parsed])
        } catch (err) {
          // Skip malformed-JSON rows individually; valid rows still batch.
          console.warn(`Skipping legacy row ${row.id} with malformed JSON`, err)
        }
      }
      await __kvSetManySilent(batch)
      await metaSet<MigrationStatus>(MIGRATION_STATUS_KEY, 'copied')
    } catch (err) {
      console.warn('Migration copy phase failed; skipping to cleanup', err)
      // Advance to 'copied' so the destroy phase runs and can delete the
      // (likely corrupt) legacy DB. Next boot will then advance to 'done'.
      await metaSet<MigrationStatus>(MIGRATION_STATUS_KEY, 'copied')
    } finally {
      // Close the pouch handle before we try to destroy the underlying DB.
      try {
        if (pdb && typeof pdb.close === 'function') {
          await pdb.close()
        }
      } catch (err) {
        console.debug('pouch close failed', err)
      }
    }
  }

  // -------------------------- destroy phase -------------------------
  const statusAfterCopy = await metaGet<MigrationStatus>(MIGRATION_STATUS_KEY)
  if (statusAfterCopy === 'copied') {
    // Try pouch destroy first — it's the most graceful path. Fall through to
    // a raw indexedDB.deleteDatabase regardless.
    try {
      const mod = await import('pouchdb-browser')
      const PouchDBCtor = (mod as unknown as { default: PouchCtor }).default
      const pdb = new PouchDBCtor(POUCH_DB_NAME)
      try {
        await pdb.destroy()
      } catch (err) {
        console.warn('pouch destroy failed; will fall back to deleteDB', err)
      }
    } catch (err) {
      console.debug('Could not reimport pouchdb for destroy phase', err)
    }
    await deleteDBIgnoringBlocked(LEGACY_DB_NAME)
    await deleteDBIgnoringBlocked(LEGACY_BLOB_SUPPORT_DB)
    await metaSet<MigrationStatus>(MIGRATION_STATUS_KEY, 'done')
  }
}
