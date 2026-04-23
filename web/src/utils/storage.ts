/**
 * Public facade for key-value storage. Wraps the native IndexedDB backend in
 * `./storage/idb-kv.ts` and runs a one-time migration from the legacy
 * PouchDB store on first use.
 *
 * Every exported symbol here matches the pre-migration PouchDB API exactly
 * so existing callers remain drop-in compatible.
 */
import {
  kvAddListener as _kvAddListener,
  kvClear as _kvClear,
  kvDel as _kvDel,
  kvEstimate as _kvEstimate,
  kvExists as _kvExists,
  kvGet as _kvGet,
  kvList as _kvList,
  kvRemoveListener as _kvRemoveListener,
  kvRename as _kvRename,
  kvSet as _kvSet,
  type KvListener,
  type KvOperation as _KvOperation,
} from './storage/idb-kv'
import { migrateFromPouchDB } from './storage/migration'

// Storage key constants — must match legacy keys from chat.js
export const StorageKeys = {
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
  CHAT_DATA_PREFIX: 'chat_data_', // ${prefix}${role}_${chatID}
  DELETED_CHAT_IDS: 'deleted_chat_ids',
} as const

export type KvOperation = _KvOperation

// ---------------------------------------------------------------------------
// Migration bootstrap — run exactly once, cached across callers.
// ---------------------------------------------------------------------------

let migrationPromise: Promise<void> | null = null

function ensureMigrated(): Promise<void> {
  if (!migrationPromise) {
    migrationPromise = migrateFromPouchDB().catch((err) => {
      console.error(
        'PouchDB -> IndexedDB migration failed; continuing with empty store',
        err,
      )
    })
  }
  return migrationPromise
}

// Test-only: drop the cached migration promise so the next facade call
// re-runs migration. Not part of the public API.
export function __resetMigrationForTests(): void {
  migrationPromise = null
}

// ---------------------------------------------------------------------------
// Public API — each method awaits ensureMigrated() before delegating.
// ---------------------------------------------------------------------------

export async function kvSet<T>(key: string, val: T): Promise<void> {
  await ensureMigrated()
  return _kvSet(key, val)
}

export async function kvGet<T>(key: string): Promise<T | null> {
  await ensureMigrated()
  return _kvGet<T>(key)
}

export async function kvExists(key: string): Promise<boolean> {
  await ensureMigrated()
  return _kvExists(key)
}

export async function kvRename(oldKey: string, newKey: string): Promise<void> {
  await ensureMigrated()
  return _kvRename(oldKey, newKey)
}

export async function kvDel(key: string): Promise<void> {
  await ensureMigrated()
  return _kvDel(key)
}

export async function kvList(): Promise<string[]> {
  await ensureMigrated()
  return _kvList()
}

export async function kvClear(): Promise<void> {
  await ensureMigrated()
  return _kvClear()
}

export async function kvAddListener(
  keyPrefix: string,
  callback: KvListener,
  callbackName?: string,
): Promise<void> {
  await ensureMigrated()
  return _kvAddListener(keyPrefix, callback, callbackName)
}

export function kvRemoveListener(
  keyPrefix: string,
  callbackName: string,
): void {
  // Synchronous — matches legacy signature. Listener registry lives in
  // idb-kv; no migration dependency here.
  _kvRemoveListener(keyPrefix, callbackName)
}

export async function kvEstimate(): Promise<{
  usage: number
  quota: number
} | null> {
  await ensureMigrated()
  return _kvEstimate()
}
