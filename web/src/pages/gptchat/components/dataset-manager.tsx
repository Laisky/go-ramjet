import { Loader2, Trash2 } from 'lucide-react'
import { useCallback, useEffect, useMemo, useState } from 'react'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { kvGet, kvSet, StorageKeys } from '@/utils/storage'
import type { SessionConfig } from '../types'
import { api } from '../utils/api'

interface DatasetManagerProps {
  config: SessionConfig
}

export function DatasetManager({ config }: DatasetManagerProps) {
  const [datasetKey, setDatasetKey] = useState('')
  const [datasetName, setDatasetName] = useState('')
  const [datasetFile, setDatasetFile] = useState<File | null>(null)
  const [datasets, setDatasets] = useState<
    Array<{ name: string; taskStatus?: string; progress?: number }>
  >([])
  const [chatbots, setChatbots] = useState<string[]>([])
  const [activeChatbot, setActiveChatbot] = useState<string | undefined>(
    undefined,
  )
  const [isDatasetLoading, setIsDatasetLoading] = useState(false)
  const [datasetError, setDatasetError] = useState<string | null>(null)

  const randomString = useCallback((length = 16) => {
    const chars = 'abcdefghijklmnopqrstuvwxyz0123456789'
    let result = ''
    for (let i = 0; i < length; i += 1) {
      result += chars.charAt(Math.floor(Math.random() * chars.length))
    }
    return result
  }, [])

  const handleDatasetKeyChange = useCallback(async (value: string) => {
    setDatasetKey(value)
    await kvSet(StorageKeys.CUSTOM_DATASET_PASSWORD, value)
  }, [])

  const refreshDatasets = useCallback(async () => {
    if (!datasetKey || !config.api_token) return
    setIsDatasetLoading(true)
    setDatasetError(null)
    try {
      const resp = await api.listDatasets(
        datasetKey,
        config.api_token,
        config.api_base,
      )
      const list = resp.datasets || []
      setDatasets(list)
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err)
      setDatasetError(msg)
    } finally {
      setIsDatasetLoading(false)
    }
  }, [config.api_base, config.api_token, datasetKey])

  const refreshChatbots = useCallback(async () => {
    if (!datasetKey || !config.api_token) return
    setIsDatasetLoading(true)
    setDatasetError(null)
    try {
      const resp = await api.listChatbots(
        datasetKey,
        config.api_token,
        config.api_base,
      )
      setChatbots(resp.chatbots || [])
      setActiveChatbot(resp.current)
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err)
      setDatasetError(msg)
    } finally {
      setIsDatasetLoading(false)
    }
  }, [config.api_base, config.api_token, datasetKey])

  const handleUploadDataset = useCallback(async () => {
    const trimmedName = String(datasetName || '').trim()
    if (!datasetFile || !trimmedName) {
      alert('Choose a file and dataset name first.')
      return
    }
    setIsDatasetLoading(true)
    setDatasetError(null)
    try {
      await api.uploadDataset(
        datasetFile,
        trimmedName,
        datasetKey,
        config.api_token,
        config.api_base,
      )
      await refreshDatasets()
      alert('Upload succeeded. Processing may take a few minutes.')
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err)
      setDatasetError(msg)
      alert(`Upload failed: ${msg}`)
    } finally {
      setIsDatasetLoading(false)
    }
  }, [
    config.api_base,
    config.api_token,
    datasetFile,
    datasetKey,
    datasetName,
    refreshDatasets,
  ])

  const handleDeleteDataset = useCallback(
    async (name: string) => {
      setIsDatasetLoading(true)
      setDatasetError(null)
      try {
        await api.deleteDataset(
          name,
          datasetKey,
          config.api_token,
          config.api_base,
        )
        setDatasets((prev) => prev.filter((d) => d.name !== name))
      } catch (err) {
        const msg = err instanceof Error ? err.message : String(err)
        setDatasetError(msg)
        alert(`Delete failed: ${msg}`)
      } finally {
        setIsDatasetLoading(false)
      }
    },
    [config.api_base, config.api_token, datasetKey],
  )

  const handleSetActiveChatbot = useCallback(
    async (name: string) => {
      setIsDatasetLoading(true)
      setDatasetError(null)
      try {
        await api.setActiveChatbot(
          datasetKey,
          name,
          config.api_token,
          config.api_base,
        )
        setActiveChatbot(name)
      } catch (err) {
        const msg = err instanceof Error ? err.message : String(err)
        setDatasetError(msg)
        alert(`Activate failed: ${msg}`)
      } finally {
        setIsDatasetLoading(false)
      }
    },
    [config.api_base, config.api_token, datasetKey],
  )

  useEffect(() => {
    let mounted = true
    ;(async () => {
      try {
        const stored = await kvGet<string>(StorageKeys.CUSTOM_DATASET_PASSWORD)
        const key = stored && stored.length > 0 ? stored : randomString(16)
        if (!stored) {
          await kvSet(StorageKeys.CUSTOM_DATASET_PASSWORD, key)
        }
        if (mounted) {
          setDatasetKey(key)
        }
      } catch (err) {
        console.warn('Failed to load dataset key', err)
      }
    })()
    return () => {
      mounted = false
    }
  }, [randomString])

  useEffect(() => {
    if (!datasetKey) return
    refreshDatasets()
  }, [datasetKey, refreshDatasets])

  const acceptFileTypes = useMemo(() => '.pdf,.md,.ppt,.pptx,.doc,.docx', [])

  return (
    <div className="space-y-3 rounded-lg border border-border p-3">
      <div className="flex items-center justify-between">
        <label className="text-sm font-medium">
          Private Dataset (PDF Chat)
        </label>
        {isDatasetLoading && <Loader2 className="h-4 w-4 animate-spin" />}
      </div>
      <div className="space-y-2">
        <div>
          <label className="mb-1 block text-xs text-muted-foreground">
            Dataset Key (keeps uploads private)
          </label>
          <Input
            type="text"
            value={datasetKey}
            onChange={(e) => handleDatasetKeyChange(e.target.value)}
            className="text-xs"
            placeholder="dataset-key"
          />
        </div>
        <div className="flex gap-2">
          <Input
            type="text"
            value={datasetName}
            onChange={(e) => setDatasetName(e.target.value)}
            placeholder="Dataset name"
            className="text-xs"
          />
          <Input
            type="file"
            accept={acceptFileTypes}
            className="text-xs"
            onChange={(e) => {
              const file = e.target.files?.[0] || null
              setDatasetFile(file)
              if (file) {
                const base = file.name.replace(/\.[^.]+$/, '')
                setDatasetName(base.replace(/[^a-zA-Z0-9]/g, '_'))
              }
            }}
          />
        </div>
        <div className="flex gap-2">
          <Button
            size="sm"
            variant="outline"
            className="flex-1"
            onClick={handleUploadDataset}
            disabled={isDatasetLoading}
          >
            Upload Dataset
          </Button>
          <Button
            size="sm"
            variant="outline"
            className="flex-1"
            onClick={refreshDatasets}
            disabled={isDatasetLoading}
          >
            Refresh Datasets
          </Button>
          <Button
            size="sm"
            variant="outline"
            className="flex-1"
            onClick={refreshChatbots}
            disabled={isDatasetLoading}
          >
            List Bots
          </Button>
        </div>
        {datasetError && (
          <p className="text-xs text-destructive">{datasetError}</p>
        )}
      </div>

      {datasets.length > 0 && (
        <div className="space-y-2">
          <div className="text-xs font-semibold text-foreground">Datasets</div>
          <div className="space-y-2">
            {datasets.map((ds) => (
              <div
                key={ds.name}
                className="flex items-center justify-between rounded border border-border p-2 text-sm"
              >
                <div>
                  <div className="font-medium">{ds.name}</div>
                  {ds.taskStatus && (
                    <div className="text-xs text-muted-foreground">
                      {ds.taskStatus}
                      {typeof ds.progress === 'number' &&
                        ` • ${Math.round(ds.progress)}%`}
                    </div>
                  )}
                </div>
                <Button
                  size="sm"
                  variant="ghost"
                  onClick={() => handleDeleteDataset(ds.name)}
                  disabled={isDatasetLoading}
                >
                  <Trash2 className="h-4 w-4" />
                </Button>
              </div>
            ))}
          </div>
        </div>
      )}

      {chatbots.length > 0 && (
        <div className="space-y-2">
          <div className="text-xs font-semibold text-foreground">Chatbots</div>
          <div className="space-y-1">
            {chatbots.map((bot) => (
              <label
                key={bot}
                className="flex items-center justify-between rounded border border-border p-2 text-sm"
              >
                <span>{bot}</span>
                <input
                  type="radio"
                  name="chatbot"
                  checked={activeChatbot === bot}
                  onChange={() => handleSetActiveChatbot(bot)}
                />
              </label>
            ))}
          </div>
        </div>
      )}

      <p className="text-xs text-muted-foreground">
        Upload PDFs/office docs to build a private dataset, then pick a chatbot
        to talk with it. Processing may take a few minutes.
      </p>
    </div>
  )
}
