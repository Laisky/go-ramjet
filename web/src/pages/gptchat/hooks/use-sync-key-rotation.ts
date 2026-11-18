/**
 * Sync key rotation wired to the GPTChat config hook and the cloud API, and
 * the device-local list of old cloud backups that still have to be deleted.
 *
 * Rotation deletes the backup under the old key last. That delete can fail
 * (network, an older server), and afterwards no UI holds the old key any
 * more. The old key is therefore recorded before the delete and forgotten
 * only once the backup is gone, so the cleanup can always be retried, even
 * after a closed tab. The record never leaves this device (see
 * EXPORT_EXCLUDED_KEYS and IMPORT_SKIPPED_KEYS in data-sync.ts).
 */
import { useCallback, useEffect, useState } from 'react'

import { kvGet, kvSet, StorageKeys } from '@/utils/storage'
import { api } from '../utils/api'
import type { ConfigUpdateHandler } from '../utils/credential-policy'
import {
  rotateSyncKey,
  type SyncKeyRotationResult,
} from '../utils/sync-key-rotation'

/**
 * readPendingBackupDeletions returns the sync keys whose cloud backup still
 * has to be deleted, oldest first.
 */
export async function readPendingBackupDeletions(): Promise<string[]> {
  const stored = await kvGet<unknown>(StorageKeys.PENDING_BACKUP_DELETIONS)
  return Array.isArray(stored)
    ? stored.filter((key): key is string => typeof key === 'string' && !!key)
    : []
}

/**
 * deleteBackupRecorded deletes the cloud backup syncKey unlocks. The key is
 * recorded as pending first and dropped from the record only after the
 * delete succeeded. It rejects when the delete fails, leaving the key
 * recorded for a retry.
 */
export async function deleteBackupRecorded(syncKey: string): Promise<void> {
  const pending = await readPendingBackupDeletions()
  if (!pending.includes(syncKey)) {
    await kvSet(StorageKeys.PENDING_BACKUP_DELETIONS, [...pending, syncKey])
  }

  await api.deleteUserData(syncKey)

  const remaining = (await readPendingBackupDeletions()).filter(
    (key) => key !== syncKey,
  )
  await kvSet(StorageKeys.PENDING_BACKUP_DELETIONS, remaining)
}

/**
 * SyncKeyRotationOptions wires useSyncKeyRotation to the config hook:
 * exportData and importData move all local data to and from a backup, and
 * updateConfig stores the new sync key.
 */
export interface SyncKeyRotationOptions {
  exportData: () => Promise<unknown>
  importData: (
    data: Record<string, unknown>,
    mode: 'merge' | 'download',
  ) => Promise<void>
  updateConfig: ConfigUpdateHandler
}

/**
 * useSyncKeyRotation returns rotate(oldKey), which moves the cloud backup
 * from oldKey to a new random key (see rotateSyncKey). The new key is
 * stored through updateConfig, so it reaches every session config, and the
 * old backup is deleted through deleteBackupRecorded, so a failed delete
 * can be retried. rotate resolves with the new key and whether the old
 * backup is gone, and rejects when the key could not be changed.
 */
export function useSyncKeyRotation({
  exportData,
  importData,
  updateConfig,
}: SyncKeyRotationOptions): (oldKey: string) => Promise<SyncKeyRotationResult> {
  return useCallback(
    (oldKey: string) =>
      rotateSyncKey(oldKey, {
        download: (key) => api.downloadUserData(key),
        upload: (key, data) => api.uploadUserData(key, data),
        removeBackup: deleteBackupRecorded,
        importData,
        exportData,
        persistSyncKey: async (key) => {
          const { rejections } = await updateConfig({ sync_key: key })
          if (rejections.length > 0) {
            throw new Error(
              `the new sync key was refused: ${rejections.map((r) => r.reason).join(',')}`,
            )
          }
        },
      }),
    [exportData, importData, updateConfig],
  )
}

/**
 * usePendingBackupDeletions lists the sync keys whose cloud backup still
 * has to be deleted, and returns retry(), which deletes each of them. retry
 * rejects with the first failure; keys deleted before it stay deleted.
 */
export function usePendingBackupDeletions(): {
  pending: string[]
  retry: () => Promise<void>
} {
  const [pending, setPending] = useState<string[]>([])
  const [generation, setGeneration] = useState(0)

  useEffect(() => {
    let cancelled = false
    readPendingBackupDeletions()
      .then((keys) => {
        if (!cancelled) setPending(keys)
      })
      .catch((error: unknown) => {
        console.error(
          `[usePendingBackupDeletions] reading the cleanup list failed: ${String(error)}`,
        )
      })
    return () => {
      cancelled = true
    }
  }, [generation])

  const retry = useCallback(async () => {
    try {
      for (const key of await readPendingBackupDeletions()) {
        await deleteBackupRecorded(key)
      }
    } finally {
      setGeneration((n) => n + 1)
    }
  }, [])

  return { pending, retry }
}
