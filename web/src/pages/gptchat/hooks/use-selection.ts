import { useEffect, useState } from 'react'
import type { SelectionData } from '../types'
import { rangeToMarkdown } from '../utils/selection-markdown'

/**
 * useSelection tracks text selection within a container.
 */
export function useSelection(
  containerRef: React.RefObject<HTMLElement | null>,
) {
  const [selectionData, setSelectionData] = useState<SelectionData | null>(null)

  useEffect(() => {
    const handleGlobalMouseUp = (e: MouseEvent) => {
      const { clientX, clientY } = e
      // Small delay to allow selection to be finalized
      setTimeout(() => {
        const selection = window.getSelection()
        if (!selection || selection.toString().trim().length === 0) {
          setSelectionData(null)
          return
        }

        if (selection.rangeCount === 0) {
          setSelectionData(null)
          return
        }

        const range = selection.getRangeAt(0)
        const rect = range.getBoundingClientRect()

        // Check if selection is within the messages container
        const container = containerRef.current
        const anchorNode = selection.anchorNode
        if (container && anchorNode && container.contains(anchorNode)) {
          const text = selection.toString()
          const markdown = rangeToMarkdown(range)
          setSelectionData({
            text,
            copyText: markdown || text,
            source: 'message',
            position: {
              top: clientY || rect.top,
              left: clientX || rect.left + rect.width / 2,
            },
          })
          return
        }

        setSelectionData(null)
      }, 10)
    }

    document.addEventListener('mouseup', handleGlobalMouseUp)
    return () => document.removeEventListener('mouseup', handleGlobalMouseUp)
  }, [containerRef])

  return {
    selectionData,
    setSelectionData,
  }
}
