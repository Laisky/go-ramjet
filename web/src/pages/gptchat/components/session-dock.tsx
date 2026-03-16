import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { cn } from '@/utils/cn'
import { Plus, Trash2 } from 'lucide-react'
import { ConfirmAction } from './confirm-action'

interface SessionDockProps {
  sessions: { id: number; name: string; visible: boolean }[]
  activeSessionId: number
  onSwitchSession: (id: number) => void
  onCreateSession: () => void
  onClearChats: () => void
}

export function SessionDock({
  sessions,
  activeSessionId,
  onSwitchSession,
  onCreateSession,
  onClearChats,
}: SessionDockProps) {
  // Get first character of name for the badge
  const getAbbr = (name: string) => {
    return name.slice(0, 1).toUpperCase() || '#'
  }

  const visibleSessions = sessions.filter(
    (s) => s.visible || s.id === activeSessionId,
  )

  return (
    <TooltipProvider>
      <div className="flex flex-1 flex-col py-1">
        <div className="flex w-full flex-1 flex-col overflow-y-auto">
          {visibleSessions.map((session) => (
            <Tooltip key={session.id}>
              <TooltipTrigger asChild>
                <button
                  onClick={() => onSwitchSession(session.id)}
                  aria-label={`Switch to session ${session.name}`}
                  className={cn(
                    'flex h-11 w-full items-center justify-center border-b border-border text-[11px] font-bold transition-colors',
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
          ))}

          <Tooltip>
            <TooltipTrigger asChild>
              <button
                onClick={onCreateSession}
                aria-label="New Session"
                className="flex h-11 w-full items-center justify-center border-b border-dashed border-border text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
              >
                <Plus className="h-4 w-4" />
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
                className="flex h-11 w-full items-center justify-center bg-warning text-warning-foreground transition-colors hover:bg-warning/90"
                aria-label="Clear Chat History"
                title="Clear Chat History"
              >
                <Trash2 className="h-4 w-4" />
              </button>
            }
          />
        </div>
      </div>
    </TooltipProvider>
  )
}
