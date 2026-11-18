/**
 * Selection toolbar state of the GPTChat page: the text selected in the
 * messages or in the chat input, and the toolbar actions on it.
 */
import { useCallback, useState, type RefObject } from 'react'

import type { SelectionData } from '../types'
import { useSelection } from './use-selection'

/**
 * UseSelectionActionsOptions configures useSelectionActions: containerRef
 * is the messages container whose selection is tracked, onPrefill puts a
 * quote into the chat input, and onSpeak reads text aloud.
 */
export interface UseSelectionActionsOptions {
  containerRef: RefObject<HTMLElement | null>
  onPrefill: (draft: { id: string; text: string }) => void
  onSpeak: (text: string) => void
}

/**
 * useSelectionActions tracks the active selection (the chat input's wins
 * over the messages') and returns it with the toolbar handlers: quote,
 * copy, read aloud, clear, and the chat input's selection callback.
 */
export function useSelectionActions({
  containerRef,
  onPrefill,
  onSpeak,
}: UseSelectionActionsOptions) {
  const { selectionData, setSelectionData } = useSelection(containerRef)
  const [inputSelectionData, setInputSelectionData] =
    useState<SelectionData | null>(null)
  const activeSelectionData = inputSelectionData || selectionData

  const handleQuote = useCallback(
    (text: string) => {
      onPrefill({ id: Date.now().toString(), text })
      setSelectionData(null)
      setInputSelectionData(null)
    },
    [onPrefill, setSelectionData],
  )

  /**
   * handleInputSelectionChange updates selection state from the input textarea.
   */
  const handleInputSelectionChange = useCallback(
    (data: SelectionData | null) => {
      setInputSelectionData(data)
      if (data) {
        setSelectionData(null)
      }
    },
    [setSelectionData],
  )

  /**
   * clearSelection resets selection toolbar state.
   */
  const clearSelection = useCallback(() => {
    setSelectionData(null)
    setInputSelectionData(null)
  }, [setSelectionData])

  const handleSelectionCopy = useCallback(async () => {
    if (!activeSelectionData) return
    try {
      await navigator.clipboard.writeText(
        activeSelectionData.copyText || activeSelectionData.text,
      )
    } catch (err) {
      console.error(`Failed to copy selection: ${String(err)}`)
    }
  }, [activeSelectionData])

  const handleSelectionTTS = useCallback(() => {
    if (activeSelectionData) {
      onSpeak(activeSelectionData.text)
    }
  }, [activeSelectionData, onSpeak])

  return {
    activeSelectionData,
    handleQuote,
    handleInputSelectionChange,
    clearSelection,
    handleSelectionCopy,
    handleSelectionTTS,
  }
}
