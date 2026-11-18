/**
 * Ordered, coalescing persistence for values that are rewritten often.
 */

/**
 * LatestValueWriter persists values by key, one write at a time, in call
 * order. write resolves once the value passed, or a newer value for the same
 * key, has been stored; when a write fails, its value stays pending and the
 * next write of that key retries it. whenIdle resolves once every write
 * scheduled so far has settled, so a reader can see storage that is up to
 * date.
 */
export interface LatestValueWriter<T> {
  write: (key: string, value: T) => Promise<void>
  whenIdle: () => Promise<void>
}

/**
 * createLatestValueWriter wraps persist, which stores one value under one
 * key, so writes never run concurrently and an older value can never land
 * after a newer one, even when persist retries internally. Values scheduled
 * for a key while an earlier write of that key is still queued are
 * coalesced into a single write of the newest value.
 * It returns the writer.
 */
export function createLatestValueWriter<T>(
  persist: (key: string, value: T) => Promise<void>,
): LatestValueWriter<T> {
  const pending = new Map<string, T>()
  let tail: Promise<void> = Promise.resolve()

  const write = (key: string, value: T): Promise<void> => {
    pending.set(key, value)
    const run = tail.then(async () => {
      if (!pending.has(key)) return // A queued run already stored it.
      const latest = pending.get(key) as T
      pending.delete(key)
      try {
        await persist(key, latest)
      } catch (error) {
        // Keep the value for the next write of this key unless a newer one
        // has been scheduled meanwhile.
        if (!pending.has(key)) pending.set(key, latest)
        throw error
      }
    })
    tail = run.catch(() => undefined)
    return run
  }

  return {
    write,
    whenIdle: () => tail,
  }
}
