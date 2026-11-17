import { Download, Loader2, Upload } from 'lucide-react'
import { useState } from 'react'

import { Button } from '@/components/ui/button'
import { SecretInput } from '@/components/ui/secret-input'
import type { SessionConfig } from '../types'
import { api } from '../utils/api'

interface DataSyncManagerProps {
  config: SessionConfig
  onConfigChange: (updates: Partial<SessionConfig>) => void
  onExportData: () => Promise<unknown>
  onImportData: (data: unknown, mode?: 'merge' | 'download') => Promise<void>
}

export function DataSyncManager({
  config,
  onConfigChange,
  onExportData,
  onImportData,
}: DataSyncManagerProps) {
  const [isSyncing, setIsSyncing] = useState(false)

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
          htmlFor="cfg-sync-key"
          className="mb-1 block text-xs text-muted-foreground"
        >
          Sync Key
        </label>
        <SecretInput
          id="cfg-sync-key"
          name="gptchat-sync-key"
          label="sync key"
          value={config.sync_key || ''}
          onValueChange={(syncKey) => onConfigChange({ sync_key: syncKey })}
          placeholder="sync-..."
          className="text-xs"
          iconClassName="h-3.5 w-3.5"
        />
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
