import * as DropdownMenuPrimitive from '@radix-ui/react-dropdown-menu'

import { cn } from '@/utils/cn'

export const DropdownMenu = DropdownMenuPrimitive.Root
export const DropdownMenuTrigger = DropdownMenuPrimitive.Trigger

/**
 * DropdownMenuContent renders a styled dropdown content container.
 */
export function DropdownMenuContent(
  props: DropdownMenuPrimitive.DropdownMenuContentProps,
) {
  const { className, sideOffset = 8, ...rest } = props
  return (
    <DropdownMenuPrimitive.Portal>
      <DropdownMenuPrimitive.Content
        sideOffset={sideOffset}
        className={cn(
          'z-50 min-w-[10rem] overflow-hidden rounded-md border border-black/10 bg-white p-1 text-sm shadow-md dark:border-white/10 dark:bg-black',
          className,
        )}
        {...rest}
      />
    </DropdownMenuPrimitive.Portal>
  )
}

/**
 * DropdownMenuItem renders a styled clickable item.
 */
export function DropdownMenuItem(props: DropdownMenuPrimitive.DropdownMenuItemProps) {
  const { className, ...rest } = props
  return (
    <DropdownMenuPrimitive.Item
      className={cn(
        'flex cursor-default select-none items-center rounded px-2 py-1.5 outline-none focus:bg-black/5 dark:focus:bg-white/10',
        className,
      )}
      {...rest}
    />
  )
}
