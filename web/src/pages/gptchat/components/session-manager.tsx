import { ConfirmDialog } from '@/components/confirm-dialog'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import {
  Check,
  Copy,
  Edit2,
  MessageSquare,
  Plus,
  Trash2,
  X,
} from 'lucide-react'
import { useState } from 'react'

interface SessionManagerProps {
  sessions: { id: number; name: string }[]
  activeSessionId: number
  onSwitchSession: (id: number) => void
  onCreateSession: (name: string) => void
  onDeleteSession: (id: number) => void
  onRenameSession: (id: number, name: string) => void
  onDuplicateSession?: (id: number) => void
}

export function SessionManager({
  sessions,
  activeSessionId,
  onSwitchSession,
  onCreateSession,
  onDeleteSession,
  onRenameSession,
  onDuplicateSession,
}: SessionManagerProps) {
  const [isCreating, setIsCreating] = useState(false)
  const [editingId, setEditingId] = useState<number | null>(null)
  const [newName, setNewName] = useState('')

  const handleCreate = () => {
    if (newName.trim()) {
      onCreateSession(newName.trim())
      setNewName('')
      setIsCreating(false)
    }
  }

  const handleRename = () => {
    if (editingId && newName.trim()) {
      onRenameSession(editingId, newName.trim())
      setEditingId(null)
      setNewName('')
    }
  }

  const startEdit = (id: number, currentName: string) => {
    setEditingId(id)
    setNewName(currentName)
    setIsCreating(false)
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <label className="text-sm font-medium">Sessions</label>
        <Button
          variant="outline"
          size="sm"
          className="h-6 w-6 p-0"
          onClick={() => {
            setIsCreating(true)
            setNewName(`Chat Session ${sessions.length + 1}`)
            setEditingId(null)
          }}
          disabled={isCreating}
        >
          <Plus className="h-3.5 w-3.5" />
        </Button>
      </div>

      {(isCreating || editingId !== null) && (
        <Card className="p-2 bg-gray-50 dark:bg-gray-900 border-dashed mb-2">
          <div className="flex gap-2">
            <Input
              value={newName}
              onChange={(e) => setNewName(e.target.value)}
              className="h-8 text-sm"
              autoFocus
              placeholder="Session Name"
              onKeyDown={(e) => {
                if (e.key === 'Enter') {
                  if (isCreating) {
                    handleCreate()
                  } else {
                    handleRename()
                  }
                }
                if (e.key === 'Escape') {
                  setIsCreating(false)
                  setEditingId(null)
                }
              }}
            />
            <Button
              size="sm"
              className="h-8 w-8 p-0"
              onClick={isCreating ? handleCreate : handleRename}
            >
              <Check className="h-4 w-4" />
            </Button>
            <Button
              variant="ghost"
              size="sm"
              className="h-8 w-8 p-0"
              onClick={() => {
                setIsCreating(false)
                setEditingId(null)
              }}
            >
              <X className="h-4 w-4" />
            </Button>
          </div>
        </Card>
      )}

      <div className="space-y-1 max-h-[200px] overflow-y-auto pr-1">
        {sessions.map((session) => (
          <div
            key={session.id}
            className={`group flex items-center justify-between gap-2 rounded-md border p-2 text-sm transition-colors ${
              session.id === activeSessionId
                ? 'bg-blue-50 border-blue-200 dark:bg-blue-900/20 dark:border-blue-800'
                : 'hover:bg-gray-100 dark:hover:bg-gray-800 border-transparent'
            }`}
          >
            <button
              className="flex flex-1 items-center gap-2 truncate text-left"
              onClick={() => onSwitchSession(session.id)}
            >
              <MessageSquare
                className={`h-3.5 w-3.5 ${session.id === activeSessionId ? 'text-blue-500' : 'text-gray-400'}`}
              />
              <span
                className={`truncate ${session.id === activeSessionId ? 'font-medium' : ''}`}
              >
                {session.name}
              </span>
            </button>

            <div className="flex gap-1 opacity-0 transition-opacity group-hover:opacity-100">
              <Button
                variant="ghost"
                size="sm"
                className="h-6 w-6 p-0 text-gray-400 hover:text-blue-500"
                onClick={() => startEdit(session.id, session.name)}
              >
                <Edit2 className="h-3 w-3" />
              </Button>

              {onDuplicateSession && (
                <Button
                  variant="ghost"
                  size="sm"
                  className="h-6 w-6 p-0 text-gray-400 hover:text-emerald-500"
                  onClick={() => onDuplicateSession(session.id)}
                >
                  <Copy className="h-3 w-3" />
                </Button>
              )}

              {sessions.length > 1 && (
                <ConfirmDialog
                  title="Delete Session"
                  description={`Are you sure you want to delete "${session.name}"? This will delete all chat history and settings for this session.`}
                  onConfirm={() => onDeleteSession(session.id)}
                  trigger={
                    <Button
                      variant="ghost"
                      size="sm"
                      className="h-6 w-6 p-0 text-gray-400 hover:text-red-500"
                    >
                      <Trash2 className="h-3 w-3" />
                    </Button>
                  }
                />
              )}
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}
