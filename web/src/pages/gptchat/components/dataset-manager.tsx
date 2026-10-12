import { ChevronDown, ChevronUp, Loader2, Trash2 } from 'lucide-react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { kvGet, kvSet, StorageKeys } from '@/utils/storage'
import type { SessionConfig } from '../types'
import { api } from '../utils/api'
import {
  clearOwnershipSession,
  consumeOwnershipSession,
  GPTCHAT_SSO_STORAGE_KEY,
  ownershipSignInURL,
  readOwnershipSession,
} from '../utils/ownership-session'

interface DatasetManagerProps {
  config: SessionConfig
}

/** DatasetManager keeps the private account session separate from model configuration. */
export function DatasetManager({ config }: DatasetManagerProps) {
  const [sessionToken, setSessionToken] = useState(consumeOwnershipSession)
  useEffect(() => {
    const onStorage = (event: StorageEvent) => {
      if (event.key === GPTCHAT_SSO_STORAGE_KEY || event.key === null) {
        setSessionToken(readOwnershipSession())
      }
    }
    window.addEventListener('storage', onStorage)
    return () => window.removeEventListener('storage', onStorage)
  }, [])
  return (
    <DatasetManagerContent
      key={JSON.stringify([sessionToken, config.api_token, config.api_base])}
      config={config}
      sessionToken={sessionToken}
      onSignOut={() => {
        clearOwnershipSession()
        setSessionToken(null)
      }}
    />
  )
}

