import { Loader2, RefreshCw } from 'lucide-react'
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

  const handleSync = async () => {
    if (!config.sync_key) {
      alert('Please enter a Sync Key to sync.')
      return
    }

    setIsSyncing(true)
    try {
      const cloudData = await api.downloadUserData(config.sync_key)
      await onImportData(cloudData)

      const merged = await onExportData()
      await api.uploadUserData(config.sync_key, merged)

      alert('Sync successful!')
      window.location.reload()
    } catch (err) {
      console.error(err)
      alert('Sync failed. See console for details.')
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
          onClick={handleSync}
          disabled={isSyncing}
        >
          {isSyncing ? (
            <Loader2 className="h-4 w-4 animate-spin" />
          ) : (
            <RefreshCw className="h-4 w-4" />
          )}
          Sync with Cloud
        </Button>
      </div>
      <p className="mt-1 text-xs text-muted-foreground">
        Sync downloads first (merge), then uploads.
      </p>
    </div>
  )
}
