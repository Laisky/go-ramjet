import { Button } from '@/components/ui/button'
import { Check, Copy, Quote, Volume2, X } from 'lucide-react'
import React, { useEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'

interface SelectionToolbarProps {
  text: string
  /** Position relative to the viewport */
  position: { top: number; left: number }
  onCopy: () => void
  onTTS: () => void
  onQuote?: (text: string) => void
  onClose: () => void
}

/**
 * SelectionToolbar is a floating toolbar that appears when text is selected.
 * It provides actions like Copy, TTS, and Quote.
 */
export function SelectionToolbar({
  text,
  position,
  onCopy,
  onTTS,
  onQuote,
  onClose,
}: SelectionToolbarProps) {
  const toolbarRef = useRef<HTMLDivElement>(null)
  const [copied, setCopied] = useState(false)
  const [adjustedPosition, setAdjustedPosition] = useState(position)

  // Ensure the toolbar stays within the viewport
  useEffect(() => {
    if (toolbarRef.current) {
      const rect = toolbarRef.current.getBoundingClientRect()
      const viewportWidth = window.innerWidth

      let { top, left } = position

      // Center horizontally relative to the selection
      left = left - rect.width / 2

      // Adjust horizontally to stay in viewport
      if (left + rect.width > viewportWidth - 10) {
        left = viewportWidth - rect.width - 10
      }
      if (left < 10) {
        left = 10
      }

      // Adjust vertically (show above selection if possible, else below)
      // 'top' passed in is the top of the selection rect or mouse Y
      const viewportHeight = window.innerHeight
      if (top - rect.height < 10) {
        // Not enough space above, show below the selection
        top = top + 25

        // If also not enough space below, clamp to viewport
        if (top + rect.height > viewportHeight - 10) {
          top = viewportHeight - rect.height - 10
        }
      } else {
        top = top - rect.height - 10
      }

      setAdjustedPosition({ top, left })
    }
  }, [position, text])

  const handleCopy = (e: React.MouseEvent) => {
    e.stopPropagation()
    onCopy()
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  const handleTTS = (e: React.MouseEvent) => {
    e.stopPropagation()
    onTTS()
  }

  const handleQuote = (e: React.MouseEvent) => {
    e.stopPropagation()
    if (onQuote) onQuote(text)
  }

  const handleClose = (e: React.MouseEvent) => {
    e.stopPropagation()
    onClose()
  }

  return createPortal(
    <div
      ref={toolbarRef}
      className="fixed z-[100] flex items-center gap-0.5 rounded-lg border bg-popover p-1 shadow-lg animate-in fade-in zoom-in duration-150"
      style={{
        top: `${adjustedPosition.top}px`,
        left: `${adjustedPosition.left}px`,
      }}
      onMouseDown={(e) => e.stopPropagation()}
      onMouseUp={(e) => e.stopPropagation()}
    >
      <Button
        variant="ghost"
        size="sm"
        className="h-8 w-8 p-0"
        onClick={handleCopy}
        title="Copy selection"
      >
        {copied ? (
          <Check className="h-4 w-4 text-success" />
        ) : (
          <Copy className="h-4 w-4" />
        )}
      </Button>
      <Button
        variant="ghost"
        size="sm"
        className="h-8 w-8 p-0"
        onClick={handleTTS}
        title="Read selection"
      >
        <Volume2 className="h-4 w-4" />
      </Button>
      {onQuote && (
        <Button
          variant="ghost"
          size="sm"
          className="h-8 w-8 p-0"
          onClick={handleQuote}
          title="Quote selection"
        >
          <Quote className="h-4 w-4" />
        </Button>
      )}
      <div className="mx-1 h-4 w-[1px] bg-border" />
      <Button
        variant="ghost"
        size="sm"
        className="h-8 w-8 p-0"
        onClick={handleClose}
        title="Close"
      >
        <X className="h-4 w-4" />
      </Button>
    </div>,
    document.body,
  )
}
