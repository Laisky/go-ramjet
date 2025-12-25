import { CloudDownload, CloudUpload, Loader2 } from 'lucide-react'
import { useState } from 'react'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import type { SessionConfig } from '../types'
import { api } from '../utils/api'

interface DataSyncManagerProps {
  config: SessionConfig
  onConfigChange: (updates: Partial<SessionConfig>) => void
  onExportData: () => Promise<unknown>
  onImportData: (data: unknown) => Promise<void>
  showApiKey: boolean
}

export function DataSyncManager({
  config,
  onConfigChange,
  onExportData,
  onImportData,
  showApiKey,
}: DataSyncManagerProps) {
  const [isSyncing, setIsSyncing] = useState(false)

  const handleUpload = async () => {
    if (!config.sync_key) {
      alert('Please enter a Sync Key to sync.')
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

  const handleDownload = async () => {
    if (!config.sync_key) {
      alert('Please enter a Sync Key to sync.')
      return
    }

    if (!confirm('This will overwrite your local data. Continue?')) {
      return
    }

    setIsSyncing(true)
    try {
      const data = await api.downloadUserData(config.sync_key)
      await onImportData(data)
      alert('Download and restore successful!')
    } catch (err) {
      console.error(err)
      alert('Download failed. See console for details.')
    } finally {
      setIsSyncing(false)
    }
  }

  return (
    <div>
      <label className="mb-2 block text-sm font-medium">Data Sync</label>

      <div className="mb-2">
        <label className="mb-1 block text-xs text-muted-foreground">
          Sync Key
        </label>
        <div className="relative">
          <Input
            type={showApiKey ? 'text' : 'password'}
            value={config.sync_key || ''}
            onChange={(e) => onConfigChange({ sync_key: e.target.value })}
            placeholder="sync-..."
            className="pr-10 text-xs"
          />
        </div>
      </div>

      <div className="flex gap-2">
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
            <CloudUpload className="h-4 w-4" />
          )}
          Upload
        </Button>
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
            <CloudDownload className="h-4 w-4" />
          )}
          Download
        </Button>
      </div>
      <p className="mt-1 text-xs text-muted-foreground">
        Sync your settings and chat history using this key. Keep it safe!
      </p>
    </div>
  )
}
