
import { Trash2, Plus } from 'lucide-react'
import { cn } from '@/utils/cn'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { ConfirmDialog } from '@/components/confirm-dialog'

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
    <div className="flex flex-col items-center gap-2 border-r border-black/10 bg-gray-50/50 py-4 dark:border-white/10 dark:bg-gray-900/50 w-[50px] h-full shrink-0">
      <div className="flex-1 space-y-2 overflow-y-auto no-scrollbar w-full px-1 flex flex-col items-center">
        {sessions.map((session) => (
          <TooltipProvider key={session.id}>
            <Tooltip>
              <TooltipTrigger asChild>
                <button
                  onClick={() => onSwitchSession(session.id)}
                  className={cn(
                    'flex h-8 w-8 items-center justify-center rounded text-xs font-medium transition-colors',
                    session.id === activeSessionId
                      ? 'bg-blue-600 text-white shadow-sm'
                      : 'bg-white text-gray-700 hover:bg-gray-200 dark:bg-gray-800 dark:text-gray-300 dark:hover:bg-gray-700'
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
                className="flex h-8 w-8 items-center justify-center rounded border border-dashed border-gray-400 text-gray-500 hover:border-gray-600 hover:bg-gray-100 hover:text-gray-700 dark:border-gray-600 dark:text-gray-400 dark:hover:border-gray-400 dark:hover:bg-gray-800"
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

      <div className="w-full px-1 flex flex-col items-center pt-2 border-t border-black/5 dark:border-white/5">
        <ConfirmDialog
          title="Delete Current Session"
          description="Are you sure you want to delete the current active session? This action cannot be undone."
          onConfirm={() => onDeleteSession(activeSessionId)}
          trigger={
            <button
              className="flex h-8 w-8 items-center justify-center rounded bg-yellow-500 text-white shadow-sm hover:bg-yellow-600 transition-colors"
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
