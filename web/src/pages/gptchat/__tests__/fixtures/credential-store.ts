/**
 * Shared session-config fixtures for GPTChat credential tests. Pair it with
 * storage-mock.ts, which stands in for '@/utils/storage'.
 */
import { renderHook, waitFor } from '@testing-library/react'
import { expect } from 'vitest'

import { useConfig } from '../../hooks/use-config'
import { DefaultSessionConfig, type SessionConfig } from '../../types'
import { StorageKeys } from './storage-mock'

export { installStore, kvDel, kvGet, kvList, kvSet } from './storage-mock'

/** SYNC_KEY is the generated-shape sync key most fixtures share. */
export const SYNC_KEY = 'sync-k2x9q4w7p1z8m3'
/** PROVIDER_A and PROVIDER_B are two distinct API endpoints. */
export const PROVIDER_A = 'https://provider-a.example'
export const PROVIDER_B = 'https://provider-b.example'

/**
 * sessionConfig builds a stored session config that shares SYNC_KEY, with
 * overrides applied on top. It returns the config.
 */
export function sessionConfig(
  overrides: Partial<SessionConfig> = {},
): SessionConfig {
  return {
    ...DefaultSessionConfig,
    sync_key: SYNC_KEY,
    ...overrides,
  }
}

/**
 * storedConfig returns the persisted config of sessionId in store.
 */
export function storedConfig(
  store: Record<string, unknown>,
  sessionId: number,
): SessionConfig {
  return store[
    `${StorageKeys.SESSION_CONFIG_PREFIX}${sessionId}`
  ] as SessionConfig
}

/**
 * renderLoadedConfig mounts useConfig and waits until hydration has
 * finished. It returns the renderHook handle.
 */
export async function renderLoadedConfig() {
  const hook = renderHook(() => useConfig())
  await waitFor(() => {
    expect(hook.result.current.isLoading).toBe(false)
  })
  return hook
}
