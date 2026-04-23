import { IDBFactory } from 'fake-indexeddb'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import {
  __resetForTests,
  kvAddListener,
  kvClear,
  kvDel,
  kvEstimate,
  kvExists,
  kvGet,
  kvList,
  kvRemoveListener,
  kvRename,
  kvSet,
  metaGet,
  metaSet,
  type KvListener,
} from '@/utils/storage/idb-kv'

beforeEach(async () => {
  await __resetForTests()
  // Fresh fake IndexedDB for isolation between tests.
  ;(globalThis as unknown as { indexedDB: IDBFactory }).indexedDB =
    new IDBFactory()
})

afterEach(async () => {
  await __resetForTests()
})

describe('kvGet', () => {
  it('returns null for missing keys (never undefined)', async () => {
    const result = await kvGet('missing')
    expect(result).toBeNull()
  })

  it('round-trips a string', async () => {
    await kvSet('k', 'hello')
    expect(await kvGet('k')).toBe('hello')
  })

  it('round-trips a number', async () => {
    await kvSet('k', 42)
    expect(await kvGet('k')).toBe(42)
  })

  it('round-trips zero', async () => {
    await kvSet('k', 0)
    expect(await kvGet('k')).toBe(0)
  })

  it('round-trips a boolean', async () => {
    await kvSet('kt', true)
    await kvSet('kf', false)
    expect(await kvGet('kt')).toBe(true)
    expect(await kvGet('kf')).toBe(false)
  })

  it('round-trips explicit null', async () => {
    await kvSet('k', null)
    // kvGet returns null either way, but kvExists must distinguish.
    expect(await kvGet('k')).toBeNull()
    expect(await kvExists('k')).toBe(true)
  })

  it('round-trips empty string', async () => {
    await kvSet('k', '')
    expect(await kvGet('k')).toBe('')
  })

  it('round-trips empty array', async () => {
    await kvSet('k', [])
    expect(await kvGet('k')).toEqual([])
  })

  it('round-trips empty object', async () => {
    await kvSet('k', {})
    expect(await kvGet('k')).toEqual({})
  })

  it('round-trips nested objects', async () => {
    const obj = { a: 1, b: { c: [1, 2, { d: 'x' }] } }
    await kvSet('k', obj)
    expect(await kvGet('k')).toEqual(obj)
  })

  it('round-trips arrays of 1000 ints', async () => {
    const arr = Array.from({ length: 1000 }, (_, i) => i)
    await kvSet('k', arr)
    expect(await kvGet('k')).toEqual(arr)
  })

  it('round-trips a ~1MB base64 string', async () => {
    const big = 'A'.repeat(1024 * 1024)
    await kvSet('k', { b64: big })
    const got = await kvGet<{ b64: string }>('k')
    expect(got?.b64.length).toBe(big.length)
  })

  it('round-trips a Date object via structured clone', async () => {
    const d = new Date('2025-06-01T12:34:56.000Z')
    await kvSet('k', d)
    const got = await kvGet<Date>('k')
    expect(got).toBeInstanceOf(Date)
    expect(got?.toISOString()).toBe(d.toISOString())
  })
})

