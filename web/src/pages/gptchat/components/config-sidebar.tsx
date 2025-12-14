/**
 * Configuration sidebar for chat settings.
 */
import { Settings, Save, Trash2, X } from 'lucide-react'
import { useState, useCallback } from 'react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { cn } from '@/utils/cn'
import { ConfirmDialog } from '@/components/confirm-dialog'
import { ModelSelector } from './model-selector'
import type { SessionConfig, PromptShortcut } from '../types'

export interface ConfigSidebarProps {
  isOpen: boolean
  onClose: () => void
  config: SessionConfig
  onConfigChange: (updates: Partial<SessionConfig>) => void
  onClearChats: () => void
  onReset: () => void
  promptShortcuts?: PromptShortcut[]
  onSavePrompt?: (name: string, prompt: string) => void
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
}: ConfigSidebarProps) {
  const [showSavePrompt, setShowSavePrompt] = useState(false)
  const [newPromptName, setNewPromptName] = useState('')

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
    [onConfigChange]
  )

  if (!isOpen) return null

  return (
    <div className="fixed inset-0 z-50 flex">
      {/* Backdrop */}
      <div
        className="absolute inset-0 bg-black/50"
        onClick={onClose}
      />

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

        <div className="space-y-4">
          {/* API Token */}
          <div>
            <label className="mb-1 block text-sm font-medium">API Key</label>
            <Input
              type="password"
              value={config.api_token}
              onChange={(e) => onConfigChange({ api_token: e.target.value })}
              placeholder="sk-..."
            />
          </div>

          {/* Model Selection */}
          <div>
            <label className="mb-1 block text-sm font-medium">Model</label>
            <ModelSelector
              selectedModel={config.selected_model}
              onModelChange={(model) => onConfigChange({ selected_model: model })}
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
                      'cursor-pointer transition-colors hover:bg-blue-500 hover:text-white',
                      config.system_prompt === shortcut.prompt &&
                      'bg-blue-500 text-white'
                    )}
                    onClick={() => handleSelectPrompt(shortcut.prompt)}
                  >
                    {shortcut.name}
                  </Badge>
                ))}
              </div>
            </div>
          )}

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
          </div>
        </div>
      </div>
    </div>
  )
}
