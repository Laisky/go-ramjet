import { kvGet, kvList, kvSet } from '@/utils/storage'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { type Mock, beforeEach, describe, expect, it, vi } from 'vitest'

import { useConfig } from '../../hooks/use-config'
import { DefaultSessionConfig, type SessionConfig } from '../../types'
import { ConfigSidebar } from '../config-sidebar'
import { McpServerManager } from '../mcp-server-manager'

vi.mock('@/utils/storage', () => ({
  kvGet: vi.fn(),
  kvSet: vi.fn(),
  kvDel: vi.fn(),
  kvList: vi.fn(),
  StorageKeys: {
    SESSION_CONFIG_PREFIX: 'chat_user_config_',
    SELECTED_SESSION: 'config_selected_session',
    VERSION_DATE: 'config_version_date',
    SESSION_HISTORY_PREFIX: 'chat_user_session_',
    SESSION_ORDER: 'config_session_order',
    SYNC_KEY: 'config_sync_key',
  },
}))

vi.mock('@/pages/gptchat/utils/migration', () => ({
  migrateLegacyData: vi.fn().mockResolvedValue(undefined),
}))

vi.mock('@/utils/api', () => ({
  getApiBase: () => 'http://localhost:24456',
}))

vi.mock('../../hooks/use-user', () => ({
  useUser: () => ({
    user: null,
    isLoading: false,
    isError: null,
    mutate: vi.fn(),
  }),
}))

vi.mock('../session-manager', () => ({
  SessionManager: () => <div data-testid="session-manager" />,
}))

vi.mock('../model-selector', () => ({
  ModelSelector: () => <div data-testid="model-selector" />,
}))

vi.mock('../prompt-shortcut-manager', () => ({
  PromptShortcutManager: () => <div data-testid="prompt-shortcuts" />,
}))

const SYNC_KEY = 'sync-k2x9q4w7p1z8m3'

/**
 * SidebarHarness wires the real useConfig hook to the real ConfigSidebar and
 * lets the test close the sidebar like the page does.
 */
function SidebarHarness() {
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
 * openSidebar seeds storage with session 1 built from overrides, renders the
 * sidebar, and returns the live store.
 */
async function openSidebar(
  overrides: Partial<SessionConfig>,
): Promise<Record<string, unknown>> {
  const store: Record<string, unknown> = {
    config_selected_session: 1,
    config_sync_key: SYNC_KEY,
    chat_user_config_1: {
      ...DefaultSessionConfig,
      sync_key: SYNC_KEY,
      ...overrides,
    },
  }
  ;(kvGet as Mock).mockImplementation((key: string) =>
    Promise.resolve(store[key] ?? null),
  )
  ;(kvSet as Mock).mockImplementation(async (key: string, value: unknown) => {
    store[key] = value
  })
  ;(kvList as Mock).mockImplementation(async () => Object.keys(store))

  render(<SidebarHarness />)
  await screen.findByLabelText('API Key')
  return store
}

/**
 * storedConfig reads the persisted config of session 1.
 */
function storedConfig(store: Record<string, unknown>): SessionConfig {
  return store.chat_user_config_1 as SessionConfig
}

describe('ConfigSidebar API key feedback', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('explains, instead of silently dropping, a pasted sync key', async () => {
    const user = userEvent.setup()
    const store = await openSidebar({ api_token: 'sk-original' })
    const apiKeyInput = screen.getByLabelText<HTMLInputElement>('API Key')

    await user.tripleClick(apiKeyInput)
    await user.paste(SYNC_KEY)

    expect(await screen.findByRole('alert')).toHaveTextContent(/sync key/i)
    expect(apiKeyInput).toHaveAttribute('aria-invalid', 'true')
    expect(apiKeyInput.value).toBe('sk-original')
    expect(storedConfig(store).api_token).toBe('sk-original')

    await user.tripleClick(apiKeyInput)
    await user.paste('sk-fixed')

    await waitFor(() => {
      expect(storedConfig(store).api_token).toBe('sk-fixed')
    })
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    expect(apiKeyInput).not.toHaveAttribute('aria-invalid', 'true')
  })

  it('keeps a pending reset until the user leaves the API key field', async () => {
    const user = userEvent.setup()
    const store = await openSidebar({
      api_token: 'FREETIER-placeholder',
      api_token_reset_required: true,
    })
    const apiKeyInput = screen.getByLabelText<HTMLInputElement>('API Key')

    await user.tripleClick(apiKeyInput)
    await user.paste('sk-new-key')

    await waitFor(() => {
      expect(storedConfig(store).api_token).toBe('sk-new-key')
    })
    expect(screen.getByTestId('reset-required')).toHaveTextContent('true')

    await user.click(screen.getByLabelText('Sync Key'))

    await waitFor(() => {
      expect(screen.getByTestId('reset-required')).toHaveTextContent('false')
    })
    expect(storedConfig(store).api_token_reset_required).toBe(false)
  })

  it('completes a pending reset when the sidebar is closed', async () => {
    const user = userEvent.setup()
    const store = await openSidebar({
      api_token: 'FREETIER-placeholder',
      api_token_reset_required: true,
    })
    const apiKeyInput = screen.getByLabelText<HTMLInputElement>('API Key')

    await user.tripleClick(apiKeyInput)
    await user.paste('sk-new-key')
    await user.keyboard('{Escape}')

    await waitFor(() => {
      expect(storedConfig(store).api_token_reset_required).toBe(false)
    })
  })

  it('keeps the reset pending when the field is left without a real key', async () => {
    const user = userEvent.setup()
    const store = await openSidebar({
      api_token: 'FREETIER-placeholder',
      api_token_reset_required: true,
    })

    await user.click(screen.getByLabelText('API Key'))
    await user.click(screen.getByLabelText('Sync Key'))
    await user.keyboard('{Escape}')

    await waitFor(() => {
      expect(screen.queryByLabelText('API Key')).not.toBeInTheDocument()
    })
    expect(storedConfig(store).api_token_reset_required).toBe(true)
  })
})

describe('McpServerManager accessibility', () => {
  it('gives every server form field an accessible name', async () => {
    const user = userEvent.setup()
    render(<McpServerManager servers={[]} onChange={vi.fn()} />)

    await user.click(screen.getByRole('button', { name: 'Add MCP server' }))

    expect(
      screen.getByRole('textbox', { name: 'MCP server name' }),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('textbox', { name: 'MCP server URL' }),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('textbox', { name: 'MCP server API key' }),
    ).toBeInTheDocument()
  })
})
