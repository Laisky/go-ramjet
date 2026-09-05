import { beforeEach, describe, expect, it, vi } from 'vitest'

// An in-memory store whose reads and writes yield to the event loop. Real
// IndexedDB access is asynchronous, so a writer that reads the history index,
// mutates it and writes it back can be interleaved by another writer. A store
// that resolves synchronously hides exactly the bug this module exists to stop.
const store = new Map<string, unknown>()
const tick = () => new Promise((resolve) => setTimeout(resolve, 0))

vi.mock('@/utils/storage', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/utils/storage')>()
  return {
    ...actual,
    // Spreading alone dropped StorageKeys, which the key helpers need.
    StorageKeys: actual.StorageKeys,
    kvGet: async (key: string) => {
      await tick()
      return store.has(key) ? structuredClone(store.get(key)) : null
    },
    kvSet: async (key: string, value: unknown) => {
      await tick()
      store.set(key, structuredClone(value))
    },
  }
})

const { appendMessageToSession } = await import('../chat-append')
const { getChatDataKey, getSessionHistoryKey } =
  await import('../../utils/chat-storage')
type ChatMessageData = import('../../types').ChatMessageData
type SessionHistoryItem = import('../../types').SessionHistoryItem

/** message builds a minimal stored message for a turn. */
function message(
  chatID: string,
  role: 'user' | 'assistant',
  content: string,
): ChatMessageData {
  return { chatID, role, content, timestamp: 1 }
}

/** history reads the ordered index for a session. */
function history(sessionId: number): SessionHistoryItem[] {
  return (store.get(getSessionHistoryKey(sessionId)) ??
    []) as SessionHistoryItem[]
}

describe('appendMessageToSession', () => {
  beforeEach(() => {
    store.clear()
  })

  it('writes to the session it is given, not an ambient one', async () => {
    await appendMessageToSession(7, message('c1', 'user', 'pinned'))
    expect(history(7).map((h) => h.chatID)).toEqual(['c1'])
    expect(history(8)).toEqual([])
  })

  it('keeps a turn ordered when its text arrives after a later message', async () => {
    // A voice turn reserves its slot before transcription returns.
    await appendMessageToSession(7, message('c1', 'user', ''))
    await appendMessageToSession(7, message('c1', 'assistant', 'answer'))
    // The caller's transcript lands last but must not move to the end.
    await appendMessageToSession(7, message('c1', 'user', 'question'))

    expect(history(7).map((h) => `${h.role}:${h.content}`)).toEqual([
      'user:question',
      'assistant:answer',
    ])
    expect(
      (store.get(getChatDataKey('c1', 'user')) as ChatMessageData).content,
    ).toBe('question')
  })

  it('does not lose a history entry when commits overlap', async () => {
    // Three commits started together. Each reads the index, appends to it and
    // writes it back, so without serialization the later writes clobber the
    // earlier ones and their messages become unreachable.
    await Promise.all([
      appendMessageToSession(7, message('a', 'user', 'one')),
      appendMessageToSession(7, message('b', 'user', 'two')),
      appendMessageToSession(7, message('c', 'user', 'three')),
    ])
    expect(
      history(7)
        .map((h) => h.chatID)
        .sort(),
    ).toEqual(['a', 'b', 'c'])
  })
})
