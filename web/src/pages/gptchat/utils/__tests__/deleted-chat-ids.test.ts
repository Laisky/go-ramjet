import { describe, expect, it } from 'vitest'

import {
  mergeDeletedChatIds,
  normalizeDeletedChatIds,
  trimDeletedChatIds,
} from '../deleted-chat-ids'

describe('deleted-chat-ids', () => {
  it('normalizes legacy string arrays', () => {
    const entries = normalizeDeletedChatIds(['a', 'b'])
    expect(entries).toEqual([
      { chat_id: 'a', deleted_version: '' },
      { chat_id: 'b', deleted_version: '' },
    ])
  })

  it('merges by keeping newest deletion marker', () => {
    const older = [
      { chat_id: 'x', deleted_version: '00000000-0000-7000-8000-000000000001' },
    ]
    const newer = [
      { chat_id: 'x', deleted_version: '00000000-0000-7000-8000-000000000002' },
    ]

    const merged = mergeDeletedChatIds(older, newer)
    expect(merged).toHaveLength(1)
    expect(merged[0].deleted_version).toBe(
      '00000000-0000-7000-8000-000000000002',
    )
  })

  it('trims to the newest N entries', () => {
    const entries = [
      { chat_id: 'a', deleted_version: '00000000-0000-7000-8000-000000000001' },
      { chat_id: 'b', deleted_version: '00000000-0000-7000-8000-000000000002' },
      { chat_id: 'c', deleted_version: '00000000-0000-7000-8000-000000000003' },
    ]

    const trimmed = trimDeletedChatIds(entries, 2)
    expect(trimmed.map((e) => e.chat_id)).toEqual(['b', 'c'])
  })
})
