/* eslint-disable react-refresh/only-export-components -- test fixture, never hot-reloaded */
/**
 * Settings-sidebar harness shared by GPTChat credential UI tests. Pair it
 * with storage-mock.ts and mock '../../hooks/use-user', the session
 * manager, the model selector, and the prompt-shortcut manager in the test
 * file.
 */
import { render, screen } from '@testing-library/react'
import { useState } from 'react'
import { vi } from 'vitest'

import { ConfigSidebar } from '../../components/config-sidebar'
import { useConfig } from '../../hooks/use-config'
import type { SessionConfig } from '../../types'
import { sessionConfig, SYNC_KEY } from './credential-store'
import { installStore } from './storage-mock'

/**
 * SidebarHarness wires the real useConfig hook to the real ConfigSidebar
 * the way the GPTChat page does, exposes the reset flag for assertions,
 * and lets the test close the sidebar.
 */
export function SidebarHarness() {
  const { config, updateConfig, isLoading } = useConfig()
  const [open, setOpen] = useState(true)
  if (isLoading) return null

  return (
    <>
      <output data-testid="reset-required">
        {String(config.api_token_reset_required === true)}
      </output>
      <ConfigSidebar
        isOpen={open}
        onClose={() => setOpen(false)}
        config={config}
        onConfigChange={updateConfig}
        onClearChats={vi.fn()}
        onReset={vi.fn()}
        onExportData={vi.fn().mockResolvedValue({})}
        onImportData={vi.fn().mockResolvedValue(undefined)}
      />
    </>
  )
}

/**
 * openSidebar seeds storage with session 1 built from overrides (sharing
 * SYNC_KEY unless overridden by syncKey), renders the sidebar, and waits
 * for it. It returns the live store.
 */
export async function openSidebar(
  overrides: Partial<SessionConfig>,
  syncKey: string = SYNC_KEY,
): Promise<Record<string, unknown>> {
  const store = installStore({
    config_selected_session: 1,
    config_sync_key: syncKey,
    chat_user_config_1: sessionConfig({ sync_key: syncKey, ...overrides }),
  })
  render(<SidebarHarness />)
  await screen.findByRole('button', { name: /^(Replace|Set) API key$/ })
  return store
}

/**
 * storedSession1 returns the persisted config of session 1.
 */
export function storedSession1(store: Record<string, unknown>): SessionConfig {
  return store.chat_user_config_1 as SessionConfig
}
