import { Input } from '@/components/ui/input'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { cn } from '@/utils/cn'
import {
  closestCenter,
  DndContext,
  PointerSensor,
  useSensor,
  useSensors,
  type DragEndEvent,
} from '@dnd-kit/core'
import {
  arrayMove,
  SortableContext,
  useSortable,
  verticalListSortingStrategy,
} from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { Check, MessageSquarePlus, Trash2, X } from 'lucide-react'
import { useCallback, useEffect, useRef, useState } from 'react'
import { ConfirmAction } from './confirm-action'

interface SessionDockProps {
  sessions: { id: number; name: string; visible: boolean }[]
  activeSessionId: number
  onSwitchSession: (id: number) => void
  onCreateSession: () => void
  onClearChats: () => void
  onReorderSessions?: (ids: number[]) => void
  onRenameSession?: (id: number, name: string) => void
}

export function SessionDock({
  sessions,
  activeSessionId,
  onSwitchSession,
  onCreateSession,
  onClearChats,
  onReorderSessions,
  onRenameSession,
}: SessionDockProps) {
  const [editingId, setEditingId] = useState<number | null>(null)
  const [editName, setEditName] = useState('')

  const visibleSessions = sessions.filter(
    (s) => s.visible || s.id === activeSessionId,
  )

  const sensors = useSensors(
    useSensor(PointerSensor, {
      activationConstraint: {
        distance: 8,
      },
    }),
  )

  const handleDragEnd = useCallback(
    (event: DragEndEvent) => {
      const { active, over } = event
      if (over && active.id !== over.id && onReorderSessions) {
        const fullIds = sessions.map((s) => s.id)
        const oldFullIndex = fullIds.indexOf(Number(active.id))
        const newFullIndex = fullIds.indexOf(Number(over.id))
        if (oldFullIndex !== -1 && newFullIndex !== -1) {
          const newOrder = arrayMove(fullIds, oldFullIndex, newFullIndex)
          onReorderSessions(newOrder)
        }
      }
    },
    [sessions, onReorderSessions],
  )

  const handleDoubleClick = useCallback(
    (session: { id: number; name: string }) => {
      if (!onRenameSession) return
      setEditingId(session.id)
      setEditName(session.name)
    },
    [onRenameSession],
  )

  const handleRenameConfirm = useCallback(() => {
    const trimmed = editName.trim()
    if (editingId !== null && trimmed && onRenameSession) {
      onRenameSession(editingId, trimmed)
    }
    setEditingId(null)
    setEditName('')
  }, [editingId, editName, onRenameSession])

  const handleRenameCancel = useCallback(() => {
    setEditingId(null)
    setEditName('')
  }, [])

  return (
    <TooltipProvider>
      <div className="flex flex-1 flex-col py-1">
        <div className="flex w-full flex-1 flex-col overflow-y-auto">
          <DndContext
            sensors={sensors}
            collisionDetection={closestCenter}
            onDragEnd={handleDragEnd}
          >
            <SortableContext
              items={visibleSessions.map((s) => s.id)}
              strategy={verticalListSortingStrategy}
            >
              {visibleSessions.map((session) => (
                <SortableDockItem
                  key={session.id}
                  session={session}
                  isActive={session.id === activeSessionId}
                  onSwitchSession={onSwitchSession}
                  onDoubleClick={handleDoubleClick}
                />
              ))}
            </SortableContext>
          </DndContext>

          <Tooltip>
            <TooltipTrigger asChild>
              <button
                onClick={onCreateSession}
                aria-label="New Session"
                className="flex h-11 w-full items-center justify-center border-b border-dashed border-border text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
              >
                <MessageSquarePlus className="h-4 w-4" />
              </button>
            </TooltipTrigger>
            <TooltipContent side="right">
              <p>New Session</p>
            </TooltipContent>
          </Tooltip>
        </div>

        <div className="flex w-full flex-col border-t border-border">
          <ConfirmAction
            action="clear-chat-history"
            onConfirm={onClearChats}
            trigger={
              <button
                className="flex h-11 w-full items-center justify-center bg-muted text-muted-foreground transition-colors hover:bg-destructive hover:text-destructive-foreground"
                aria-label="Clear Chat History"
                title="Clear Chat History"
              >
                <Trash2 className="h-4 w-4" />
              </button>
            }
          />
        </div>
      </div>

      {/* Rename popover - renders outside the dock, positioned to the right */}
      {editingId !== null && (
        <RenamePopover
          editName={editName}
          onEditNameChange={setEditName}
          onConfirm={handleRenameConfirm}
          onCancel={handleRenameCancel}
        />
      )}
    </TooltipProvider>
  )
}

function RenamePopover({
  editName,
  onEditNameChange,
  onConfirm,
  onCancel,
}: {
  editName: string
  onEditNameChange: (name: string) => void
  onConfirm: () => void
  onCancel: () => void
}) {
  const inputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    // Focus and select all text when popover opens
    setTimeout(() => {
      inputRef.current?.focus()
      inputRef.current?.select()
    }, 0)
  }, [])

  return (
    <div className="fixed left-14 top-1/2 z-50 -translate-y-1/2 rounded-lg border border-border bg-popover p-3 shadow-lg">
      <div className="flex flex-col gap-2">
        <label className="text-xs font-medium text-muted-foreground">
          Rename Session
        </label>
        <Input
          ref={inputRef}
          value={editName}
          onChange={(e) => onEditNameChange(e.target.value)}
          className="h-8 w-48 text-sm"
          placeholder="Session Name"
          onKeyDown={(e) => {
            if (e.nativeEvent.isComposing) return
            if (e.key === 'Enter') onConfirm()
            if (e.key === 'Escape') onCancel()
          }}
        />
        <div className="flex justify-end gap-1">
          <button
            onClick={onCancel}
            className="inline-flex h-7 items-center gap-1 rounded-md px-2 text-xs text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
          >
            <X className="h-3 w-3" />
            Cancel
          </button>
          <button
            onClick={onConfirm}
            className="inline-flex h-7 items-center gap-1 rounded-md bg-primary px-2 text-xs text-primary-foreground transition-colors hover:bg-primary/90"
          >
            <Check className="h-3 w-3" />
            Confirm
          </button>
        </div>
      </div>
    </div>
  )
}

interface SortableDockItemProps {
  session: { id: number; name: string; visible: boolean }
  isActive: boolean
  onSwitchSession: (id: number) => void
  onDoubleClick: (session: { id: number; name: string }) => void
}

function SortableDockItem({
  session,
  isActive,
  onSwitchSession,
  onDoubleClick,
}: SortableDockItemProps) {
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

  const getAbbr = (name: string) => {
    return name.slice(0, 1).toUpperCase() || '#'
  }

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <button
          ref={setNodeRef}
          style={style}
          {...attributes}
          {...listeners}
          onClick={() => onSwitchSession(session.id)}
          onDoubleClick={(e) => {
            e.preventDefault()
            onDoubleClick(session)
          }}
          aria-label={`Switch to session ${session.name}`}
          className={cn(
            'flex h-11 w-full items-center justify-center border-b border-border text-[11px] font-bold transition-colors cursor-grab active:cursor-grabbing',
            isActive
              ? 'bg-primary text-primary-foreground'
              : 'bg-transparent text-foreground hover:bg-muted',
            isDragging && 'opacity-50',
          )}
        >
          {getAbbr(session.name)}
        </button>
      </TooltipTrigger>
      <TooltipContent side="right">
        <p>{session.name}</p>
      </TooltipContent>
    </Tooltip>
  )
}
