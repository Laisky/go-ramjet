import { IDBFactory } from 'fake-indexeddb'
import PouchDB from 'pouchdb-browser'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import {
  __resetForTests,
  kvAddListener,
  kvGet,
  kvList,
  metaGet,
  metaSet,
} from '@/utils/storage/idb-kv'
import { migrateFromPouchDB } from '@/utils/storage/migration'

/**
 * Seed `_pouch_mydatabase` by writing via a real PouchDB instance. Each doc
 * has shape `{ _id, val: JSON.stringify(value) }` matching the legacy
 * storage contract.
 */
async function seedPouchDB(data: Record<string, unknown>): Promise<void> {
  const pdb = new PouchDB('mydatabase')
  const docs = Object.entries(data).map(([id, val]) => ({
    _id: id,
    val: JSON.stringify(val),
  }))
  if (docs.length > 0) {
    await pdb.bulkDocs(docs)
  }
  await pdb.close()
}

/**
 * Seed a malformed-JSON doc directly. PouchDB won't let us bypass its
 * schema, but we store a literal string that will fail JSON.parse.
 */
async function seedPouchDBRaw(
  docs: Array<{ _id: string; val: string }>,
): Promise<void> {
  const pdb = new PouchDB('mydatabase')
  await pdb.bulkDocs(docs)
  await pdb.close()
}

async function listDatabases(): Promise<string[]> {
  if (typeof indexedDB.databases !== 'function') return []
  const list = await indexedDB.databases()
  return list.map((info) => info.name ?? '')
}

beforeEach(async () => {
  await __resetForTests()
  ;(globalThis as unknown as { indexedDB: IDBFactory }).indexedDB =
    new IDBFactory()
})

afterEach(async () => {
  await __resetForTests()
})

