import { Plus } from 'lucide-react'

import { Button } from '@/components/ui/button'

export interface ConversationInsertProps {
  /** Called when the user chooses this conversation boundary. */
  onInsert: () => void
  /** Prevents starting another request while a response is being generated. */
  disabled?: boolean
}

/**
 * ConversationInsert renders a quiet insertion affordance between two messages.
 */
export function ConversationInsert({
  onInsert,
  disabled = false,
}: ConversationInsertProps) {
  return (
    <div className="group/insert relative flex h-7 items-center justify-center">
      <Button
        type="button"
        variant="outline"
        size="sm"
        onClick={onInsert}
        disabled={disabled}
        className="relative z-10 h-7 gap-1 rounded-full border-primary/30 bg-background/95 px-2.5 text-[11px] font-medium text-primary shadow-sm transition-[opacity,transform,background-color] hover:bg-primary/10 sm:pointer-events-none sm:translate-y-1 sm:opacity-0 sm:group-hover/insert:pointer-events-auto sm:group-hover/insert:translate-y-0 sm:group-hover/insert:opacity-100 sm:group-focus-within/insert:pointer-events-auto sm:group-focus-within/insert:translate-y-0 sm:group-focus-within/insert:opacity-100"
        aria-label="Insert message here"
        title="Insert a message at this point"
      >
        <Plus className="h-3.5 w-3.5" aria-hidden="true" />
        <span>Insert here</span>
      </Button>
    </div>
  )
}
