import { cn } from '@/utils/cn'
import * as React from 'react'

export type TextareaProps = React.TextareaHTMLAttributes<HTMLTextAreaElement>

/**
 * Textarea component with auto-resize support.
 */
const Textarea = React.forwardRef<HTMLTextAreaElement, TextareaProps>(
  ({ className, ...props }, ref) => {
    return (
      <textarea
        className={cn(
          'theme-input theme-focus-ring flex min-h-[80px] w-full rounded-md border px-3 py-2 text-sm placeholder:opacity-80 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50',
          className,
        )}
        ref={ref}
        {...props}
      />
    )
  },
)
Textarea.displayName = 'Textarea'

export { Textarea }
