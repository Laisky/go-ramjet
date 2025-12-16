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
                    'flex h-9 w-full items-center justify-center border-b border-slate-200 text-[11px] font-bold transition-colors dark:border-slate-700',
                    session.id === activeSessionId
                      ? 'bg-blue-600 text-white'
                      : 'bg-transparent text-slate-700 hover:bg-slate-100 dark:text-slate-200 dark:hover:bg-slate-800',
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
                className="flex h-9 w-full items-center justify-center border-b border-dashed border-slate-300 text-slate-500 transition-colors hover:bg-slate-100 hover:text-slate-700 dark:border-slate-600 dark:text-slate-400 dark:hover:bg-slate-800"
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

      <div className="flex w-full flex-col border-t border-slate-200 dark:border-slate-700">
        <ConfirmDialog
          title="Delete Current Session"
          description="Are you sure you want to delete the current active session? This action cannot be undone."
          onConfirm={() => onDeleteSession(activeSessionId)}
          trigger={
            <button
              className="flex h-9 w-full items-center justify-center bg-yellow-500 text-white transition-colors hover:bg-yellow-600"
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
