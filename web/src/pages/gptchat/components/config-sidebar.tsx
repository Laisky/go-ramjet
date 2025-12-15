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
import { useCallback, useState } from 'react'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { cn } from '@/utils/cn'
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

  const { user } = useUser(config.api_token)

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
      <div className="absolute inset-0 bg-black/50" onClick={onClose} />

      {/* Sidebar */}
      <div className="relative ml-auto w-full max-w-md overflow-y-auto bg-white p-4 shadow-lg dark:bg-black">
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
              <section className="space-y-3 pb-4 border-b border-gray-100 dark:border-gray-800">
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
            <div className="flex items-center gap-3 rounded-lg border p-3 border-black/10 dark:border-white/10">
              <div className="flex h-10 w-10 items-center justify-center rounded-full bg-blue-100 dark:bg-blue-900">
                {user.image_url && !user.image_url.includes('openai.com') ? (
                  <img
                    src={user.image_url}
                    alt={user.user_name}
                    className="h-10 w-10 rounded-full"
                  />
                ) : (
                  <User className="h-6 w-6 text-blue-600 dark:text-blue-300" />
                )}
              </div>
              <div className="overflow-hidden">
                <div className="truncate font-medium">{user.user_name}</div>
                <div className="truncate text-xs text-gray-500">
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
                className="absolute right-0 top-0 flex h-full items-center px-3 text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200"
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
          <div>
            <label className="mb-1 block text-sm font-medium">Model</label>
            <ModelSelector
              selectedModel={config.selected_model}
              onModelChange={(model) =>
                onConfigChange({ selected_model: model })
              }
            />
          </div>

          {/* Context Count */}
          <div>
            <label className="mb-1 flex items-center justify-between text-sm font-medium">
              <span>Contexts</span>
              <span className="text-black/50 dark:text-white/50">
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
              <span className="text-black/50 dark:text-white/50">
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
              <span className="text-black/50 dark:text-white/50">
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
              <span className="text-black/50 dark:text-white/50">
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
              <span className="text-black/50 dark:text-white/50">
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
                      'cursor-pointer transition-colors hover:bg-blue-500 hover:text-white pr-1',
                      config.system_prompt === shortcut.prompt &&
                        'bg-blue-500 text-white',
                    )}
                    onClick={() => handleSelectPrompt(shortcut.prompt)}
                  >
                    {shortcut.name}
                    {onDeletePrompt && (
                      <button
                        className="ml-1 rounded-full p-0.5 hover:bg-black/20 dark:hover:bg-white/20"
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
              <label className="mb-1 block text-xs text-gray-500">
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
            <p className="mt-1 text-xs text-gray-500">
              Sync your settings and chat history using this key. Keep it safe!
            </p>
          </div>

          {/* Actions */}
          <div className="flex gap-2 border-t border-black/10 pt-4 dark:border-white/10">
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
                    className="flex items-center gap-1 text-red-600 border-red-400 hover:bg-red-50"
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
