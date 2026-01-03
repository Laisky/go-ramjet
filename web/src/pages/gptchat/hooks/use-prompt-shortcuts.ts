import { kvGet, kvSet, StorageKeys } from '@/utils/storage'
import { useCallback, useEffect, useState } from 'react'
import type { PromptShortcut } from '../types'

/**
 * usePromptShortcuts manages custom prompt shortcuts.
 */
export function usePromptShortcuts(configLoading: boolean) {
  const [promptShortcuts, setPromptShortcuts] = useState<PromptShortcut[]>([])

  const loadPromptShortcuts = useCallback(async () => {
    let shortcuts = await kvGet<PromptShortcut[]>(StorageKeys.PROMPT_SHORTCUTS)

    // If no shortcuts found (or empty array), use defaults
    if (!shortcuts || shortcuts.length === 0) {
      const { DefaultPrompts } = await import('../data/prompts')
      shortcuts = DefaultPrompts
    }

    setPromptShortcuts(shortcuts)
  }, [])

  // Load shortcuts on mount or when config finishes loading
  useEffect(() => {
    if (!configLoading) {
      loadPromptShortcuts()
    }
  }, [configLoading, loadPromptShortcuts])

  const handleSavePrompt = useCallback(
    async (name: string, prompt: string) => {
      const newShortcut: PromptShortcut = { name, prompt }
      // Check if already exists, if so update it, else append
      const index = promptShortcuts.findIndex((s) => s.name === name)
      let updated: PromptShortcut[]
      if (index >= 0) {
        updated = [...promptShortcuts]
        updated[index] = newShortcut
      } else {
        updated = [...promptShortcuts, newShortcut]
      }
      setPromptShortcuts(updated)
      await kvSet(StorageKeys.PROMPT_SHORTCUTS, updated)
    },
    [promptShortcuts],
  )

  const handleEditPrompt = useCallback(
    async (oldName: string, newName: string, newPrompt: string) => {
      const updated = promptShortcuts.map((s) =>
        s.name === oldName ? { name: newName, prompt: newPrompt } : s,
      )
      setPromptShortcuts(updated)
      await kvSet(StorageKeys.PROMPT_SHORTCUTS, updated)
    },
    [promptShortcuts],
  )

  const handleDeletePrompt = useCallback(
    async (name: string) => {
      const updated = promptShortcuts.filter((s) => s.name !== name)
      setPromptShortcuts(updated)
      await kvSet(StorageKeys.PROMPT_SHORTCUTS, updated)
    },
    [promptShortcuts],
  )

  return {
    promptShortcuts,
    handleSavePrompt,
    handleEditPrompt,
    handleDeletePrompt,
  }
}
