/**
 * Data Sync section of the GPTChat settings sidebar: the sync key, cloud
 * upload/download, and sync key rotation.
 */
import { Download, Loader2, RefreshCw, ShieldAlert, Upload } from 'lucide-react'
import { useEffect, useState } from 'react'

import { Button } from '@/components/ui/button'
import { SecretField } from '@/components/ui/secret-field'
import { kvSet, StorageKeys } from '@/utils/storage'
import {
  findSessionsReusingSyncKey,
  isApiTokenOfAnySession,
} from '../hooks/session-credentials'
import type { SessionConfig } from '../types'
import { api } from '../utils/api'
import { maskSecretHint } from '../utils/api-token'
import {
  isSyncKeyReusedAsApiToken,
  validateCredentialUpdate,
} from '../utils/credential-policy'
import { rotateSyncKey } from '../utils/sync-key-rotation'

const SYNC_KEY_INPUT_ID = 'cfg-sync-key'
const SYNC_KEY_LABEL_ID = 'cfg-sync-key-label'

/**
 * SYNC_KEY_IS_API_TOKEN_MESSAGE explains why an API key cannot double as
 * the sync key.
 */
const SYNC_KEY_IS_API_TOKEN_MESSAGE =
  'That value is an API key of one of your sessions, so it was not saved as the sync key. Anyone who has the API key could read your cloud backup; choose a different sync key.'

/**
 * DataSyncManagerProps configures DataSyncManager: config is the active
 * session config, onConfigChange persists a partial update of it, and
 * onExportData / onImportData move all local data to and from the cloud.
 */
interface DataSyncManagerProps {
  config: SessionConfig
  onConfigChange: (updates: Partial<SessionConfig>) => void | Promise<void>
  onExportData: () => Promise<unknown>
  onImportData: (data: unknown, mode?: 'merge' | 'download') => Promise<void>
}

/**
 * useSyncKeyReuse reports whether syncKey is also the API token of the
 * active session (apiToken) or of any stored session. Such a key lets
 * whoever holds that API token read the cloud backup.
 * It returns true while the reuse persists.
 */
function useSyncKeyReuse(syncKey: string, apiToken: string): boolean {
  const [storedReuse, setStoredReuse] = useState(false)

  useEffect(() => {
    let cancelled = false
    if (!syncKey) return
    findSessionsReusingSyncKey(syncKey)
      .then(({ sessionIds }) => {
        if (!cancelled) setStoredReuse(sessionIds.length > 0)
      })
      .catch((error: unknown) => {
        console.error(`[DataSyncManager] reuse check failed: ${String(error)}`)
      })
    return () => {
      cancelled = true
    }
  }, [syncKey, apiToken])

  return (
    (!!syncKey && storedReuse) || isSyncKeyReusedAsApiToken(apiToken, syncKey)
  )
}

/**
 * DataSyncManager renders the sync key as a write-only field together with
 * the upload, download, and rotation actions. It returns the section.
 */