describe('kvSet', () => {
  it('overwrites existing values', async () => {
    await kvSet('k', 'one')
    await kvSet('k', 'two')
    expect(await kvGet('k')).toBe('two')
  })

  // T1: document current behavior when storing `undefined`. Storing
  // `undefined` is discouraged — the key exists in the store, but kvGet
  // normalizes the "absent" sentinel to null, so callers cannot distinguish
  // "stored undefined" from "missing". This test pins that contract so a
  // refactor can't regress it silently.
  it('storing undefined: kvExists=true but kvGet returns null (current contract)', async () => {
    await kvSet('k', undefined as unknown)
    expect(await kvExists('k')).toBe(true)
    expect(await kvGet('k')).toBeNull()
  })

  // T2: QuotaExceededError is wrapped with a "Storage quota exceeded:" prefix.
  it('wraps QuotaExceededError with a "Storage quota exceeded:" prefix', async () => {
    // Seed once so the DB is open before we poison the prototype.
    await kvSet('warmup', 1)
    const IDBObjectStoreCtor = (
      globalThis as unknown as {
        IDBObjectStore: { prototype: { put: (...a: unknown[]) => unknown } }
      }
    ).IDBObjectStore
    const origPut = IDBObjectStoreCtor.prototype.put
    let firedOnce = false
    IDBObjectStoreCtor.prototype.put = function (...args: unknown[]) {
      if (!firedOnce) {
        firedOnce = true
        throw new DOMException('quota', 'QuotaExceededError')
      }
      return origPut.apply(this, args as Parameters<typeof origPut>)
    }
    try {
      await expect(kvSet('k', 'v')).rejects.toThrow(/^Storage quota exceeded:/)
    } finally {
      IDBObjectStoreCtor.prototype.put = origPut
    }
  })

  // T10: structured clone rejects non-cloneable values (functions). The
  // caller sees a thrown error (DataCloneError from structured clone).
  it('throws when the value is not structured-cloneable (function)', async () => {
    await expect(
      kvSet('k', { fn: () => 42 } as unknown),
    ).rejects.toBeInstanceOf(Error)
  })
})

describe('kvDel', () => {
  it('is a no-op when the key is missing (does not throw, no listener)', async () => {
    const cb = vi.fn()
    await kvAddListener('', cb)
    await expect(kvDel('missing')).resolves.toBeUndefined()
    expect(cb).not.toHaveBeenCalled()
  })

  it('removes an existing key', async () => {
    await kvSet('k', 'v')
    await kvDel('k')
    expect(await kvGet('k')).toBeNull()
    expect(await kvExists('k')).toBe(false)
  })

  it('fires a del listener with the old value', async () => {
    await kvSet('k', 'old')
    const events: unknown[][] = []
    const listener: KvListener = (...args) => events.push(args)
    await kvAddListener('', listener)
    await kvDel('k')
    expect(events).toHaveLength(1)
    expect(events[0][0]).toBe('k')
    expect(events[0][1]).toBe('del')
    expect(events[0][2]).toBe('old')
    expect(events[0][3]).toBeNull()
  })
})

describe('kvExists', () => {
  it('returns false for missing keys', async () => {
    expect(await kvExists('nope')).toBe(false)
  })

  it('returns true for present keys', async () => {
    await kvSet('k', 'x')
    expect(await kvExists('k')).toBe(true)
  })

  it('returns true even when the stored value is null', async () => {
    await kvSet('k', null)
    expect(await kvExists('k')).toBe(true)
  })

  // T3: kvExists must not collapse on falsy stored values.
  it('returns true for a stored number zero', async () => {
    await kvSet('k', 0)
    expect(await kvExists('k')).toBe(true)
  })

  it('returns true for a stored empty string', async () => {
    await kvSet('k', '')
    expect(await kvExists('k')).toBe(true)
  })

  it('returns true for a stored boolean false', async () => {
    await kvSet('k', false)
    expect(await kvExists('k')).toBe(true)
  })
})

