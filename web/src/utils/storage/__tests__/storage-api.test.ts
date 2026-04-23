import { IDBFactory } from 'fake-indexeddb'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

// Intercept migration module so we can assert it runs exactly once across
// many facade calls and simulate its throwing case.
vi.mock('@/utils/storage/migration', () => ({
  migrateFromPouchDB: vi.fn(async () => {
    // no-op by default; individual tests can re-mock
  }),
}))

import { __resetForTests } from '@/utils/storage/idb-kv'
import { migrateFromPouchDB } from '@/utils/storage/migration'
import {
  __resetMigrationForTests,
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
  StorageKeys,
  type KvOperation,
} from '@/utils/storage'

const migrateMock = vi.mocked(migrateFromPouchDB)

beforeEach(async () => {
  migrateMock.mockReset()
  migrateMock.mockResolvedValue(undefined)
  __resetMigrationForTests()
  await __resetForTests()
  ;(globalThis as unknown as { indexedDB: IDBFactory }).indexedDB =
    new IDBFactory()
})

afterEach(async () => {
  __resetMigrationForTests()
  await __resetForTests()
})

describe('storage.ts public facade — API surface', () => {
  it('exports every expected symbol with the correct runtime type', () => {
    expect(typeof kvGet).toBe('function')
    expect(typeof kvSet).toBe('function')
    expect(typeof kvDel).toBe('function')
    expect(typeof kvExists).toBe('function')
    expect(typeof kvRename).toBe('function')
    expect(typeof kvList).toBe('function')
    expect(typeof kvAddListener).toBe('function')
    expect(typeof kvRemoveListener).toBe('function')
    expect(typeof kvEstimate).toBe('function')
    expect(StorageKeys).toBeDefined()
    // Exercise the exported type so TS doesn't tree-shake it away.
    const op: KvOperation = 'set'
    expect(op).toBe('set')
  })

  it('StorageKeys constants are unchanged', () => {
    expect(StorageKeys).toEqual({
      PINNED_MATERIALS: 'config_api_pinned_materials',
      ALLOWED_MODELS: 'config_chat_models',
      CUSTOM_DATASET_PASSWORD: 'config_chat_dataset_key',
      PROMPT_SHORTCUTS: 'config_prompt_shortcuts',
      SESSION_HISTORY_PREFIX: 'chat_user_session_',
      SESSION_CONFIG_PREFIX: 'chat_user_config_',
      SELECTED_SESSION: 'config_selected_session',
      SESSION_ORDER: 'config_session_order',
      SYNC_KEY: 'config_sync_key',
      VERSION_DATE: 'config_version_date',
      IGNORED_VERSION: 'config_ignored_version',
      USER_INFO: 'config_user_info',
      SESSION_DRAFTS: 'chat_session_drafts',
      CHAT_DATA_PREFIX: 'chat_data_',
      DELETED_CHAT_IDS: 'deleted_chat_ids',
    })
    expect(Object.keys(StorageKeys)).toHaveLength(15)
  })
})

describe('storage.ts public facade — behavior', () => {
  it('kvGet returns null (not undefined) for a missing key', async () => {
    const v = await kvGet('no_such_key')
    expect(v).toBeNull()
  })

  it('round-trips a realistic SessionConfig-shaped object', async () => {
    interface SessionConfig {
      model: string
      temperature: number
      systemPrompt: string
      stream: boolean
    }
    const cfg: SessionConfig = {
      model: 'claude-opus-4-7',
      temperature: 0.7,
      systemPrompt: 'You are helpful.',
      stream: true,
    }
    await kvSet(`${StorageKeys.SESSION_CONFIG_PREFIX}s1`, cfg)
    const got = await kvGet<SessionConfig>(
      `${StorageKeys.SESSION_CONFIG_PREFIX}s1`,
    )
    expect(got).toEqual(cfg)
  })

  it('round-trips a ChatMessageData-shaped object with a base64 image', async () => {
    const img = 'A'.repeat(512 * 1024)
    const msg = {
      role: 'user' as const,
      content: 'here is a picture',
      attachments: [{ type: 'image', data: `data:image/png;base64,${img}` }],
      ts: Date.now(),
    }
    await kvSet(`${StorageKeys.CHAT_DATA_PREFIX}user_abc123`, msg)
    const got = await kvGet<typeof msg>(
      `${StorageKeys.CHAT_DATA_PREFIX}user_abc123`,
    )
    expect(got?.content).toBe('here is a picture')
    expect(got?.attachments[0]?.data.length).toBe(
      `data:image/png;base64,${img}`.length,
    )
  })

  it('runs migration exactly once across many facade calls', async () => {
    for (let i = 0; i < 10; i++) {
      await kvGet(`k${i}`)
    }
    expect(migrateMock).toHaveBeenCalledTimes(1)
  })

  it('swallows migration errors — subsequent ops still work', async () => {
    const err = vi.spyOn(console, 'error').mockImplementation(() => {})
    migrateMock.mockRejectedValueOnce(new Error('migration boom'))
    // First call triggers the failing migration but must not throw.
    expect(await kvGet('whatever')).toBeNull()
    // Subsequent calls continue to work on the empty new DB.
    await kvSet('k', 'v')
    expect(await kvGet('k')).toBe('v')
    err.mockRestore()
  })

  it('listeners registered via the facade fire on set', async () => {
    const events: Array<[string, KvOperation]> = []
    await kvAddListener('config_', (k, op) => {
      events.push([k, op])
    })
    await kvSet('config_foo', 1)
    await kvSet('chat_bar', 2)
    expect(events).toEqual([['config_foo', 'set']])
  })

  it('listeners registered via the facade fire on del', async () => {
    await kvSet('config_foo', 1)
    const events: Array<[string, KvOperation]> = []
    await kvAddListener('config_', (k, op) => {
      events.push([k, op])
    })
    await kvDel('config_foo')
    expect(events).toEqual([['config_foo', 'del']])
  })

  it('kvRemoveListener via the facade works', async () => {
    const cb = vi.fn()
    await kvAddListener('k', cb, 'named')
    kvRemoveListener('k', 'named')
    await kvSet('k', 'v')
    expect(cb).not.toHaveBeenCalled()
  })

  // T16: kvClear through the facade fires one del-listener per seeded key,
  // complementing the lower-level assertion in idb-kv.test.ts (T6).
  it('kvClear via the facade fires a del-listener for each seeded key', async () => {
    await kvSet('alpha', 1)
    await kvSet('beta', 'two')
    await kvSet('gamma', { nested: true })
    const events: Array<[string, KvOperation]> = []
    await kvAddListener('', (k, op) => {
      events.push([k, op])
    })
    await kvClear()
    expect(events).toHaveLength(3)
    const keys = events.map(([k]) => k).sort()
    expect(keys).toEqual(['alpha', 'beta', 'gamma'])
    expect(events.every(([, op]) => op === 'del')).toBe(true)
    expect(await kvList()).toEqual([])
  })
})
