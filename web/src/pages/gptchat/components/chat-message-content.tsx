/**
 * Progressive disclosure for long user and assistant message content.
 */
import { ChevronDown, ChevronUp } from 'lucide-react'
import type { ReactNode } from 'react'
import { useCallback, useEffect, useId, useRef, useState } from 'react'

import { Button } from '@/components/ui/button'
import { cn } from '@/utils/cn'

const PREVIEW_HEIGHT_REM = 16
const PREVIEW_LINE_COUNT = 10
const EXPAND_LABEL = 'Show more'
const EXPAND_ARIA_LABEL = 'Expand full message'
const COLLAPSE_LABEL = 'Show less'
const COLLAPSE_ARIA_LABEL = 'Collapse message'

/**
 * ChatMessageContentProps describes a message body that may be disclosed progressively.
 */
export interface ChatMessageContentProps {
  content: string
  children: ReactNode
  variant: 'user' | 'assistant'
  isStreaming?: boolean
}

/**
 * isLikelyLongMessage provides a body-line fallback for environments without rendered dimensions.
 * @param content The raw message content.
 * @returns Whether the message is likely to exceed the preview size.
 */
function isLikelyLongMessage(content: string): boolean {
  return content.split(/\r?\n/).length > PREVIEW_LINE_COUNT
}

/**
 * getPreviewHeightPx converts the CSS preview height to pixels for overflow measurement.
 * @returns The preview height in pixels.
 */
function getPreviewHeightPx(): number {
  if (typeof document === 'undefined') {
    return PREVIEW_HEIGHT_REM * 16
  }

  const rootFontSize = Number.parseFloat(
    window.getComputedStyle(document.documentElement).fontSize,
  )
  return (
    (Number.isFinite(rootFontSize) ? rootFontSize : 16) * PREVIEW_HEIGHT_REM
  )
}

/**
 * ChatMessageContent renders message content with a ten-line-style preview for long messages.
 * @param props The message content, visual variant, and streaming state.
 * @returns A message body and an accessible disclosure control when needed.
 */
export function ChatMessageContent({
  content,
  children,
  variant,
  isStreaming = false,
}: ChatMessageContentProps) {
  const contentRef = useRef<HTMLDivElement>(null)
  const contentId = `chat-message-content-${useId().replaceAll(':', '')}`
  const [hasOverflow, setHasOverflow] = useState(() =>
    isLikelyLongMessage(content),
  )
  const [isExpanded, setIsExpanded] = useState(isStreaming)

  const measureOverflow = useCallback(() => {
    const element = contentRef.current
    if (
      !element ||
      (element.clientHeight === 0 && element.scrollHeight === 0)
    ) {
      return
    }

    setHasOverflow(element.scrollHeight > getPreviewHeightPx() + 1)
  }, [])

  useEffect(() => {
    measureOverflow()

    const element = contentRef.current
    if (!element) {
      return
    }

    const resizeObserver =
      typeof ResizeObserver !== 'undefined'
        ? new ResizeObserver(measureOverflow)
        : null
    resizeObserver?.observe(element)
    window.addEventListener('resize', measureOverflow)

    return () => {
      resizeObserver?.disconnect()
      window.removeEventListener('resize', measureOverflow)
    }
  }, [content, measureOverflow])

  const isCollapsible = hasOverflow && !isStreaming
  const shouldClip = isCollapsible && !isExpanded

  return (
    <>
      <div className="relative">
        <div
          ref={contentRef}
          id={contentId}
          className={cn('relative', shouldClip && 'max-h-64 overflow-hidden')}
        >
          {children}
        </div>
        {shouldClip && (
          <div
            aria-hidden="true"
            className={cn(
              'chat-message-content-fade pointer-events-none absolute inset-x-0 bottom-0 h-16',
              variant === 'user'
                ? 'chat-message-content-fade-user'
                : 'chat-message-content-fade-assistant',
            )}
          />
        )}
      </div>

      {isCollapsible && (
        <div className="mt-1 flex justify-center border-t border-border/40 pt-1">
          <Button
            type="button"
            variant="ghost"
            size="sm"
            aria-controls={contentId}
            aria-expanded={isExpanded}
            aria-label={isExpanded ? COLLAPSE_ARIA_LABEL : EXPAND_ARIA_LABEL}
            title={isExpanded ? COLLAPSE_ARIA_LABEL : EXPAND_ARIA_LABEL}
            onClick={() => setIsExpanded((expanded) => !expanded)}
            className="h-7 gap-1 px-2 text-xs text-muted-foreground hover:text-foreground"
          >
            {isExpanded ? (
              <ChevronUp aria-hidden="true" className="h-3.5 w-3.5" />
            ) : (
              <ChevronDown aria-hidden="true" className="h-3.5 w-3.5" />
            )}
            {isExpanded ? COLLAPSE_LABEL : EXPAND_LABEL}
          </Button>
        </div>
      )}
    </>
  )
}
