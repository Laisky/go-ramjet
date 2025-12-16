import * as React from 'react'

import { cn } from '@/utils/cn'

export type ButtonProps = React.ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: 'default' | 'ghost' | 'outline' | 'destructive'
  size?: 'default' | 'sm'
}

/**
 * Button is a minimal Tailwind button component.
 */
export const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant = 'default', size = 'default', ...props }, ref) => {
    const base =
      'inline-flex items-center justify-center gap-2 rounded-md text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-offset-2 disabled:pointer-events-none disabled:opacity-50 theme-focus-ring'

    const variants: Record<NonNullable<ButtonProps['variant']>, string> = {
      default:
        'bg-[color:var(--accent)] text-[color:var(--accent-contrast)] hover:bg-[color:var(--accent-strong)]',
      ghost:
        'text-[color:var(--text-primary)] hover:bg-[color:var(--bg-muted)]',
      outline:
        'border theme-border bg-[color:var(--bg-surface)] text-[color:var(--text-primary)] hover:bg-[color:var(--bg-muted)]',
      destructive: 'bg-[#ef4444] text-white hover:bg-[#dc2626]',
    }

    const sizes: Record<NonNullable<ButtonProps['size']>, string> = {
      default: 'h-9 px-4 py-2',
      sm: 'h-8 px-3',
    }

    return (
      <button
        ref={ref}
        className={cn(base, variants[variant], sizes[size], className)}
        {...props}
      />
    )
  },
)
Button.displayName = 'Button'
