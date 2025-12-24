/**
 * Configuration sidebar for chat settings.
 */
import {
  CloudDownload,
  CloudUpload,
  Eye,
  EyeOff,
  Loader2,
  Save,
  Settings,
  Trash2,
  User,
  X,
} from 'lucide-react'
import { useCallback, useEffect, useMemo, useState } from 'react'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { cn } from '@/utils/cn'
import { kvGet, kvSet, StorageKeys } from '@/utils/storage'
import { McpServerManager } from './mcp-server-manager'
import { ModelSelector } from './model-selector'
import { SessionManager } from './session-manager'

import { useUser } from '../hooks/use-user'
import type { PromptShortcut, SessionConfig } from '../types'
import { api } from '../utils/api'

export interface ConfigSidebarProps {
  isOpen: boolean
  onClose: () => void
  config: SessionConfig
  onConfigChange: (updates: Partial<SessionConfig>) => void
  onClearChats: () => void
  onReset: () => void
  promptShortcuts?: PromptShortcut[]
  onSavePrompt?: (name: string, prompt: string) => void
  onDeletePrompt?: (name: string) => void
  onExportData: () => Promise<unknown>
  onImportData: (data: unknown) => Promise<void>

  // Session Management
  sessions?: { id: number; name: string }[]
  activeSessionId?: number
  onSwitchSession?: (id: number) => void
  onCreateSession?: (name: string) => void
  onDeleteSession?: (id: number) => void
  onRenameSession?: (id: number, name: string) => void
  onDuplicateSession?: (id: number) => void
  onPurgeAllSessions?: () => void
}

/**
 * ConfigSidebar provides settings controls for chat configuration.
 */
