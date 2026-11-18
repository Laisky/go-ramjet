/**
 * Persistence of the GPTChat session display order.
 */
import { kvGet, kvSet, StorageKeys } from '@/utils/storage'

/**
 * getSessionOrderFromKv reads the persisted session display order.
 * It accepts both the legacy bare-array shape and the timestamped object shape,
 * and returns the ordered session IDs (empty when nothing is stored).
 */
export async function getSessionOrderFromKv(): Promise<number[]> {
  const raw = await kvGet<number[] | { data: number[]; updated_at: number }>(
    StorageKeys.SESSION_ORDER,
  )
  return Array.isArray(raw) ? raw : raw?.data || []
}

/**
 * setSessionOrderToKv persists the session display order given by order,
 * stamping it with the current time so sync can pick the newest copy.
 */
export async function setSessionOrderToKv(order: number[]): Promise<void> {
  await kvSet(StorageKeys.SESSION_ORDER, {
    data: order,
    updated_at: Date.now(),
  })
}