describe('kvRename', () => {
  it('moves an existing key to a new name', async () => {
    await kvSet('old', 'v')
    await kvRename('old', 'new')
    expect(await kvGet('new')).toBe('v')
    expect(await kvExists('old')).toBe(false)
  })

  it('is a no-op when the source key is missing', async () => {
    const cb = vi.fn()
    await kvAddListener('', cb)
    await expect(kvRename('absent', 'target')).resolves.toBeUndefined()
    expect(cb).not.toHaveBeenCalled()
    expect(await kvExists('target')).toBe(false)
  })

  it('fires set+del listeners on rename', async () => {
    await kvSet('old', 'v')
    const events: string[] = []
    await kvAddListener('', (key, op) => {
      events.push(`${op}:${key}`)
    })
    await kvRename('old', 'new')
    expect(events).toEqual(['set:new', 'del:old'])
  })

  // T4: destination key already exists — rename overwrites it and the
  // listener payload reports the previous destination value as oldVal.
  it('overwrites destination when it already exists, with correct listener oldVals', async () => {
    await kvSet('old', 'A')
    await kvSet('new', 'B')
    const events: unknown[][] = []
    await kvAddListener('', (...args) => events.push(args))
    await kvRename('old', 'new')
    expect(await kvGet('new')).toBe('A')
    expect(await kvGet('old')).toBeNull()
    expect(events).toHaveLength(2)
    expect(events[0]).toEqual(['new', 'set', 'B', 'A'])
    expect(events[1]).toEqual(['old', 'del', 'A', null])
  })

  // T5: source == target. Current behavior: the set writes the value back
  // to the same key, then the delete wipes it. Net effect: the key is gone.
  // NOTE: this is the observed behavior and is arguably surprising — pin it
  // with an assertion so any future change to kvRename is an explicit
  // decision, not an accident.
  it('source == target: key ends up deleted (pinned current behavior)', async () => {
    await kvSet('k', 'v')
    await kvRename('k', 'k')
    // NOTE: set-then-delete within the single tx leaves k absent.
    expect(await kvGet('k')).toBeNull()
    expect(await kvExists('k')).toBe(false)
  })
})

describe('kvList', () => {
  it('returns all keys', async () => {
    await kvSet('a', 1)
    await kvSet('b', 2)
    await kvSet('c', 3)
    const keys = await kvList()
    expect(keys.sort()).toEqual(['a', 'b', 'c'])
  })

  it('returns only kv-store keys, not meta sentinels', async () => {
    await kvSet('user_data', 'x')
    await metaSet('migration_status', 'done')
    const keys = await kvList()
    expect(keys).toEqual(['user_data'])
  })

  it('returns an empty array on a fresh DB', async () => {
    expect(await kvList()).toEqual([])
  })
})

describe('kvClear', () => {
  it('removes all kv data but preserves meta', async () => {
    await kvSet('a', 1)
    await kvSet('b', 2)
    await metaSet('migration_status', 'done')
    await kvClear()
    expect(await kvList()).toEqual([])
    expect(await kvGet('a')).toBeNull()
    expect(await metaGet('migration_status')).toBe('done')
  })

  // T6: listener payload per key. After the 3A single-tx refactor, clear
  // must still fire exactly one del notification per seeded key, carrying
  // that key's stored oldVal and newVal=null.
  it('notifies listeners once per key with correct old values', async () => {
    await kvSet('alpha', 'A')
    await kvSet('beta', 2)
    await kvSet('gamma', { nested: true })
    const events: unknown[][] = []
    await kvAddListener('', (...args) => events.push(args))
    await kvClear()
    expect(events).toHaveLength(3)
    // Order isn't guaranteed by kvClear — sort for a stable assertion.
    const byKey = [...events].sort((a, b) =>
      String(a[0]).localeCompare(String(b[0])),
    )
    expect(byKey[0]).toEqual(['alpha', 'del', 'A', null])
    expect(byKey[1]).toEqual(['beta', 'del', 2, null])
    expect(byKey[2]).toEqual(['gamma', 'del', { nested: true }, null])
  })
})

describe('listeners — prefix matching', () => {
  it('fires for matching prefixes and not for non-matching', async () => {
    const configEvents: string[] = []
    const chatEvents: string[] = []
    await kvAddListener('config_', (k) => configEvents.push(k))
    await kvAddListener('chat_', (k) => chatEvents.push(k))
    await kvSet('config_foo', 1)
    await kvSet('chat_foo', 2)
    expect(configEvents).toEqual(['config_foo'])
    expect(chatEvents).toEqual(['chat_foo'])
  })
})

