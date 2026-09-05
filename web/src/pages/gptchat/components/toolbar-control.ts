import { cn } from '@/utils/cn'

/**
 * ToolbarTone selects the emphasis of a composer toolbar control.
 *
 * - `idle` is the resting outline used by an inactive toggle.
 * - `active` marks an enabled feature or a call in progress.
 * - `danger` marks a destructive action such as stopping a recording.
 */
export type ToolbarTone = 'idle' | 'active' | 'danger'

/** TOOLBAR_ICON_CLASS keeps every glyph in the toolbar row at one optical size. */
export const TOOLBAR_ICON_CLASS = 'h-3 w-3'

const TONE_CLASSES: Record<ToolbarTone, string> = {
  idle: 'border border-border bg-transparent text-muted-foreground hover:bg-muted hover:text-foreground',
  active:
    'bg-primary text-primary-foreground shadow-sm ring-1 ring-primary hover:bg-primary/90',
  danger:
    'bg-destructive text-destructive-foreground shadow-sm ring-1 ring-destructive hover:bg-destructive/90',
}

/**
 * toolbarControlClasses is the single source of truth for the composer toolbar row.
 *
 * The feature toggles and the audio controls sit side by side, so they must share one
 * type scale and one control height. Styling the audio controls with the generic Button
 * component instead left them at 14px next to 11px neighbours, which read as a mistake.
 *
 * Parameters:
 *   - tone: emphasis of the control.
 *   - className: extra layout classes for the specific control.
 *
 * Returns: the merged class string.
 */
export function toolbarControlClasses(
  tone: ToolbarTone = 'idle',
  className?: string,
): string {
  return cn(
    'rounded-md px-2.5 py-1.5 text-[11px] font-medium transition-colors',
    'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-offset-1 theme-focus-ring',
    'disabled:pointer-events-none disabled:opacity-60',
    TONE_CLASSES[tone],
    className,
  )
}

/** TOOLBAR_BUTTON_LAYOUT aligns an icon with its optional label. */
export const TOOLBAR_BUTTON_LAYOUT = 'flex items-center gap-1'
