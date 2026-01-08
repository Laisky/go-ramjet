/**
 * Configuration sidebar for chat settings.
 */
import { Eye, EyeOff, Settings, Trash2, User, X } from 'lucide-react'
import { useState } from 'react'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { DataSyncManager } from './data-sync-manager'
import { DatasetManager } from './dataset-manager'
import { McpServerManager } from './mcp-server-manager'
import { ModelSelector } from './model-selector'
import { PromptShortcutManager } from './prompt-shortcut-manager'
import { SessionManager } from './session-manager'

import { useUser } from '../hooks/use-user'
import type { PromptShortcut, SessionConfig } from '../types'

export interface ConfigSidebarProps {
  isOpen: boolean
  onClose: () => void
  config: SessionConfig
  onConfigChange: (updates: Partial<SessionConfig>) => void
  onClearChats: () => void
  onReset: () => void
  promptShortcuts?: PromptShortcut[]
  onSavePrompt?: (name: string, prompt: string) => void
  onEditPrompt?: (oldName: string, newName: string, newPrompt: string) => void
  onDeletePrompt?: (name: string) => void
  onExportData: () => Promise<unknown>
  onImportData: (data: unknown) => Promise<void>

  // Session Management
  sessions?: { id: number; name: string; visible: boolean }[]
  activeSessionId?: number
  onSwitchSession?: (id: number) => void
  onCreateSession?: (name: string) => void
  onDeleteSession?: (id: number) => void
  onRenameSession?: (id: number, name: string) => void
  onUpdateSessionVisibility?: (id: number, visible: boolean) => void
  onDuplicateSession?: (id: number) => void
  onReorderSessions?: (ids: number[]) => void
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
  onEditPrompt,
  onDeletePrompt,
  onExportData,
  onImportData,
  sessions = [],
  activeSessionId = 1,
  onSwitchSession,
  onCreateSession,
  onDeleteSession,
  onRenameSession,
  onUpdateSessionVisibility,
  onDuplicateSession,
  onReorderSessions,
  onPurgeAllSessions,
}: ConfigSidebarProps) {
  const [showApiKey, setShowApiKey] = useState(false)

  const { user } = useUser(config.api_token)

  if (!isOpen) return null

  return (
    <div className="fixed inset-0 z-50 flex">
      {/* Backdrop */}
      <div
        className="absolute inset-0 bg-background/80 backdrop-blur-sm"
        onClick={onClose}
      />

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
                  onUpdateSessionVisibility={onUpdateSessionVisibility}
                  onDuplicateSession={onDuplicateSession}
                  onReorderSessions={onReorderSessions}
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

          {/* N Images */}
          <div>
            <label className="mb-1 flex items-center justify-between text-sm font-medium">
              <span>N Images</span>
              <span className="text-muted-foreground">
                {config.chat_switch.draw_n_images}
              </span>
            </label>
            <input
              type="range"
              min={1}
              max={4}
              step={1}
              value={config.chat_switch.draw_n_images}
              onChange={(e) =>
                onConfigChange({
                  chat_switch: {
                    ...config.chat_switch,
                    draw_n_images: parseInt(e.target.value, 10),
                  },
                })
              }
              className="w-full"
            />
          </div>

          {/* Context Count */}
          <div>
            <label className="mb-1 flex items-center justify-between text-sm font-medium">
              <span>Contexts</span>
              <span className="text-muted-foreground">{config.n_contexts}</span>
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
              <span className="text-muted-foreground">{config.max_tokens}</span>
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

          <PromptShortcutManager
            config={config}
            onConfigChange={onConfigChange}
            promptShortcuts={promptShortcuts}
            onSavePrompt={onSavePrompt}
            onEditPrompt={onEditPrompt}
            onDeletePrompt={onDeletePrompt}
          />

          <div className="h-px bg-border" />

          <DatasetManager config={config} />

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

          <DataSyncManager
            config={config}
            onConfigChange={onConfigChange}
            onExportData={onExportData}
            onImportData={onImportData}
          />

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