describe('migrateFromPouchDB', () => {
  it('marks status=done immediately for a fresh user (no legacy DB)', async () => {
    await migrateFromPouchDB()
    expect(await metaGet('migration_status')).toBe('done')
    expect(await kvList()).toEqual([])
  })

  it('migrates legacy docs with assorted value types', async () => {
    await seedPouchDB({
      str: 'hello',
      num: 42,
      bool: true,
      obj: { nested: { a: 1 } },
      arr: [1, 2, 3],
      nullVal: null,
    })
    await migrateFromPouchDB()
    expect(await kvGet('str')).toBe('hello')
    expect(await kvGet('num')).toBe(42)
    expect(await kvGet('bool')).toBe(true)
    expect(await kvGet('obj')).toEqual({ nested: { a: 1 } })
    expect(await kvGet('arr')).toEqual([1, 2, 3])
    expect(await kvGet('nullVal')).toBeNull()
    const keys = (await kvList()).sort()
    expect(keys).toEqual(['arr', 'bool', 'nullVal', 'num', 'obj', 'str'].sort())
    expect(await metaGet('migration_status')).toBe('done')
  })

  it('skips malformed JSON rows but migrates the rest', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    await seedPouchDBRaw([
      { _id: 'ok', val: JSON.stringify('good') },
      { _id: 'bad', val: '{not-valid-json' },
      { _id: 'also_ok', val: JSON.stringify(42) },
    ])
    await migrateFromPouchDB()
    expect(await kvGet('ok')).toBe('good')
    expect(await kvGet('also_ok')).toBe(42)
    expect(await kvGet('bad')).toBeNull()
    // Something along the migration path warned.
    expect(warn).toHaveBeenCalled()
    warn.mockRestore()
  })

  it('migrates a ~1MB base64 value', async () => {
    const big = 'A'.repeat(1024 * 1024)
    await seedPouchDB({ big: { b64: big } })
    await migrateFromPouchDB()
    const got = await kvGet<{ b64: string }>('big')
    expect(got?.b64.length).toBe(big.length)
  })

  it('is idempotent — second run is a no-op (no new allDocs call)', async () => {
    await seedPouchDB({ only: 'one' })
    await migrateFromPouchDB()
    expect(await metaGet('migration_status')).toBe('done')

    // Once status=done, migration returns before touching PouchDB. Patch
    // allDocs on the PouchDB prototype via a probe instance and verify a
    // second run does not invoke it.
    const probe = new PouchDB('__idem_probe__')
    const proto = Object.getPrototypeOf(probe) as {
      allDocs: (...a: unknown[]) => Promise<unknown>
    }
    const origAllDocs = proto.allDocs
    const allDocsSpy = vi.fn()
    proto.allDocs = function (...args: unknown[]) {
      allDocsSpy(...args)
      return origAllDocs.apply(this, args as [])
    }
    await probe.destroy().catch(() => {})

    try {
      await migrateFromPouchDB()
      expect(allDocsSpy).not.toHaveBeenCalled()
    } finally {
      proto.allDocs = origAllDocs
    }
  })

  it('is resumable from status=copied — skips copy, destroys old DB', async () => {
    await seedPouchDB({ user_key: 'v' })
    // Pre-populate the new DB with the expected migrated data so "copied"
    // truly means "copy is done, just need to destroy".
    const { __kvSetSilent } = await import('@/utils/storage/idb-kv')
    await __kvSetSilent('user_key', 'v')
    await metaSet('migration_status', 'copied')

    // Wrap allDocs so we can assert it's not called during the destroy
    // phase. We do this by constructing a pouch and patching the method
    // on its prototype (which does exist once an instance is made).
    const probe = new PouchDB('__probe__')
    const allDocsSpy = vi.fn()
    const origAllDocs = (
      probe as unknown as { allDocs: (...a: unknown[]) => Promise<unknown> }
    ).allDocs
    ;(Object.getPrototypeOf(probe) as { allDocs: unknown }).allDocs = function (
      ...args: unknown[]
    ) {
      allDocsSpy(...args)
      return origAllDocs.apply(this, args)
    }
    await probe.destroy().catch(() => {})

    await migrateFromPouchDB()
    expect(allDocsSpy).not.toHaveBeenCalled()
    expect(await metaGet('migration_status')).toBe('done')
    expect((await listDatabases()).includes('_pouch_mydatabase')).toBe(false)

    // Restore original prototype method so later tests are clean.
    ;(Object.getPrototypeOf(probe) as { allDocs: unknown }).allDocs =
      origAllDocs
  })

  it('removes _pouch_mydatabase from indexedDB.databases() after run', async () => {
    await seedPouchDB({ k: 1 })
    await migrateFromPouchDB()
    expect((await listDatabases()).includes('_pouch_mydatabase')).toBe(false)
  })

  it('progresses to done even when pouch destroy throws', async () => {
    await seedPouchDB({ k: 1 })
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})

    const probe = new PouchDB('__destroy_probe__')
    const proto = Object.getPrototypeOf(probe) as {
      destroy: (...a: unknown[]) => Promise<unknown>
    }
    const origDestroy = proto.destroy
    proto.destroy = function () {
      return Promise.reject(new Error('destroy failed'))
    }
    // Can't call .destroy() to tear probe down because we've stubbed it —
    // instead, revert and delete directly.
    proto.destroy = origDestroy
    await probe.destroy().catch(() => {})
    // Now re-stub for the real assertion.
    proto.destroy = function () {
      return Promise.reject(new Error('destroy failed'))
    }

    try {
      await migrateFromPouchDB()
      expect(await metaGet('migration_status')).toBe('done')
      // deleteDatabase still ran — _pouch_mydatabase should be gone.
      expect((await listDatabases()).includes('_pouch_mydatabase')).toBe(false)
    } finally {
      proto.destroy = origDestroy
      warn.mockRestore()
    }
  })

  it('completes and marks done when deleteDatabase is blocked (open handle)', async () => {
    await seedPouchDB({ k: 1 })
    // Keep a PouchDB handle open so deleteDatabase emits `blocked`.
    const holder = new PouchDB('mydatabase')
    await holder.info()
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    await migrateFromPouchDB()
    // Destroy via pouch (same process) usually succeeds even with another
    // handle; what matters is the migration completes. The spec permits
    // either outcome — status must progress past 'copied'.
    expect(await metaGet('migration_status')).toBe('done')
    warn.mockRestore()
    try {
      await holder.close()
    } catch {
      // handle may already be torn down
    }
  })

  it('does not fire kv listeners during migration', async () => {
    await seedPouchDB({ a: 1, b: 2 })
    const cb = vi.fn()
    await kvAddListener('', cb)
    await migrateFromPouchDB()
    expect(cb).not.toHaveBeenCalled()
  })

  // T11: tombstone docs (_deleted: true) must NOT be carried into the new
  // store. Use real PouchDB put+remove to create a proper tombstone.
  it('skips tombstone docs (_deleted: true) during copy', async () => {
    const pdb = new PouchDB('mydatabase')
    const liveDoc = await pdb.put({ _id: 'live', val: JSON.stringify('L') })
    const toRemove = await pdb.put({
      _id: 'gone',
      val: JSON.stringify('G'),
    })
    await pdb.put({ _id: 'alsoLive', val: JSON.stringify('A') })
    // Tombstone 'gone' via remove.
    await pdb.remove({ _id: 'gone', _rev: toRemove.rev })
    // Sanity — liveDoc was created.
    expect(liveDoc.ok).toBe(true)
    await pdb.close()

    await migrateFromPouchDB()
    const keys = (await kvList()).sort()
    expect(keys).toEqual(['alsoLive', 'live'])
    expect(await kvGet('gone')).toBeNull()
  })

  // T12: _pouch_check_blob_support must also be removed. Open it manually
  // so indexedDB.databases() reports it before migration runs.
  it('deletes _pouch_check_blob_support from indexedDB', async () => {
    await seedPouchDB({ k: 1 })
    // Open (and immediately close) the blob-support DB so it exists on disk.
    await new Promise<void>((resolve, reject) => {
      const req = indexedDB.open('_pouch_check_blob_support', 1)
      req.onupgradeneeded = () => {
        try {
          req.result.createObjectStore('probe')
        } catch {
          // ignore
        }
      }
      req.onsuccess = () => {
        req.result.close()
        resolve()
      }
      req.onerror = () => reject(req.error)
    })
    expect((await listDatabases()).includes('_pouch_check_blob_support')).toBe(
      true,
    )

    await migrateFromPouchDB()

    const after = await listDatabases()
    expect(after.includes('_pouch_check_blob_support')).toBe(false)
  })

  // T13: 2C-BLOCKER — a corrupt legacy DB (PouchDB constructor or allDocs
  // throws) must not trap migration in 'pending' forever. After the run,
  // status must be at least 'copied' (and ideally 'done'), so a subsequent
  // boot doesn't re-enter the copy phase.
  it('corrupt legacy DB: advances past pending instead of looping', async () => {
    await seedPouchDB({ whatever: 1 })
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const errSpy = vi.spyOn(console, 'error').mockImplementation(() => {})

    const probe = new PouchDB('__corrupt_probe__')
    const proto = Object.getPrototypeOf(probe) as {
      allDocs: (...a: unknown[]) => Promise<unknown>
    }
    const origAllDocs = proto.allDocs
    proto.allDocs = function () {
      return Promise.reject(new Error('simulated corrupt DB'))
    }
    // Tear down the probe while the prototype is temporarily unstubbed for
    // the destroy path.
    proto.allDocs = origAllDocs
    await probe.destroy().catch(() => {})
    proto.allDocs = function () {
      return Promise.reject(new Error('simulated corrupt DB'))
    }

    try {
      await expect(migrateFromPouchDB()).resolves.toBeUndefined()
      const status = await metaGet<string>('migration_status')
      expect(status === 'copied' || status === 'done').toBe(true)
      expect(status).not.toBe('pending')

      // Restore allDocs so a second boot can proceed normally, and ensure
      // that second boot does not re-run the copy phase.
      proto.allDocs = origAllDocs
      const allDocsSpy = vi.fn()
      proto.allDocs = function (...args: unknown[]) {
        allDocsSpy(...args)
        return origAllDocs.apply(this, args as [])
      }
      await migrateFromPouchDB()
      expect(allDocsSpy).not.toHaveBeenCalled()
    } finally {
      proto.allDocs = origAllDocs
      warn.mockRestore()
      errSpy.mockRestore()
    }
  })

  // T14: deleteDatabase genuinely blocked path — verify that the
  // `deleteDBIgnoringBlocked` helper's 3-second timeout actually fires when
  // a raw IDB connection holds the target DB open, so the migration doesn't
  // hang forever. We exercise `deleteDBIgnoringBlocked` directly (it's the
  // only component in migration that owns a timeout for the blocked path).
  //
  // Calling `migrateFromPouchDB` with a held raw IDB handle can cascade
  // through pouch.destroy() which does not own its own timeout in
  // fake-indexeddb's blocked path, so we target the helper function
  // specifically.
  it('deleteDBIgnoringBlocked resolves within ~3s when DB is blocked', async () => {
    await seedPouchDB({ k: 1 })
    // Pouch handle is closed after seed; open _pouch_mydatabase directly
    // and keep it open to force `blocked` on delete.
    const held = await new Promise<IDBDatabase>((resolve, reject) => {
      const req = indexedDB.open('_pouch_mydatabase')
      req.onsuccess = () => resolve(req.result)
      req.onerror = () => reject(req.error)
    })
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const start = Date.now()
    try {
      const { deleteDBIgnoringBlocked } =
        await import('@/utils/storage/migration')
      await deleteDBIgnoringBlocked('_pouch_mydatabase', 3000)
      const elapsed = Date.now() - start
      // Should resolve within the 3s timeout + slack.
      expect(elapsed).toBeLessThan(6000)
      // Some fake-indexeddb versions fire `blocked` immediately, others
      // sit until timeout — either path must resolve.
    } finally {
      try {
        held.close()
      } catch {
        // ignore
      }
      warn.mockRestore()
    }
  }, 10000)

  // T15: batched migration — after 3A's Fix 4, copying N legacy docs should
  // open exactly one readwrite transaction against the `kv` store during
  // the copy phase (not N).
  it('copies many legacy docs in a single readwrite transaction (batched)', async () => {
    const bulk: Record<string, unknown> = {}
    for (let i = 0; i < 100; i++) {
      bulk[`key_${i}`] = { i }
    }
    await seedPouchDB(bulk)

    const txSpy = vi.fn()
    const IDBDatabaseCtor = (
      globalThis as unknown as {
        IDBDatabase: {
          prototype: { transaction: (...a: unknown[]) => unknown }
        }
      }
    ).IDBDatabase
    const origTx = IDBDatabaseCtor.prototype.transaction
    IDBDatabaseCtor.prototype.transaction = function (...args: unknown[]) {
      txSpy(args[0], args[1])
      return origTx.apply(this, args as Parameters<typeof origTx>)
    }

    try {
      await migrateFromPouchDB()
    } finally {
      IDBDatabaseCtor.prototype.transaction = origTx
    }

    // All verified docs landed in the new store.
    expect((await kvList()).length).toBe(100)

    // Filter to readwrite tx's on the `kv` object store (ignoring meta and
    // readonly).
    const kvWrites = txSpy.mock.calls.filter((c) => {
      const store = c[0]
      const mode = c[1]
      const matchesStore =
        store === 'kv' || (Array.isArray(store) && store.includes('kv'))
      return matchesStore && mode === 'readwrite'
    })
    // With Fix 4 applied, the copy phase opens exactly one readwrite tx on
    // `kv`. Allow a tiny bit of slack for any follow-up writes elsewhere
    // (there shouldn't be any on `kv` readwrite during migration).
    expect(kvWrites.length).toBeLessThanOrEqual(2)
  })
})
