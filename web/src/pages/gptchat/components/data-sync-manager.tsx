import { Eye, EyeOff, Loader2, RefreshCw } from 'lucide-react'
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
}

export function DataSyncManager({
  config,
  onConfigChange,
  onExportData,
  onImportData,
}: DataSyncManagerProps) {
  const [isSyncing, setIsSyncing] = useState(false)
  const [showSyncKey, setShowSyncKey] = useState(false)

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
            type={showSyncKey ? 'text' : 'password'}
            value={config.sync_key || ''}
            onChange={(e) => onConfigChange({ sync_key: e.target.value })}
            placeholder="sync-..."
            className="pr-10 text-xs"
          />
          <button
            type="button"
            onClick={() => setShowSyncKey(!showSyncKey)}
            className="absolute right-0 top-0 flex h-full items-center px-3 text-muted-foreground hover:text-foreground"
          >
            {showSyncKey ? (
              <EyeOff className="h-3.5 w-3.5" />
            ) : (
              <Eye className="h-3.5 w-3.5" />
            )}
          </button>
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
