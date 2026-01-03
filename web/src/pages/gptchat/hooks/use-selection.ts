import { useEffect, useState } from 'react'

interface SelectionData {
  text: string
  position: { top: number; left: number }
}

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
        if (selection && selection.toString().trim().length > 0) {
          const range = selection.getRangeAt(0)
          const rect = range.getBoundingClientRect()

          // Check if selection is within the messages container
          const container = containerRef.current
          if (container && container.contains(selection.anchorNode)) {
            setSelectionData({
              text: selection.toString(),
              position: {
                top: clientY || rect.top,
                left: clientX || rect.left + rect.width / 2,
              },
            })
          }
        } else {
          setSelectionData(null)
        }
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