describe('listeners — de-dup', () => {
  it('replaces a named listener when added twice', async () => {
    const first = vi.fn()
    const second = vi.fn()
    await kvAddListener('k', first, 'same')
    await kvAddListener('k', second, 'same')
    await kvSet('k', 1)
    expect(first).not.toHaveBeenCalled()
    expect(second).toHaveBeenCalledTimes(1)
  })

  it('de-dups anonymous listeners by function identity', async () => {
    const cb = vi.fn()
    await kvAddListener('k', cb)
    await kvAddListener('k', cb)
    await kvSet('k', 1)
    expect(cb).toHaveBeenCalledTimes(1)
  })

  it('kvRemoveListener removes a named entry; missing name is a no-op', async () => {
    const cb = vi.fn()
    await kvAddListener('k', cb, 'named')
    kvRemoveListener('k', 'named')
    kvRemoveListener('k', 'doesnotexist') // no throw
    await kvSet('k', 1)
    expect(cb).not.toHaveBeenCalled()
  })
})

describe('listeners — semantics', () => {
  it('fires set with (key, op, oldVal=null, newVal) on first set', async () => {
    const events: unknown[][] = []
    await kvAddListener('', (...args) => events.push(args))
    await kvSet('k', 'v')
    expect(events).toEqual([['k', 'set', null, 'v']])
  })

  it('fires set with the previous value as oldVal on overwrite', async () => {
    await kvSet('k', 'v1')
    const events: unknown[][] = []
    await kvAddListener('', (...args) => events.push(args))
    await kvSet('k', 'v2')
    expect(events).toEqual([['k', 'set', 'v1', 'v2']])
  })

  it('a throwing listener does not break the write or other listeners', async () => {
    const good = vi.fn()
    const bad = () => {
      throw new Error('boom')
    }
    await kvAddListener('', bad)
    await kvAddListener('', good)
    await expect(kvSet('k', 'v')).resolves.toBeUndefined()
    expect(good).toHaveBeenCalledTimes(1)
    expect(await kvGet('k')).toBe('v')
  })

  it('listener sees the post-commit value (fires after tx.done)', async () => {
    let observed: unknown = null
    await kvAddListener('', async (key) => {
      observed = await kvGet(key)
    })
    await kvSet('k', 'final')
    // Give the async listener time to resolve its own read.
    await new Promise((r) => setTimeout(r, 10))
    expect(observed).toBe('final')
  })
})

describe('parallel writes', () => {
  it('parallel sets to the same key do not corrupt data', async () => {
    await Promise.all([kvSet('k', 'a'), kvSet('k', 'b'), kvSet('k', 'c')])
    const v = await kvGet<string>('k')
    expect(['a', 'b', 'c']).toContain(v)
  })

  it('parallel sets to different keys all persist', async () => {
    await Promise.all([kvSet('a', 1), kvSet('b', 2), kvSet('c', 3)])
    expect(await kvGet('a')).toBe(1)
    expect(await kvGet('b')).toBe(2)
    expect(await kvGet('c')).toBe(3)
  })
})

describe('kvEstimate', () => {
  const origNav = globalThis.navigator

  afterEach(() => {
    Object.defineProperty(globalThis, 'navigator', {
      value: origNav,
      configurable: true,
    })
  })

  it('returns {usage, quota} when navigator.storage.estimate is available', async () => {
    Object.defineProperty(globalThis, 'navigator', {
      value: {
        storage: {
          estimate: async () => ({ usage: 111, quota: 2222 }),
        },
      },
      configurable: true,
    })
    expect(await kvEstimate()).toEqual({ usage: 111, quota: 2222 })
  })

  it('returns null when navigator.storage is missing', async () => {
    Object.defineProperty(globalThis, 'navigator', {
      value: {},
      configurable: true,
    })
    expect(await kvEstimate()).toBeNull()
  })

  it('returns null when navigator itself is undefined', async () => {
    Object.defineProperty(globalThis, 'navigator', {
      value: undefined,
      configurable: true,
    })
    expect(await kvEstimate()).toBeNull()
  })

  it('returns null (does not throw) when estimate throws', async () => {
    Object.defineProperty(globalThis, 'navigator', {
      value: {
        storage: {
          estimate: async () => {
            throw new Error('boom')
          },
        },
      },
      configurable: true,
    })
    expect(await kvEstimate()).toBeNull()
  })
})