export function ConfigSidebar({
  isOpen,
  onClose,
  config,
  onConfigChange,
  onClearChats,
  onReset,
  promptShortcuts = [],
  onSavePrompt,
  onDeletePrompt,
  onExportData,
  onImportData,
  sessions = [],
  activeSessionId = 1,
  onSwitchSession,
  onCreateSession,
  onDeleteSession,
  onRenameSession,
  onDuplicateSession,
  onPurgeAllSessions,
}: ConfigSidebarProps) {
  const [showSavePrompt, setShowSavePrompt] = useState(false)
  const [newPromptName, setNewPromptName] = useState('')
  const [showApiKey, setShowApiKey] = useState(false)
  const [isSyncing, setIsSyncing] = useState(false)
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

  const { user } = useUser(config.api_token)

  const randomString = useCallback((length = 16) => {
    const chars = 'abcdefghijklmnopqrstuvwxyz0123456789'
    let result = ''
    for (let i = 0; i < length; i += 1) {
      result += chars.charAt(Math.floor(Math.random() * chars.length))
    }
    return result
  }, [])

  useEffect(() => {
    let mounted = true
    ;(async () => {
      try {
        const stored = await kvGet<string>(StorageKeys.CUSTOM_DATASET_PASSWORD)

        {
          /* Private Dataset / RAG */
        }
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
        </div>
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

  useEffect(() => {
    if (!datasetKey) return
    refreshDatasets()
  }, [datasetKey, refreshDatasets])

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
    if (!datasetFile || !datasetName.trim()) {
      alert('Choose a file and dataset name first.')
      return
    }
    setIsDatasetLoading(true)
    setDatasetError(null)
    try {
      await api.uploadDataset(
        datasetFile,
        datasetName.trim(),
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

  const acceptFileTypes = useMemo(() => '.pdf,.md,.ppt,.pptx,.doc,.docx', [])

  const handleSavePrompt = useCallback(() => {
    if (onSavePrompt && newPromptName.trim() && config.system_prompt) {
      onSavePrompt(newPromptName.trim(), config.system_prompt)
      setNewPromptName('')
      setShowSavePrompt(false)
    }
  }, [onSavePrompt, newPromptName, config.system_prompt])

  const handleSelectPrompt = useCallback(
    (prompt: string) => {
      onConfigChange({ system_prompt: prompt })
    },
    [onConfigChange],
  )

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

  if (!isOpen) return null

  return (
    <div className="fixed inset-0 z-50 flex">
      {/* Backdrop */}
      <div className="absolute inset-0 bg-background/80 backdrop-blur-sm" onClick={onClose} />

      {/* Sidebar */}
      <div className="bg-card text-card-foreground relative ml-auto w-full max-w-md overflow-y-auto p-4 shadow-lg">
        {/* Header */}
        <div className="mb-4 flex items-center justify-between">
          <h2 className="flex items-center gap-2 text-lg font-semibold">
            <Settings className="h-5 w-5" />
            Configuration
          </h2>
          <Button variant="ghost" size="sm" onClick={onClose}>
            <X className="h-4 w-4" />
          </Button>
        </div>

        <div className="flex-1 overflow-y-auto p-4 space-y-6">
          {/* Session Manager */}
          {onSwitchSession &&
            onCreateSession &&
            onDeleteSession &&
            onRenameSession && (
              <section className="space-y-3 pb-4 border-b border-border">
                <SessionManager
                  sessions={sessions}
                  activeSessionId={activeSessionId}
                  onSwitchSession={onSwitchSession}
                  onCreateSession={onCreateSession}
                  onDeleteSession={onDeleteSession}
                  onRenameSession={onRenameSession}
                  onDuplicateSession={onDuplicateSession}
                />
              </section>
            )}

          {/* User Profile */}
          {user && (
            <div className="flex items-center gap-3 rounded-lg border p-3 border-border">
              <div className="flex h-10 w-10 items-center justify-center rounded-full bg-primary/10">
                {user.image_url && !user.image_url.includes('openai.com') ? (
                  <img
                    src={user.image_url}
                    alt={user.user_name}
                    className="h-10 w-10 rounded-full"
                  />
                ) : (
                  <User className="h-6 w-6 text-primary" />
                )}
              </div>
              <div className="overflow-hidden">
                <div className="truncate font-medium">{user.user_name}</div>
                <div className="truncate text-xs text-muted-foreground">
                  {user.is_free ? 'Free Tier' : 'Pro User'}
                  {user.no_limit_expensive_models && ' • Unlimited'}
                </div>
              </div>
            </div>
          )}

          {/* API Token */}
          <div>
            <label className="mb-1 block text-sm font-medium">API Key</label>
            <div className="relative">
              <Input
                type={showApiKey ? 'text' : 'password'}
                value={config.api_token}
                onChange={(e) => onConfigChange({ api_token: e.target.value })}
                placeholder="sk-..."
                className="pr-10"
              />
              <button
                type="button"
                onClick={() => setShowApiKey(!showApiKey)}
                className="absolute right-0 top-0 flex h-full items-center px-3 text-muted-foreground hover:text-foreground"
              >
                {showApiKey ? (
                  <EyeOff className="h-4 w-4" />
                ) : (
                  <Eye className="h-4 w-4" />
                )}
              </button>
            </div>
          </div>

          {/* Model Selection */}
          <div className="space-y-2">
            <label className="mb-1 block text-sm font-medium">Models</label>
            <div className="flex flex-col gap-2">
              <ModelSelector
                label="Chat"
                categories={[
                  'OpenAI',
                  'Anthropic',
                  'Google',
                  'Deepseek',
                  'Others',
                ]}
                selectedModel={
                  config.selected_chat_model || config.selected_model
                }
                active={
                  !config.selected_model ||
                  config.selected_model === config.selected_chat_model
                }
                onModelChange={(model) =>
                  onConfigChange({
                    selected_model: model,
                    selected_chat_model: model,
                  })
                }
              />
              <ModelSelector
                label="Draw"
                categories={['Image']}
                selectedModel={
                  config.selected_draw_model || config.selected_model
                }
                active={config.selected_model === config.selected_draw_model}
                onModelChange={(model) =>
                  onConfigChange({
                    selected_model: model,
                    selected_draw_model: model,
                  })
                }
              />
            </div>
          </div>

          {/* Context Count */}
          <div>
            <label className="mb-1 flex items-center justify-between text-sm font-medium">
              <span>Contexts</span>
              <span className="text-muted-foreground">
                {config.n_contexts}
              </span>
            </label>
            <input
              type="range"
              min={1}
              max={30}
              step={1}
              value={config.n_contexts}
              onChange={(e) =>
                onConfigChange({ n_contexts: parseInt(e.target.value, 10) })
              }
              className="w-full"
            />
          </div>

          {/* Max Tokens */}
          <div>
            <label className="mb-1 flex items-center justify-between text-sm font-medium">
              <span>Max Tokens</span>
              <span className="text-muted-foreground">
                {config.max_tokens}
              </span>
            </label>
            <input
              type="range"
              min={1000}
              max={100000}
              step={1000}
              value={config.max_tokens}
              onChange={(e) =>
                onConfigChange({ max_tokens: parseInt(e.target.value, 10) })
              }
              className="w-full"
            />
          </div>

          {/* Temperature */}
          <div>
            <label className="mb-1 flex items-center justify-between text-sm font-medium">
              <span>Temperature</span>
              <span className="text-muted-foreground">
                {config.temperature.toFixed(1)}
              </span>
            </label>
            <input
              type="range"
              min={0}
              max={2}
              step={0.1}
              value={config.temperature}
              onChange={(e) =>
                onConfigChange({ temperature: parseFloat(e.target.value) })
              }
              className="w-full"
            />
          </div>

          {/* Presence Penalty */}
          <div>
            <label className="mb-1 flex items-center justify-between text-sm font-medium">
              <span>Presence Penalty</span>
              <span className="text-muted-foreground">
                {config.presence_penalty.toFixed(1)}
              </span>
            </label>
            <input
              type="range"
              min={-2}
              max={2}
              step={0.1}
              value={config.presence_penalty}
              onChange={(e) =>
                onConfigChange({ presence_penalty: parseFloat(e.target.value) })
              }
              className="w-full"
            />
          </div>

          {/* Frequency Penalty */}
          <div>
            <label className="mb-1 flex items-center justify-between text-sm font-medium">
              <span>Frequency Penalty</span>
              <span className="text-muted-foreground">
                {config.frequency_penalty.toFixed(1)}
              </span>
            </label>
            <input
              type="range"
              min={-2}
              max={2}
              step={0.1}
              value={config.frequency_penalty}
              onChange={(e) =>
                onConfigChange({
                  frequency_penalty: parseFloat(e.target.value),
                })
              }
              className="w-full"
            />
          </div>

          {/* System Prompt */}
          <div>
            <label className="mb-1 flex items-center justify-between text-sm font-medium">
              <span>System Prompt</span>
              <Button
                variant="ghost"
                size="sm"
                className="h-6 px-2"
                onClick={() => setShowSavePrompt(!showSavePrompt)}
              >
                <Save className="h-3 w-3" />
              </Button>
            </label>
            <Textarea
              value={config.system_prompt}
              onChange={(e) =>
                onConfigChange({ system_prompt: e.target.value })
              }
              placeholder="System prompt..."
              rows={4}
            />
          </div>

          {/* Save prompt dialog */}
          {showSavePrompt && (
            <Card className="p-3">
              <CardTitle className="mb-2 text-sm">Save as Shortcut</CardTitle>
              <div className="flex gap-2">
                <Input
                  value={newPromptName}
                  onChange={(e) => setNewPromptName(e.target.value)}
                  placeholder="Shortcut name"
                  className="text-sm"
                />
                <Button size="sm" onClick={handleSavePrompt}>
                  Save
                </Button>
              </div>
            </Card>
          )}

          {/* Prompt Shortcuts */}
          {promptShortcuts.length > 0 && (
            <div>
              <label className="mb-1 block text-sm font-medium">
                Prompt Shortcuts
              </label>
              <div className="flex flex-wrap gap-1">
                {promptShortcuts.map((shortcut, index) => (
                  <Badge
                    key={index}
                    variant="secondary"
                    className={cn(
                      'cursor-pointer transition-colors hover:bg-primary hover:text-primary-foreground pr-1',
                      config.system_prompt === shortcut.prompt &&
                        'bg-primary text-primary-foreground',
                    )}
                    onClick={() => handleSelectPrompt(shortcut.prompt)}
                  >
                    {shortcut.name}
                    {onDeletePrompt && (
                      <button
                        className="ml-1 rounded-full p-0.5 hover:bg-muted"
                        onClick={(e) => {
                          e.stopPropagation()
                          onDeletePrompt(shortcut.name)
                        }}
                      >
                        <X className="h-3 w-3" />
                      </button>
                    )}
                  </Badge>
                ))}
              </div>
            </div>
          )}

          <div className="h-px bg-border" />

          {/* MCP Servers */}
          <div className="space-y-4">
            <div className="flex items-center justify-between">
              <label className="text-sm font-medium">Enable MCP Support</label>
              <Switch
                checked={config.chat_switch.enable_mcp}
                onCheckedChange={(checked) =>
                  onConfigChange({
                    chat_switch: {
                      ...config.chat_switch,
                      enable_mcp: checked,
                    },
                  })
                }
              />
            </div>

            {config.chat_switch.enable_mcp && (
              <McpServerManager
                servers={config.mcp_servers || []}
                onChange={(servers) => onConfigChange({ mcp_servers: servers })}
              />
            )}
          </div>

          <div className="h-px bg-border" />

          {/* Data Sync */}
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
                {/* Reuse showApiKey toggle for simplicity or add showSyncKey state */}
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

          {/* Actions */}
          <div className="flex gap-2 border-t border-border pt-4">
            <ConfirmDialog
              title="Clear Chat History"
              description="Are you sure you want to clear all chat history in this session? This action cannot be undone."
              variant="destructive"
              onConfirm={onClearChats}
              trigger={
                <Button
                  variant="destructive"
                  size="sm"
                  className="flex items-center gap-1"
                >
                  <Trash2 className="h-3 w-3" />
                  Clear Chats
                </Button>
              }
            />

            <ConfirmDialog
              title="Reset Settings"
              description="Are you sure you want to reset all settings to defaults? This will not delete your chat history."
              onConfirm={onReset}
              trigger={
                <Button
                  variant="outline"
                  size="sm"
                  className="flex items-center gap-1"
                >
                  Reset All
                </Button>
              }
            />

            {onPurgeAllSessions && (
              <ConfirmDialog
                title="Purge All Sessions"
                variant="destructive"
                description="Remove every session, config, and chat history while keeping your current API token and base URL. This cannot be undone."
                onConfirm={onPurgeAllSessions}
                trigger={
                  <Button
                    variant="outline"
                    size="sm"
                    className="flex items-center gap-1 text-destructive border-destructive/50 hover:bg-destructive/10"
                  >
                    Purge All
                  </Button>
                }
              />
            )}
          </div>
        </div>
      </div>
    </div>
  )
}