export function DataSyncManager({
  config,
  onConfigChange,
  onExportData,
  onImportData,
}: DataSyncManagerProps) {
  const [isSyncing, setIsSyncing] = useState(false)
  const syncKey = config.sync_key || ''
  const syncKeyReused = useSyncKeyReuse(syncKey, config.api_token)

  const handleSaveSyncKey = async (nextSyncKey: string) => {
    const { rejections } = validateCredentialUpdate(config, {
      sync_key: nextSyncKey,
    })
    if (rejections.length > 0 || (await isApiTokenOfAnySession(nextSyncKey))) {
      return SYNC_KEY_IS_API_TOKEN_MESSAGE
    }
    await onConfigChange({ sync_key: nextSyncKey })
  }

  const handleRotate = async () => {
    if (
      !window.confirm(
        'Rotate your sync key? Your cloud backup is merged into this device, uploaded under a new random key, and the copy under the old key is deleted. Afterwards, enter the new key on your other devices.',
      )
    ) {
      return
    }

    setIsSyncing(true)
    try {
      const { newKey, oldBackupDeleted } = await rotateSyncKey(syncKey, {
        download: (key) => api.downloadUserData(key),
        upload: (key, data) => api.uploadUserData(key, data),
        removeBackup: (key) => api.deleteUserData(key),
        importData: (data, mode) => onImportData(data, mode),
        exportData: onExportData,
        persistSyncKey: (key) => kvSet(StorageKeys.SYNC_KEY, key),
      })
      alert(
        `Your new sync key (${maskSecretHint(newKey)}) is saved and your backup moved to it. Copy it from Settings → Data Sync and enter it on your other devices.` +
          (oldBackupDeleted
            ? ''
            : ' The backup under the old key could not be deleted; try again later.'),
      )
      window.location.reload()
    } catch (err) {
      console.error(err)
      alert(
        'Rotating the sync key failed, so it was left unchanged. See console for details.',
      )
    } finally {
      setIsSyncing(false)
    }
  }

  const handleDownload = async () => {
    if (!config.sync_key) {
      alert('Please enter a Sync Key to download.')
      return
    }

    setIsSyncing(true)
    try {
      const cloudData = await api.downloadUserData(config.sync_key)
      if (!cloudData || Object.keys(cloudData).length === 0) {
        alert('No data found in cloud.')
        return
      }

      await onImportData(cloudData, 'download')
      alert('Download successful!')
      window.location.reload()
    } catch (err) {
      console.error(err)
      alert('Download failed. See console for details.')
    } finally {
      setIsSyncing(false)
    }
  }

  const handleUpload = async () => {
    if (!config.sync_key) {
      alert('Please enter a Sync Key to upload.')
      return
    }

    setIsSyncing(true)
    try {
      const data = await onExportData()
      await api.uploadUserData(config.sync_key, data)
      alert('Upload successful!')
    } catch (err) {
      console.error(err)
      alert('Upload failed. See console for details.')
    } finally {
      setIsSyncing(false)
    }
  }

  return (
    <div>
      <label className="mb-2 block text-sm font-medium">Data Sync</label>

      <div className="mb-2">
        <label
          id={SYNC_KEY_LABEL_ID}
          htmlFor={SYNC_KEY_INPUT_ID}
          className="mb-1 block text-xs text-muted-foreground"
        >
          Sync Key
        </label>
        <SecretField
          inputId={SYNC_KEY_INPUT_ID}
          name="gptchat-sync-key"
          label="sync key"
          labelId={SYNC_KEY_LABEL_ID}
          value={syncKey}
          hint={maskSecretHint(syncKey)}
          onSave={handleSaveSyncKey}
          placeholder="Paste the sync key of your other device"
          className="text-xs"
          iconClassName="h-3.5 w-3.5"
        />
        {syncKeyReused && (
          <div
            role="alert"
            className="mt-2 space-y-2 rounded-md border border-destructive p-2 text-xs text-destructive"
          >
            <p className="flex items-start gap-1.5">
              <ShieldAlert
                className="mt-0.5 h-3.5 w-3.5 shrink-0"
                aria-hidden
              />
              <span>
                Your sync key is also an API key. Anyone who has that API key,
                including its provider, could download and decrypt your cloud
                backup. Rotate the sync key: your backup moves to a new random
                key and the old copy is deleted.
              </span>
            </p>
            <Button
              variant="outline"
              size="sm"
              className="gap-2"
              onClick={handleRotate}
              disabled={isSyncing}
            >
              <RefreshCw className="h-3.5 w-3.5" aria-hidden />
              Rotate sync key
            </Button>
          </div>
        )}
      </div>

      <div className="flex gap-2">
        <Button
          variant="outline"
          size="sm"
          className="flex-1 gap-2"
          onClick={handleDownload}
          disabled={isSyncing}
        >
          {isSyncing ? (
            <Loader2 className="h-4 w-4 animate-spin" />
          ) : (
            <Download className="h-4 w-4" />
          )}
          Download
        </Button>
        <Button
          variant="outline"
          size="sm"
          className="flex-1 gap-2"
          onClick={handleUpload}
          disabled={isSyncing}
        >
          {isSyncing ? (
            <Loader2 className="h-4 w-4 animate-spin" />
          ) : (
            <Upload className="h-4 w-4" />
          )}
          Upload
        </Button>
      </div>
      <p className="mt-1 text-xs text-muted-foreground">
        Download overwrites configs and merges messages for matched sessions.
      </p>
    </div>
  )
}
