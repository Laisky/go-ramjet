import { Monitor, Moon, Sun } from 'lucide-react'
import { useTheme } from 'next-themes'

import { Button } from '@/components/ui/button'
import { cn } from '@/utils/cn'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'

export interface ThemeToggleProps {
  className?: string
}

/**
 * ThemeToggle provides a three-way theme switch: system, light, and dark.
 */
export function ThemeToggle({ className }: ThemeToggleProps) {
  const { theme, setTheme, resolvedTheme } = useTheme()

  const icon = (() => {
    if (theme === 'system')
      return <Monitor className="h-4 w-4" aria-hidden="true" />
    if (resolvedTheme === 'dark')
      return <Moon className="h-4 w-4" aria-hidden="true" />
    return <Sun className="h-4 w-4" aria-hidden="true" />
  })()

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="ghost"
          size="sm"
          className={cn('rounded-lg px-0', className)}
          aria-label="Toggle theme"
        >
          {icon}
          <span className="sr-only">Theme</span>
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuItem onSelect={() => setTheme('system')}>
          System
        </DropdownMenuItem>
        <DropdownMenuItem onSelect={() => setTheme('light')}>
          Light
        </DropdownMenuItem>
        <DropdownMenuItem onSelect={() => setTheme('dark')}>
          Dark
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
