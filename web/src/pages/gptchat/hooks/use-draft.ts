import { kvGet, kvSet, StorageKeys } from '@/utils/storage'
import { useEffect, useState } from 'react'

/**
 * useDraft manages the global chat input draft.
 */
export function useDraft() {
  const [globalDraft, setGlobalDraft] = useState<string>('')

  // Load global draft on mount
  useEffect(() => {
    const loadDraft = async () => {
      const draft = await kvGet<unknown>(StorageKeys.SESSION_DRAFTS)
      if (draft) {
        if (typeof draft === 'string') {
          setGlobalDraft(draft)
        } else if (typeof draft === 'object' && draft !== null) {
          // Migrate from old Record<number, string> format
          const values = Object.values(draft as Record<string, unknown>)
          const firstVal = values.find((v) => typeof v === 'string')
          if (firstVal) {
            setGlobalDraft(firstVal as string)
          }
        }
      }
    }
    loadDraft()
  }, [])

  // Persist global draft when it changes (debounced)
  useEffect(() => {
    const timer = setTimeout(() => {
      kvSet(StorageKeys.SESSION_DRAFTS, globalDraft)
    }, 1000)
    return () => clearTimeout(timer)
  }, [globalDraft])

  return {
    globalDraft,
    setGlobalDraft,
  }
}
