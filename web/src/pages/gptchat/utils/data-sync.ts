/**
 * Utility functions for exporting and importing chat data.
 */
import { kvList, kvGet, kvSet } from '@/utils/storage'
import { getSessionConfigKey } from './config-helpers'

/**
 * Export all data (sessions, configs, shortcuts) for sync
 */
export async function exportAllData(): Promise<Record<string, unknown>> {
  const keys = await kvList()
  const data: Record<string, unknown> = {}

  // keys to exclude
  const excludeKeys = ['MIGRATE_V1_COMPLETED']

  for (const key of keys) {
    if (excludeKeys.includes(key)) continue
    data[key] = await kvGet(key)
  }

  return data
}

/**
 * Import data (overwrite existing)
 */
export async function importAllData(
  data: Record<string, unknown>,
  sessionId: number,
): Promise<void> {
  // Clear existing data to ensure clean state (optional, but safer for full sync)
  // await kvClear() // Maybe too aggressive? Let's just overwrite.

  for (const [key, val] of Object.entries(data)) {
    await kvSet(key, val)
  }

  // Force reload config if current session was updated
  const currentKey = getSessionConfigKey(sessionId)
  if (data[currentKey]) {
    // Simple way: trigger reload by toggling a dummy state or just re-running load
    window.location.reload() // Easiest way to ensure all states (history, config) are updated
  }
}