/** DatasetManagerContent isolates pending responses and displayed data to its account and model key. */
function DatasetManagerContent({
  config,
  sessionToken,
  onSignOut,
}: DatasetManagerProps & {
  sessionToken: string | null
  onSignOut: () => void
}) {
  const [isExpanded, setIsExpanded] = useState(false)
  const [datasetKey, setDatasetKey] = useState('')
  const pendingRequests = useRef(new Set<AbortController>())
  const beginRequest = useCallback(() => {
    const controller = new AbortController()
    pendingRequests.current.add(controller)
    return controller
  }, [])
  useEffect(() => {
    const requests = pendingRequests.current
    return () => {
      for (const controller of requests) controller.abort()
      requests.clear()
    }
  }, [datasetKey])
  const [datasetName, setDatasetName] = useState('')
  const [datasetFile, setDatasetFile] = useState<File | null>(null)
  const [datasets, setDatasets] = useState<
    Array<{ name: string; taskStatus?: string; progress?: number }>
  >([])
  const [chatbots, setChatbots] = useState<string[]>([])
  const [chatbotRequestKey, setChatbotRequestKey] = useState<string | null>(
    null,
  )
  const [activeChatbot, setActiveChatbot] = useState<string | undefined>(
    undefined,
  )
  const [isDatasetMutationLoading, setIsDatasetLoading] = useState(false)
  const requestKey = JSON.stringify([
    datasetKey,
    config.api_token,
    config.api_base,
    sessionToken,
  ])
  const [loadedRequestKey, setLoadedRequestKey] = useState<string | null>(null)
  const isDatasetLoading =
    isDatasetMutationLoading ||
    Boolean(
      datasetKey &&
      config.api_token &&
      sessionToken &&
      loadedRequestKey !== requestKey,
    )
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
    setIsDatasetLoading(false)
    setDatasetError(null)
    await kvSet(StorageKeys.CUSTOM_DATASET_PASSWORD, {
      data: value,
      updated_at: Date.now(),
    })
  }, [])

  const refreshDatasets = useCallback(async () => {
    if (!datasetKey || !sessionToken) return
    const controller = beginRequest()
    setIsDatasetLoading(true)
    setDatasetError(null)
    try {
      const resp = await api.listDatasets(
        datasetKey,
        config.api_token,
        config.api_base,
        sessionToken || undefined,
        controller.signal,
      )
      if (controller.signal.aborted) return
      const list = resp.datasets || []
      setDatasets(list)
    } catch (err) {
      if (controller.signal.aborted) return
      const msg = err instanceof Error ? err.message : String(err)
      setDatasetError(msg)
    } finally {
      pendingRequests.current.delete(controller)
      if (!controller.signal.aborted) setIsDatasetLoading(false)
    }
  }, [
    config.api_base,
    config.api_token,
    datasetKey,
    sessionToken,
    beginRequest,
  ])

  const refreshChatbots = useCallback(async () => {
    if (!datasetKey || !sessionToken) return
    const controller = beginRequest()
    setIsDatasetLoading(true)
    setDatasetError(null)
    try {
      const resp = await api.listChatbots(
        datasetKey,
        config.api_token,
        config.api_base,
        sessionToken || undefined,
        controller.signal,
      )
      if (controller.signal.aborted) return
      setChatbots(resp.chatbots || [])
      setChatbotRequestKey(requestKey)
      setActiveChatbot(resp.current)
    } catch (err) {
      if (controller.signal.aborted) return
      const msg = err instanceof Error ? err.message : String(err)
      setDatasetError(msg)
    } finally {
      pendingRequests.current.delete(controller)
      if (!controller.signal.aborted) setIsDatasetLoading(false)
    }
  }, [
    config.api_base,
    config.api_token,
    datasetKey,
    sessionToken,
    beginRequest,
    requestKey,
  ])

  const handleUploadDataset = useCallback(async () => {
    const trimmedName = String(datasetName || '').trim()
    if (!datasetFile || !trimmedName) {
      alert('Choose a file and dataset name first.')
      return
    }
    const controller = beginRequest()
    setIsDatasetLoading(true)
    setDatasetError(null)
    try {
      await api.uploadDataset(
        datasetFile,
        trimmedName,
        datasetKey,
        config.api_token,
        config.api_base,
        sessionToken || undefined,
        controller.signal,
      )
      if (controller.signal.aborted) return
      await refreshDatasets()
      if (controller.signal.aborted) return
      alert('Upload succeeded. Processing may take a few minutes.')
    } catch (err) {
      if (controller.signal.aborted) return
      const msg = err instanceof Error ? err.message : String(err)
      setDatasetError(msg)
      alert(`Upload failed: ${msg}`)
    } finally {
      pendingRequests.current.delete(controller)
      if (!controller.signal.aborted) setIsDatasetLoading(false)
    }
  }, [
    config.api_base,
    config.api_token,
    datasetFile,
    datasetKey,
    datasetName,
    refreshDatasets,
    sessionToken,
    beginRequest,
  ])

  const handleDeleteDataset = useCallback(
    async (name: string) => {
      const controller = beginRequest()
      setIsDatasetLoading(true)
      setDatasetError(null)
      try {
        await api.deleteDataset(
          name,
          datasetKey,
          config.api_token,
          config.api_base,
          sessionToken || undefined,
          controller.signal,
        )
        if (controller.signal.aborted) return
        setDatasets((prev) => prev.filter((d) => d.name !== name))
      } catch (err) {
        if (controller.signal.aborted) return
        const msg = err instanceof Error ? err.message : String(err)
        setDatasetError(msg)
        alert(`Delete failed: ${msg}`)
      } finally {
        pendingRequests.current.delete(controller)
        if (!controller.signal.aborted) setIsDatasetLoading(false)
      }
    },
    [config.api_base, config.api_token, datasetKey, sessionToken, beginRequest],
  )

  const handleSetActiveChatbot = useCallback(
    async (name: string) => {
      const controller = beginRequest()
      setIsDatasetLoading(true)
      setDatasetError(null)
      try {
        await api.setActiveChatbot(
          datasetKey,
          name,
          config.api_token,
          config.api_base,
          sessionToken || undefined,
          controller.signal,
        )
        if (controller.signal.aborted) return
        setActiveChatbot(name)
      } catch (err) {
        if (controller.signal.aborted) return
        const msg = err instanceof Error ? err.message : String(err)
        setDatasetError(msg)
        alert(`Activate failed: ${msg}`)
      } finally {
        pendingRequests.current.delete(controller)
        if (!controller.signal.aborted) setIsDatasetLoading(false)
      }
    },
    [config.api_base, config.api_token, datasetKey, sessionToken, beginRequest],
  )

  useEffect(() => {
    let mounted = true
    ;(async () => {
      try {
        const raw = await kvGet<string | { data: string; updated_at: number }>(
          StorageKeys.CUSTOM_DATASET_PASSWORD,
        )
        const stored = typeof raw === 'string' ? raw : raw?.data || ''

        const key = stored && stored.length > 0 ? stored : randomString(16)
        if (!stored) {
          await kvSet(StorageKeys.CUSTOM_DATASET_PASSWORD, {
            data: key,
            updated_at: Date.now(),
          })
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
    if (!datasetKey || !sessionToken) return
    let active = true
    const controller = beginRequest()
    void api
      .listDatasets(
        datasetKey,
        config.api_token,
        config.api_base,
        sessionToken,
        controller.signal,
      )
      .then((response) => {
        if (!active) return
        setDatasets(response.datasets || [])
        setDatasetError(null)
      })
      .catch((error: unknown) => {
        if (active)
          setDatasetError(
            error instanceof Error ? error.message : String(error),
          )
      })
      .finally(() => {
        pendingRequests.current.delete(controller)
        if (active) setLoadedRequestKey(requestKey)
      })
    return () => {
      active = false
      controller.abort()
    }
  }, [
    datasetKey,
    config.api_token,
    config.api_base,
    requestKey,
    sessionToken,
    beginRequest,
  ])

  const acceptFileTypes = useMemo(() => '.pdf,.md,.ppt,.pptx,.doc,.docx', [])

  return (
    <div className="space-y-3 rounded-lg border border-border p-3">
      <div
        className="flex cursor-pointer items-center justify-between"
        onClick={() => setIsExpanded(!isExpanded)}
      >
        <label className="cursor-pointer text-sm font-medium">
          Private Dataset (PDF Chat)
        </label>
        <div className="flex items-center gap-2">
          {isDatasetLoading && <Loader2 className="h-4 w-4 animate-spin" />}
          {isExpanded ? (
            <ChevronUp className="h-4 w-4" />
          ) : (
            <ChevronDown className="h-4 w-4" />
          )}
        </div>
      </div>

      {isExpanded && (
        <>
          <div className="flex items-center gap-2">
            <p className="text-xs text-muted-foreground">
              {sessionToken
                ? 'Private datasets use your signed-in account.'
                : 'Sign in to manage your private datasets.'}
            </p>
            {sessionToken ? (
              <Button size="sm" variant="outline" onClick={onSignOut}>
                Sign out
              </Button>
            ) : (
              <Button
                size="sm"
                variant="outline"
                onClick={() => {
                  try {
                    window.location.assign(ownershipSignInURL())
                  } catch {
                    setDatasetError(
                      'Sign-in needs browser session storage. Please enable it and try again.',
                    )
                  }
                }}
              >
                Sign in with Laisky SSO
              </Button>
            )}
          </div>
          <div className="space-y-2">
            <div>
              <label className="mb-1 block text-xs text-muted-foreground">
                Dataset password
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
                disabled={
                  isDatasetLoading || !sessionToken || !config.api_token
                }
              >
                Upload Dataset
              </Button>
              <Button
                size="sm"
                variant="outline"
                className="flex-1"
                onClick={refreshDatasets}
                disabled={isDatasetLoading || !sessionToken}
              >
                Refresh Datasets
              </Button>
              <Button
                size="sm"
                variant="outline"
                className="flex-1"
                onClick={refreshChatbots}
                disabled={isDatasetLoading || !sessionToken}
              >
                List Bots
              </Button>
            </div>
            {datasetError && (
              <p className="text-xs text-destructive">{datasetError}</p>
            )}
          </div>

          {loadedRequestKey === requestKey && datasets.length > 0 && (
            <div className="space-y-2">
              <div className="text-xs font-semibold text-foreground">
                Datasets
              </div>
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
                      disabled={isDatasetLoading || !sessionToken}
                    >
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  </div>
                ))}
              </div>
            </div>
          )}

          {chatbotRequestKey === requestKey && chatbots.length > 0 && (
            <div className="space-y-2">
              <div className="text-xs font-semibold text-foreground">
                Chatbots
              </div>
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
                      disabled={
                        isDatasetLoading || !sessionToken || !config.api_token
                      }
                      checked={activeChatbot === bot}
                      onChange={() => handleSetActiveChatbot(bot)}
                    />
                  </label>
                ))}
              </div>
            </div>
          )}

          <p className="text-xs text-muted-foreground">
            Upload PDFs/office docs to build a private dataset, then pick a
            chatbot to talk with it. Processing may take a few minutes.
          </p>
        </>
      )}
    </div>
  )
}
