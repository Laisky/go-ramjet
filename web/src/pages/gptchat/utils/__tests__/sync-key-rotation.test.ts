import { describe, expect, it, vi } from 'vitest'

import { rotateSyncKey, type SyncKeyRotationDeps } from '../sync-key-rotation'

const OLD_KEY = 'sk-reused-as-sync-key'
const NEW_KEY = 'sync-n3wn3wn3wn3wn3w'

/**
 * fakeDeps builds rotation dependencies that record every call in order,
 * with overrides applied on top.
 */
function fakeDeps(overrides: Partial<SyncKeyRotationDeps> = {}) {
  const calls: string[] = []
  const deps: SyncKeyRotationDeps = {
    download: vi.fn(async (key: string) => {
      calls.push(`download:${key}`)
      return { chat_user_config_9: { api_token: 'sk-cloud-only' } }
    }),
    importData: vi.fn(async (_data, mode) => {
      calls.push(`import:${mode}`)
    }),
    exportData: vi.fn(async () => {
      calls.push('export')
      return {
        config_sync_key: OLD_KEY,
        chat_user_config_1: { api_token: 'sk-local', sync_key: OLD_KEY },
        chat_data_user_x: { content: 'hi' },
      }
    }),
    upload: vi.fn(async (key: string) => {
      calls.push(`upload:${key}`)
    }),
    persistSyncKey: vi.fn(async (key: string) => {
      calls.push(`persist:${key}`)
    }),
    removeBackup: vi.fn(async (key: string) => {
      calls.push(`remove:${key}`)
    }),
    createKey: () => NEW_KEY,
    ...overrides,
  }
  return { deps, calls }
}

describe('rotateSyncKey', () => {
  it('merges, re-uploads under a new key, switches, then deletes the old backup', async () => {
    const { deps, calls } = fakeDeps()

    const result = await rotateSyncKey(OLD_KEY, deps)

    expect(result).toEqual({ newKey: NEW_KEY, oldBackupDeleted: true })
    expect(calls).toEqual([
      `download:${OLD_KEY}`,
      'import:merge',
      'export',
      `upload:${NEW_KEY}`,
      `persist:${NEW_KEY}`,
      `remove:${OLD_KEY}`,
    ])
  })

  it('binds the uploaded backup to the new key', async () => {
    const { deps } = fakeDeps()

    await rotateSyncKey(OLD_KEY, deps)

    const uploaded = vi.mocked(deps.upload).mock.calls[0][1]
    expect(uploaded.config_sync_key).toBe(NEW_KEY)
    expect(uploaded.chat_user_config_1).toEqual({
      api_token: 'sk-local',
      sync_key: NEW_KEY,
    })
    expect(uploaded.chat_data_user_x).toEqual({ content: 'hi' })
    expect(JSON.stringify(uploaded)).not.toContain(`"sync_key":"${OLD_KEY}"`)
  })

  it('skips the merge when the old backup is empty', async () => {
    const { deps, calls } = fakeDeps({
      download: vi.fn(async () => ({})),
    })

    await rotateSyncKey(OLD_KEY, deps)

    expect(calls).not.toContain('import:merge')
  })

  it('changes nothing locally and keeps the old backup when the upload fails', async () => {
    const { deps } = fakeDeps({
      upload: vi.fn(async () => {
        throw new Error('network down')
      }),
    })

    await expect(rotateSyncKey(OLD_KEY, deps)).rejects.toThrow('network down')
    expect(deps.persistSyncKey).not.toHaveBeenCalled()
    expect(deps.removeBackup).not.toHaveBeenCalled()
  })

  it('aborts before uploading when the old backup cannot be read', async () => {
    const { deps } = fakeDeps({
      download: vi.fn(async () => {
        throw new Error('503')
      }),
    })

    await expect(rotateSyncKey(OLD_KEY, deps)).rejects.toThrow('503')
    expect(deps.upload).not.toHaveBeenCalled()
  })

  it('reports an old backup it could not delete', async () => {
    const { deps } = fakeDeps({
      removeBackup: vi.fn(async () => {
        throw new Error('404 not found on an older server')
      }),
    })

    const result = await rotateSyncKey(OLD_KEY, deps)

    expect(result).toEqual({ newKey: NEW_KEY, oldBackupDeleted: false })
    expect(deps.persistSyncKey).toHaveBeenCalledWith(NEW_KEY)
  })
})

describe('rotateSyncKey when the new key cannot be stored', () => {
  it('deletes the unused new backup, keeps the old one, and rejects', async () => {
    const { deps, calls } = fakeDeps({
      persistSyncKey: vi.fn(async () => {
        throw new Error('QuotaExceededError')
      }),
    })

    await expect(rotateSyncKey(OLD_KEY, deps)).rejects.toThrow(
      'QuotaExceededError',
    )
    expect(calls).toEqual([
      `download:${OLD_KEY}`,
      'import:merge',
      'export',
      `upload:${NEW_KEY}`,
      `remove:${NEW_KEY}`,
    ])
  })
})
