/**
 * Sync key rotation: moves the cloud backup from one sync key to a fresh
 * one without losing data, then removes the backup the old key unlocks.
 */
import { StorageKeys } from '@/utils/storage'
import { createSyncKey } from './api-token'

/**
 * SyncKeyRotationDeps are the side effects rotateSyncKey needs: cloud
 * download, upload, and delete by sync key, local import and export, and
 * persisting the new key locally. createKey makes the new key.
 */
export interface SyncKeyRotationDeps {
  download: (syncKey: string) => Promise<unknown>
  upload: (syncKey: string, data: Record<string, unknown>) => Promise<void>
  removeBackup: (syncKey: string) => Promise<void>
  importData: (
    data: Record<string, unknown>,
    mode: 'merge' | 'download',
  ) => Promise<void>
  exportData: () => Promise<unknown>
  persistSyncKey: (syncKey: string) => Promise<void>
  createKey?: () => string
}

/**
 * SyncKeyRotationResult reports the new key and whether the old cloud
 * backup was deleted. Deletion can fail, for example against an older
 * server, and the old key then still unlocks that backup.
 */
export interface SyncKeyRotationResult {
  newKey: string
  oldBackupDeleted: boolean
}

/**
 * rebindExport points an export at newKey: the browser-wide sync key and
 * every session config's copy of it. Another device that downloads it then
 * adopts newKey instead of reverting to the old one. It returns the patched
 * export.
 */
function rebindExport(data: unknown, newKey: string): Record<string, unknown> {
  const source = data && typeof data === 'object' ? data : {}
  const patched: Record<string, unknown> = {}
  for (const [key, value] of Object.entries(source)) {
    patched[key] =
      key.startsWith(StorageKeys.SESSION_CONFIG_PREFIX) &&
      value &&
      typeof value === 'object'
        ? { ...value, sync_key: newKey }
        : value
  }
  patched[StorageKeys.SYNC_KEY] = newKey
  return patched
}

/**
 * rotateSyncKey replaces oldKey without stranding data. Each step only
 * starts once the previous one has succeeded:
 *   1. merge the cloud backup under oldKey into local data, so nothing that
 *      exists only in the cloud is lost;
 *   2. upload everything under a fresh key; if this fails, nothing local
 *      has changed and the old backup is intact;
 *   3. persist the fresh key locally;
 *   4. delete the old backup, best effort.
 * It returns the new key and whether step 4 succeeded. It rejects if any of
 * steps 1-3 fails.
 */
export async function rotateSyncKey(
  oldKey: string,
  deps: SyncKeyRotationDeps,
): Promise<SyncKeyRotationResult> {
  const cloud = await deps.download(oldKey)
  if (cloud && typeof cloud === 'object' && Object.keys(cloud).length > 0) {
    await deps.importData(cloud as Record<string, unknown>, 'merge')
  }

  const newKey = (deps.createKey ?? createSyncKey)()
  await deps.upload(newKey, rebindExport(await deps.exportData(), newKey))
  await deps.persistSyncKey(newKey)

  let oldBackupDeleted = true
  try {
    await deps.removeBackup(oldKey)
  } catch (error) {
    oldBackupDeleted = false
    console.warn(
      `[sync-key-rotation] failed to delete the old cloud backup: ${String(error)}`,
    )
  }
  return { newKey, oldBackupDeleted }
}
