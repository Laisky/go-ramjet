import { beforeEach, describe, expect, it, vi } from 'vitest'

import { importAllData } from '../data-sync'

import * as storageMod from '@/utils/storage'

vi.mock('@/utils/storage', () => {
  const store = new Map<string, any>()

  return {
    kvGet: vi.fn(async (key: string) => store.get(key) ?? null),
    kvSet: vi.fn(async (key: string, val: any) => {
      store.set(key, val)
    }),
    kvDel: vi.fn(async (key: string) => {
      store.delete(key)
    }),
    kvList: vi.fn(async () => Array.from(store.keys())),
    StorageKeys: {
      SESSION_HISTORY_PREFIX: 'chat_user_session_',
      CHAT_DATA_PREFIX: 'chat_data_',
      SELECTED_SESSION: 'config_selected_session',
      DELETED_CHAT_IDS: 'deleted_chat_ids',
    },
    __store: store,
  }
})

const U1 = '00000000-0000-7000-8000-000000000001'
const U2 = '00000000-0000-7000-8000-000000000002'

function chatKey(role: 'user' | 'assistant', chatId: string) {
  return `chat_data_${role}_${chatId}`
}

function historyKey(sessionId: number) {
  return `chat_user_session_${sessionId}`
}

describe('data-sync importAllData (incremental merge)', () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    ;(storageMod as any).__store.clear()
  })

  it('keeps the newer message based on edited_version', async () => {
    // local newer
    await (storageMod as any).kvSet(chatKey('user', 'c1'), {
      chatID: 'c1',
      role: 'user',
      content: 'local',
      edited_version: U2,
      timestamp: 10,
    })

    const cloud = {
      [chatKey('user', 'c1')]: {
        chatID: 'c1',
        role: 'user',
        content: 'cloud',
        edited_version: U1,
        timestamp: 20,
      },
      [historyKey(1)]: [
        { chatID: 'c1', role: 'user', content: 'x', timestamp: 10 },
      ],
    }

    await importAllData(cloud as any, 1)

    const final = await (storageMod as any).kvGet(chatKey('user', 'c1'))
    expect(final.content).toBe('local')
  })

  it('accepts cloud message when local has no edited_version', async () => {
    await (storageMod as any).kvSet(chatKey('assistant', 'c2'), {
      chatID: 'c2',
      role: 'assistant',
      content: 'local',
      edited_version: '',
      timestamp: 10,
    })

    const cloud = {
      [chatKey('assistant', 'c2')]: {
        chatID: 'c2',
        role: 'assistant',
        content: 'cloud',
        edited_version: U1,
        timestamp: 10,
      },
      [historyKey(1)]: [
        { chatID: 'c2', role: 'assistant', content: 'x', timestamp: 10 },
      ],
    }

    await importAllData(cloud as any, 1)

    const final = await (storageMod as any).kvGet(chatKey('assistant', 'c2'))
    expect(final.content).toBe('cloud')
  })

  it('applies deleted_chat_ids before trimming and blocks resurrection', async () => {
    // local has an old chat that will be trimmed away from deleted list
    const oldChatId = 'oldest'
    await (storageMod as any).kvSet(chatKey('user', oldChatId), {
      chatID: oldChatId,
      role: 'user',
      content: 'local',
      timestamp: 1,
    })
    await (storageMod as any).kvSet(historyKey(1), [
      { chatID: oldChatId, role: 'user', content: 'local', timestamp: 1 },
    ])

    const deletedEntries = Array.from({ length: 1001 }).map((_, i) => ({
      chat_id: i === 0 ? oldChatId : `c${i}`,
      deleted_version: `00000000-0000-7000-8000-${i.toString(16).padStart(12, '0')}`,
    }))

    const cloud: any = {
      deleted_chat_ids: deletedEntries,
      [chatKey('user', oldChatId)]: {
        chatID: oldChatId,
        role: 'user',
        content: 'cloud-should-not-resurrect',
        edited_version: U2,
        timestamp: 999,
      },
      [historyKey(1)]: [
        { chatID: oldChatId, role: 'user', content: 'x', timestamp: 999 },
      ],
    }

    await importAllData(cloud, 1)

    // oldChatId must be deleted locally
    const msg = await (storageMod as any).kvGet(chatKey('user', oldChatId))
    expect(msg).toBe(null)

    // deleted list must be trimmed to 1000, so the oldest entry drops
    const savedDeleted = await (storageMod as any).kvGet('deleted_chat_ids')
    expect(savedDeleted).toHaveLength(1000)

    // and history must not contain oldChatId
    const hist = await (storageMod as any).kvGet(historyKey(1))
    expect(hist.find((h: any) => h.chatID === oldChatId)).toBeUndefined()
  })
})
