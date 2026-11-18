/**
 * In-memory stand-in for '@/utils/storage' shared by GPTChat tests.
 *
 * Test files mock the storage module with this file:
 *
 *   vi.mock('@/utils/storage', () => import('<path>/fixtures/storage-mock'))
 *
 * and call installStore to back the mocked functions with a plain record, so
 * each test can seed and inspect exactly what is persisted. This module must
 * not import application code: the mock factory loads it while application
 * modules are still being evaluated.
 */
import { type Mock, vi } from 'vitest'

/**
 * StorageKeys mirrors the production storage keys the GPTChat config layer uses.
 */
export const StorageKeys = {
  SESSION_CONFIG_PREFIX: 'chat_user_config_',
  SELECTED_SESSION: 'config_selected_session',
  VERSION_DATE: 'config_version_date',
  SESSION_HISTORY_PREFIX: 'chat_user_session_',
  SESSION_ORDER: 'config_session_order',
  SYNC_KEY: 'config_sync_key',
  CHAT_DATA_PREFIX: 'chat_data_',
  DELETED_CHAT_IDS: 'chat_deleted_chat_ids',
  PENDING_BACKUP_DELETIONS: 'config_pending_backup_deletions',
} as const

export const kvGet: Mock = vi.fn()
export const kvSet: Mock = vi.fn()
export const kvDel: Mock = vi.fn()
export const kvList: Mock = vi.fn()

/**
 * installStore backs the mocked KV functions with store, so writes are
 * visible to later reads and to the test. It returns store for chaining.
 */
export function installStore(
  store: Record<string, unknown>,
): Record<string, unknown> {
  kvGet.mockImplementation((key: string) => Promise.resolve(store[key] ?? null))
  kvSet.mockImplementation(async (key: string, value: unknown) => {
    store[key] = value
  })
  kvDel.mockImplementation(async (key: string) => {
    delete store[key]
  })
  kvList.mockImplementation(async () => Object.keys(store))
  return store
}
