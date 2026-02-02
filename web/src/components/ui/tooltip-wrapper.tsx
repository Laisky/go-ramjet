import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import type { ReactNode } from 'react'

interface TooltipWrapperProps {
  children: ReactNode
  content: string | ReactNode
  side?: 'top' | 'right' | 'bottom' | 'left'
}

export function TooltipWrapper({
  children,
  content,
  side,
}: TooltipWrapperProps) {
  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>{children}</TooltipTrigger>
        <TooltipContent side={side}>
          {typeof content === 'string' ? <p>{content}</p> : content}
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  )
}
