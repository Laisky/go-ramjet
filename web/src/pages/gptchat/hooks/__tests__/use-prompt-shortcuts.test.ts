import { act, cleanup, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { kvGet, kvSet } from '@/utils/storage'
import type { PromptShortcut } from '../../types'
import { usePromptShortcuts } from '../use-prompt-shortcuts'

vi.mock('@/utils/storage', () => ({
  kvGet: vi.fn(), kvSet: vi.fn(), StorageKeys: { PROMPT_SHORTCUTS: 'prompts' },
}))
vi.mock('../../data/prompts', () => ({ DefaultPrompts: [{ name: 'Default', prompt: 'Default prompt' }] }))

describe('prompt shortcut lifecycle', () => {
  beforeEach(() => {
    vi.mocked(kvGet).mockReset().mockResolvedValue(null)
    vi.mocked(kvSet).mockReset().mockResolvedValue(undefined)
  })
  afterEach(() => { cleanup(); vi.restoreAllMocks() })

  it('loads defaults only after configuration is available', async () => {
    const { result, rerender } = renderHook(({ loading }) => usePromptShortcuts(loading), { initialProps: { loading: true } })
    expect(kvGet).not.toHaveBeenCalled()
    rerender({ loading: false })
    await act(async () => {})
    expect(result.current.promptShortcuts).toEqual([{ name: 'Default', prompt: 'Default prompt' }])
  })

  it('drops stale storage results when configuration starts loading again', async () => {
    let resolve!: (value: PromptShortcut[]) => void
    vi.mocked(kvGet).mockReturnValueOnce(new Promise<PromptShortcut[]>((done) => { resolve = done }))
    const { result, rerender } = renderHook(({ loading }) => usePromptShortcuts(loading), { initialProps: { loading: false } })
    rerender({ loading: true })
    await act(async () => { resolve([{ name: 'Stale', prompt: 'Should not appear' }]) })
    expect(result.current.promptShortcuts).toEqual([])
  })

  it('filters empty names and preserves saving the current shortcuts', async () => {
    vi.mocked(kvGet).mockResolvedValue({ data: [{ name: ' ', prompt: 'Invalid' }, { name: 'Existing', prompt: 'Keep' }], updated_at: 1 })
    const { result } = renderHook(() => usePromptShortcuts(false))
    await act(async () => {})
    expect(result.current.promptShortcuts).toEqual([{ name: 'Existing', prompt: 'Keep' }])
    await act(async () => { await result.current.handleSavePrompt('New', 'Added') })
    expect(kvSet).toHaveBeenCalledWith('prompts', { data: [{ name: 'Existing', prompt: 'Keep' }, { name: 'New', prompt: 'Added' }], updated_at: expect.any(Number) })
  })
})