describe('re-opening the DB', () => {
  it('preserves data across __resetForTests (cached handle is reopened)', async () => {
    await kvSet('persist', { v: 1 })
    await __resetForTests()
    // Do NOT reset the fake-indexeddb factory — only drop the handle.
    expect(await kvGet('persist')).toEqual({ v: 1 })
  })
})

describe('retry on transient IndexedDB handle errors', () => {
  // Shared helper: patch IDBObjectStore.prototype.get so the first call
  // throws `err`, and subsequent calls pass through to the original.
  // Returns the spy + a restore() function.
  function patchGetToThrowOnce(err: unknown): {
    spy: { calls: number }
    restore: () => void
  } {
    const IDBObjectStoreCtor = (
      globalThis as unknown as {
        IDBObjectStore: { prototype: { get: (...a: unknown[]) => unknown } }
      }
    ).IDBObjectStore
    const origGet = IDBObjectStoreCtor.prototype.get
    let thrown = false
    const spy = { calls: 0 }
    IDBObjectStoreCtor.prototype.get = function (...args: unknown[]) {
      spy.calls += 1
      if (!thrown) {
        thrown = true
        throw err
      }
      return origGet.apply(this, args as Parameters<typeof origGet>)
    }
    return {
      spy,
      restore: () => {
        IDBObjectStoreCtor.prototype.get = origGet
      },
    }
  }

  // Suppress the warn() that `executeWithRetry` emits on each retry.
  let warnSpy: ReturnType<typeof vi.spyOn>
  beforeEach(() => {
    warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {})
  })
  afterEach(() => {
    warnSpy.mockRestore()
  })

  // T7: the previously .skip'd test — now implemented by patching the
  // IDBObjectStore prototype on fake-indexeddb.
  it('reopens the DB and retries on InvalidStateError', async () => {
    await kvSet('seeded-key', 'value')
    const { spy, restore } = patchGetToThrowOnce(
      new DOMException('state', 'InvalidStateError'),
    )
    try {
      const got = await kvGet<string>('seeded-key')
      expect(got).toBe('value')
      // First call threw, retry called get() again — so at least 2 calls.
      expect(spy.calls).toBeGreaterThanOrEqual(2)
    } finally {
      restore()
    }
  })

  // T8: 2C-BLOCKER — UnknownError with the iOS 17.4+ "Connection lost"
  // message must also trigger the retry path.
  it('retries on UnknownError with "Connection to Indexed Database server lost"', async () => {
    await kvSet('seeded-key', 'value')
    const err = new DOMException(
      'Connection to Indexed Database server lost',
      'UnknownError',
    )
    const { spy, restore } = patchGetToThrowOnce(err)
    try {
      const got = await kvGet<string>('seeded-key')
      expect(got).toBe('value')
      expect(spy.calls).toBeGreaterThanOrEqual(2)
    } finally {
      restore()
    }
  })

  // T9: TransactionInactiveError is also retryable.
  it('retries on TransactionInactiveError', async () => {
    await kvSet('seeded-key', 'value')
    const { spy, restore } = patchGetToThrowOnce(
      new DOMException('inactive', 'TransactionInactiveError'),
    )
    try {
      const got = await kvGet<string>('seeded-key')
      expect(got).toBe('value')
      expect(spy.calls).toBeGreaterThanOrEqual(2)
    } finally {
      restore()
    }
  })
})
