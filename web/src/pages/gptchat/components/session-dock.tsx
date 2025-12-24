import { ConfirmDialog } from '@/components/confirm-dialog'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { cn } from '@/utils/cn'
import { Plus, Trash2 } from 'lucide-react'

interface SessionDockProps {
  sessions: { id: number; name: string }[]
  activeSessionId: number
  onSwitchSession: (id: number) => void
  onCreateSession: () => void
  onDeleteSession: (id: number) => void
}

export function SessionDock({
  sessions,
  activeSessionId,
  onSwitchSession,
  onCreateSession,
  onDeleteSession,
}: SessionDockProps) {
  // Get first character of name for the badge
  const getAbbr = (name: string) => {
    return name.slice(0, 1).toUpperCase() || '#'
  }

  return (
    <div className="flex flex-1 flex-col py-1">
      <div className="flex w-full flex-1 flex-col overflow-y-auto no-scrollbar">
        {sessions.map((session) => (
          <TooltipProvider key={session.id}>
            <Tooltip>
              <TooltipTrigger asChild>
                <button
                  onClick={() => onSwitchSession(session.id)}
                  className={cn(
                    'flex h-9 w-full items-center justify-center border-b border-border text-[11px] font-bold transition-colors',
                    session.id === activeSessionId
                      ? 'bg-primary text-primary-foreground'
                      : 'bg-transparent text-foreground hover:bg-muted',
                  )}
                >
                  {getAbbr(session.name)}
                </button>
              </TooltipTrigger>
              <TooltipContent side="right">
                <p>{session.name}</p>
              </TooltipContent>
            </Tooltip>
          </TooltipProvider>
        ))}

        <TooltipProvider>
          <Tooltip>
            <TooltipTrigger asChild>
              <button
                onClick={onCreateSession}
                className="flex h-9 w-full items-center justify-center border-b border-dashed border-border text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
              >
                <Plus className="h-4 w-4" />
              </button>
            </TooltipTrigger>
            <TooltipContent side="right">
              <p>New Session</p>
            </TooltipContent>
          </Tooltip>
        </TooltipProvider>
      </div>

      <div className="flex w-full flex-col border-t border-border">
        <ConfirmDialog
          title="Delete Current Session"
          description="Are you sure you want to delete the current active session? This action cannot be undone."
          onConfirm={() => onDeleteSession(activeSessionId)}
          trigger={
            <button
              className="flex h-9 w-full items-center justify-center bg-warning text-warning-foreground transition-colors hover:bg-warning/90"
              title="Delete Current Session"
            >
              <Trash2 className="h-4 w-4" />
            </button>
          }
        />
      </div>
    </div>
  )
}
