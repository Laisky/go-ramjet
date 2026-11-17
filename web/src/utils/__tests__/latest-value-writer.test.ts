import { describe, expect, it } from 'vitest'

import { createLatestValueWriter } from '../latest-value-writer'

/**
 * deferred returns a promise with its resolve and reject functions.
 */
function deferred() {
  let resolve: () => void = () => {}
  let reject: (error: Error) => void = () => {}
  const promise = new Promise<void>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

describe('createLatestValueWriter', () => {
  it('never lets an older value land after a newer one', async () => {
    const stored = new Map<string, number>()
    const firstWrite = deferred()
    let calls = 0
    const writer = createLatestValueWriter<number>(async (key, value) => {
      calls += 1
      if (calls === 1) await firstWrite.promise // e.g. an IndexedDB retry
      stored.set(key, value)
    })

    const first = writer.write('cfg', 1)
    const second = writer.write('cfg', 2)
    firstWrite.resolve()
    await Promise.all([first, second])

    expect(stored.get('cfg')).toBe(2)
  })

  it('runs one write at a time, in call order', async () => {
    let inFlight = 0
    let maxInFlight = 0
    const order: string[] = []
    const writer = createLatestValueWriter<string>(async (key, value) => {
      inFlight += 1
      maxInFlight = Math.max(maxInFlight, inFlight)
      await Promise.resolve()
      order.push(`${key}=${value}`)
      inFlight -= 1
    })

    await Promise.all([
      writer.write('sync', 'k1'),
      writer.write('cfg', 'c1'),
      writer.write('other', 'o1'),
    ])

    expect(maxInFlight).toBe(1)
    expect(order).toEqual(['sync=k1', 'cfg=c1', 'other=o1'])
  })

  it('coalesces queued values of one key into the newest', async () => {
    const gate = deferred()
    const persisted: number[] = []
    const writer = createLatestValueWriter<number>(async (_key, value) => {
      await gate.promise
      persisted.push(value)
    })

    const writes = [1, 2, 3].map((value) => writer.write('cfg', value))
    gate.resolve()
    await Promise.all(writes)

    expect(persisted).toEqual([3])
  })

  it('reports a failure to its caller and retries the value on the next write', async () => {
    const stored = new Map<string, string>()
    let failNext = true
    const writer = createLatestValueWriter<string>(async (key, value) => {
      if (failNext) {
        failNext = false
        throw new Error('transient')
      }
      stored.set(key, value)
    })

    await expect(writer.write('cfg', 'v1')).rejects.toThrow('transient')
    expect(stored.has('cfg')).toBe(false)

    await writer.write('other', 'x')
    expect(stored.has('cfg')).toBe(false)

    await writer.write('cfg', 'v2')
    expect(stored.get('cfg')).toBe('v2')
  })

  it('keeps a failed value for a queued write of the same key', async () => {
    const stored = new Map<string, string>()
    const gate = deferred()
    let calls = 0
    const writer = createLatestValueWriter<string>(async (key, value) => {
      calls += 1
      if (calls === 1) {
        await gate.promise
        throw new Error('transient')
      }
      stored.set(key, value)
    })

    const first = writer.write('cfg', 'v1')
    const blocker = writer.write('other', 'x')
    const third = writer.write('cfg', 'v1-again')
    gate.resolve()

    await expect(first).rejects.toThrow('transient')
    await Promise.all([blocker, third])
    expect(stored.get('cfg')).toBe('v1-again')
  })

  it('resolves whenIdle only after every scheduled write settles', async () => {
    const gate = deferred()
    let done = false
    const writer = createLatestValueWriter<number>(async () => {
      await gate.promise
      done = true
    })

    void writer.write('cfg', 1)
    const idle = writer.whenIdle()
    gate.resolve()
    await idle

    expect(done).toBe(true)
  })
})
