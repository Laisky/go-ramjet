import { ConfirmDialog } from '@/components/confirm-dialog'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import {
  closestCenter,
  DndContext,
  KeyboardSensor,
  PointerSensor,
  useSensor,
  useSensors,
  type DragEndEvent,
} from '@dnd-kit/core'
import {
  arrayMove,
  SortableContext,
  sortableKeyboardCoordinates,
  useSortable,
  verticalListSortingStrategy,
} from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import {
  Check,
  Copy,
  Edit2,
  Eye,
  EyeOff,
  GripVertical,
  MessageSquare,
  Plus,
  Trash2,
  X,
} from 'lucide-react'
import { useState } from 'react'

interface SessionManagerProps {
  sessions: { id: number; name: string; visible: boolean }[]
  activeSessionId: number
  onSwitchSession: (id: number) => void
  onCreateSession: (name: string) => void
  onDeleteSession: (id: number) => void
  onRenameSession: (id: number, name: string) => void
  onUpdateSessionVisibility?: (id: number, visible: boolean) => void
  onDuplicateSession?: (id: number) => void
  onReorderSessions?: (ids: number[]) => void
}

export function SessionManager({
  sessions,
  activeSessionId,
  onSwitchSession,
  onCreateSession,
  onDeleteSession,
  onRenameSession,
  onUpdateSessionVisibility,
  onDuplicateSession,
  onReorderSessions,
}: SessionManagerProps) {
  const [isCreating, setIsCreating] = useState(false)
  const [editingId, setEditingId] = useState<number | null>(null)
  const [newName, setNewName] = useState('')

  const sensors = useSensors(
    useSensor(PointerSensor, {
      activationConstraint: {
        distance: 8,
      },
    }),
    useSensor(KeyboardSensor, {
      coordinateGetter: sortableKeyboardCoordinates,
    }),
  )

  const handleCreate = () => {
    const trimmed = String(newName || '').trim()
    if (trimmed) {
      onCreateSession(trimmed)
      setNewName('')
      setIsCreating(false)
    }
  }

  const handleRename = () => {
    const trimmed = String(newName || '').trim()
    if (editingId && trimmed) {
      onRenameSession(editingId, trimmed)
      setEditingId(null)
      setNewName('')
    }
  }

  const startEdit = (id: number, currentName: string) => {
    setEditingId(id)
    setNewName(currentName)
    setIsCreating(false)
  }

  const handleDragEnd = (event: DragEndEvent) => {
    const { active, over } = event

    if (over && active.id !== over.id) {
      const oldIndex = sessions.findIndex((s) => s.id === active.id)
      const newIndex = sessions.findIndex((s) => s.id === over.id)

      if (onReorderSessions) {
        const newSessions = arrayMove(sessions, oldIndex, newIndex)
        onReorderSessions(newSessions.map((s) => s.id))
      }
    }
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
        <Card className="p-2 bg-muted border-dashed mb-2">
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
        <DndContext
          sensors={sensors}
          collisionDetection={closestCenter}
          onDragEnd={handleDragEnd}
        >
          <SortableContext
            items={sessions.map((s) => s.id)}
            strategy={verticalListSortingStrategy}
          >
            {sessions.map((session) => (
              <SortableSessionItem
                key={session.id}
                session={session}
                activeSessionId={activeSessionId}
                onSwitchSession={onSwitchSession}
                onStartEdit={startEdit}
                onUpdateSessionVisibility={onUpdateSessionVisibility}
                onDuplicateSession={onDuplicateSession}
                onDeleteSession={onDeleteSession}
                canDelete={sessions.length > 1}
              />
            ))}
          </SortableContext>
        </DndContext>
      </div>
    </div>
  )
}

interface SortableSessionItemProps {
  session: { id: number; name: string; visible: boolean }
  activeSessionId: number
  onSwitchSession: (id: number) => void
  onStartEdit: (id: number, name: string) => void
  onUpdateSessionVisibility?: (id: number, visible: boolean) => void
  onDuplicateSession?: (id: number) => void
  onDeleteSession: (id: number) => void
  canDelete: boolean
}

function SortableSessionItem({
  session,
  activeSessionId,
  onSwitchSession,
  onStartEdit,
  onUpdateSessionVisibility,
  onDuplicateSession,
  onDeleteSession,
  canDelete,
}: SortableSessionItemProps) {
  const {
    attributes,
    listeners,
    setNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({ id: session.id })

  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
    zIndex: isDragging ? 10 : 1,
    position: 'relative' as const,
  }

  return (
    <div
      ref={setNodeRef}
      style={style}
      className={`group flex items-center justify-between gap-2 rounded-md border p-2 text-sm transition-colors ${
        session.id === activeSessionId
          ? 'bg-primary/10 border-primary/20'
          : 'hover:bg-muted border-transparent'
      } ${isDragging ? 'opacity-50' : ''} ${!session.visible ? 'opacity-60' : ''}`}
    >
      <div className="flex flex-1 items-center gap-2 truncate">
        <div
          {...attributes}
          {...listeners}
          className="cursor-grab active:cursor-grabbing p-1 -ml-1 text-muted-foreground hover:text-foreground"
        >
          <GripVertical className="h-3.5 w-3.5" />
        </div>
        <button
          className="flex flex-1 items-center gap-2 truncate text-left"
          onClick={() => onSwitchSession(session.id)}
        >
          <MessageSquare
            className={`h-3.5 w-3.5 ${session.id === activeSessionId ? 'text-primary' : 'text-muted-foreground'}`}
          />
          <span
            className={`truncate ${session.id === activeSessionId ? 'font-medium' : ''}`}
          >
            {session.name}
          </span>
        </button>
      </div>

      <div
        className={`flex gap-1 transition-opacity ${
          !session.visible ? 'opacity-100' : 'opacity-0 group-hover:opacity-100'
        }`}
      >
        {onUpdateSessionVisibility && (
          <Button
            variant="ghost"
            size="sm"
            className={`h-6 w-6 p-0 ${session.visible ? 'text-muted-foreground' : 'text-warning'} hover:text-primary`}
            onClick={() =>
              onUpdateSessionVisibility(session.id, !session.visible)
            }
            title={session.visible ? 'Hide from sidebar' : 'Show in sidebar'}
          >
            {session.visible ? (
              <Eye className="h-3 w-3" />
            ) : (
              <EyeOff className="h-3 w-3" />
            )}
          </Button>
        )}

        <Button
          variant="ghost"
          size="sm"
          className="h-6 w-6 p-0 text-muted-foreground hover:text-primary"
          onClick={() => onStartEdit(session.id, session.name)}
        >
          <Edit2 className="h-3 w-3" />
        </Button>

        {onDuplicateSession && (
          <Button
            variant="ghost"
            size="sm"
            className="h-6 w-6 p-0 text-muted-foreground hover:text-success"
            onClick={() => onDuplicateSession(session.id)}
          >
            <Copy className="h-3 w-3" />
          </Button>
        )}

        {canDelete && (
          <ConfirmDialog
            title="Delete Session"
            description={`Are you sure you want to delete "${session.name}"? This will delete all chat history and settings for this session.`}
            onConfirm={() => onDeleteSession(session.id)}
            trigger={
              <Button
                variant="ghost"
                size="sm"
                className="h-6 w-6 p-0 text-muted-foreground hover:text-destructive"
              >
                <Trash2 className="h-3 w-3" />
              </Button>
            }
          />
        )}
      </div>
    </div>
  )
}
