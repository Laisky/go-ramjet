import { Pencil, Save, ShoppingBag, X } from 'lucide-react'
import { useCallback, useState } from 'react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { cn } from '@/utils/cn'
import type { PromptShortcut, SessionConfig } from '../types'
import { PromptMarket } from './prompt-market'

interface PromptShortcutManagerProps {
  config: SessionConfig
  onConfigChange: (updates: Partial<SessionConfig>) => void
  promptShortcuts: PromptShortcut[]
  onSavePrompt?: (name: string, prompt: string) => void
  onEditPrompt?: (oldName: string, newName: string, newPrompt: string) => void
  onDeletePrompt?: (name: string) => void
}

export function PromptShortcutManager({
  config,
  onConfigChange,
  promptShortcuts,
  onSavePrompt,
  onEditPrompt,
  onDeletePrompt,
}: PromptShortcutManagerProps) {
  const [showSavePrompt, setShowSavePrompt] = useState(false)
  const [showPromptMarket, setShowPromptMarket] = useState(false)
  const [editingPrompt, setEditingPrompt] = useState<{
    oldName: string
    name: string
    prompt: string
  } | null>(null)
  const [newPromptName, setNewPromptName] = useState('')

  const handleSavePrompt = useCallback(() => {
    const trimmedName = String(newPromptName || '').trim()
    if (onSavePrompt && trimmedName && config.system_prompt) {
      onSavePrompt(trimmedName, config.system_prompt)
      setNewPromptName('')
      setShowSavePrompt(false)
    }
  }, [onSavePrompt, newPromptName, config.system_prompt])

  const handleUpdatePrompt = useCallback(() => {
    if (editingPrompt && onEditPrompt) {
      onEditPrompt(
        editingPrompt.oldName,
        editingPrompt.name,
        editingPrompt.prompt,
      )
      setEditingPrompt(null)
    }
  }, [editingPrompt, onEditPrompt])

  const handleMarketAdd = useCallback(
    (name: string, prompt: string) => {
      if (onSavePrompt) {
        onSavePrompt(name, prompt)
      }
    },
    [onSavePrompt],
  )

  const handleSelectPrompt = useCallback(
    (prompt: string) => {
      onConfigChange({ system_prompt: prompt })
    },
    [onConfigChange],
  )

  return (
    <div className="space-y-6">
      {/* System Prompt */}
      <div>
        <label className="mb-1 flex items-center justify-between text-sm font-medium">
          <span>System Prompt</span>
          <div className="flex gap-1">
            <Button
              variant="ghost"
              size="sm"
              className="h-6 px-2"
              onClick={() => setShowPromptMarket(!showPromptMarket)}
              title="Prompt Marketplace"
            >
              <ShoppingBag className="h-3 w-3" />
            </Button>
            <Button
              variant="ghost"
              size="sm"
              className="h-6 px-2"
              onClick={() => setShowSavePrompt(!showSavePrompt)}
              title="Save as Shortcut"
            >
              <Save className="h-3 w-3" />
            </Button>
          </div>
        </label>
        <Textarea
          value={config.system_prompt}
          onChange={(e) => onConfigChange({ system_prompt: e.target.value })}
          placeholder="System prompt..."
          rows={4}
        />
      </div>

      {/* Prompt Marketplace */}
      {showPromptMarket && (
        <Card className="p-3">
          <div className="mb-2 flex items-center justify-between">
            <CardTitle className="text-sm">Prompt Marketplace</CardTitle>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => setShowPromptMarket(false)}
            >
              <X className="h-3 w-3" />
            </Button>
          </div>
          <PromptMarket onAddPrompt={handleMarketAdd} />
        </Card>
      )}

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
                <div className="ml-1 flex items-center gap-0.5">
                  <button
                    className="rounded-full p-0.5 hover:bg-muted"
                    onClick={(e) => {
                      e.stopPropagation()
                      setEditingPrompt({
                        oldName: shortcut.name,
                        name: shortcut.name,
                        prompt: shortcut.prompt,
                      })
                    }}
                  >
                    <Pencil className="h-3 w-3" />
                  </button>
                  {onDeletePrompt && (
                    <button
                      className="rounded-full p-0.5 hover:bg-muted"
                      onClick={(e) => {
                        e.stopPropagation()
                        onDeletePrompt(shortcut.name)
                      }}
                    >
                      <X className="h-3 w-3" />
                    </button>
                  )}
                </div>
              </Badge>
            ))}
          </div>
        </div>
      )}

      {/* Edit prompt dialog */}
      {editingPrompt && (
        <Card className="p-3 space-y-3">
          <div className="flex items-center justify-between">
            <CardTitle className="text-sm">Edit Shortcut</CardTitle>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => setEditingPrompt(null)}
            >
              <X className="h-3 w-3" />
            </Button>
          </div>
          <div className="space-y-2">
            <Input
              value={editingPrompt.name}
              onChange={(e) =>
                setEditingPrompt({ ...editingPrompt, name: e.target.value })
              }
              placeholder="Shortcut name"
              className="text-sm"
            />
            <Textarea
              value={editingPrompt.prompt}
              onChange={(e) =>
                setEditingPrompt({
                  ...editingPrompt,
                  prompt: e.target.value,
                })
              }
              placeholder="Prompt content"
              className="text-sm"
              rows={3}
            />
            <Button size="sm" className="w-full" onClick={handleUpdatePrompt}>
              Update Shortcut
            </Button>
          </div>
        </Card>
      )}
    </div>
  )
}
