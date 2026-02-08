import { describe, expect, it } from 'vitest'

import { getConfirmActionCopy } from '../confirm-action'

describe('getConfirmActionCopy', () => {
  it('returns destructive preset for clear history', () => {
    const copy = getConfirmActionCopy('clear-chat-history')

    expect(copy.title).toBe('Clear Chat History')
    expect(copy.variant).toBe('destructive')
  })

  it('returns destructive preset for purge all', () => {
    const copy = getConfirmActionCopy('purge-all-sessions')

    expect(copy.title).toBe('Purge All Sessions')
    expect(copy.variant).toBe('destructive')
  })

  it('returns destructive preset for message deletion with explicit confirm text', () => {
    const copy = getConfirmActionCopy('delete-message')

    expect(copy.title).toBe('Delete Message')
    expect(copy.confirmText).toBe('Delete')
    expect(copy.variant).toBe('destructive')
  })

  it('injects session name for delete-session copy', () => {
    const copy = getConfirmActionCopy('delete-session', {
      sessionName: 'Session A',
    })

    expect(copy.description).toContain('"Session A"')
    expect(copy.variant).toBe('destructive')
  })
})
