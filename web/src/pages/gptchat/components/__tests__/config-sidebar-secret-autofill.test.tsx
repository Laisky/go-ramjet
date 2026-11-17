import { kvGet, kvList, kvSet } from '@/utils/storage'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { type Mock, beforeEach, describe, expect, it, vi } from 'vitest'

import {
  SECRET_INPUT_MASK_CLASS,
  SECRET_INPUT_OPT_OUT_ATTRIBUTES,
} from '@/components/ui/secret-input-policy'
import { useConfig } from '../../hooks/use-config'
import { DefaultSessionConfig, type SessionConfig } from '../../types'
import { ConfigSidebar } from '../config-sidebar'

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

const API_TOKEN = 'sk-original-api-key'
const SYNC_KEY = 'sync-k2x9q4w7p1z8m3'

/**
 * simulatePasswordManagerFill mimics a browser password manager filling a
 * saved site password into a field that just appeared: it writes the value
 * through the native setter (bypassing React) and dispatches the input and
 * change events browsers fire, without moving focus into the field.
 */
function simulatePasswordManagerFill(input: HTMLInputElement, value: string) {
  const setter = Object.getOwnPropertyDescriptor(
    HTMLInputElement.prototype,
    'value',
  )?.set
  setter?.call(input, value)
  input.dispatchEvent(new Event('input', { bubbles: true }))
  input.dispatchEvent(new Event('change', { bubbles: true }))
}

/**
 * SidebarHarness wires the real useConfig hook to the real ConfigSidebar, the
 * same way the GPTChat page does once the user clicks "Settings".
 */
function SidebarHarness() {
  const { config, updateConfig, isLoading } = useConfig()
  if (isLoading) return null

  return (
    <ConfigSidebar
      isOpen={true}
      onClose={vi.fn()}
      config={config}
      onConfigChange={updateConfig}
      onClearChats={vi.fn()}
      onReset={vi.fn()}
      onExportData={vi.fn().mockResolvedValue({})}
      onImportData={vi.fn().mockResolvedValue(undefined)}
    />
  )
}

/**
 * openSidebarWithStore seeds storage with a session holding API_TOKEN and
 * SYNC_KEY, renders the sidebar, and returns the live store.
 */
async function openSidebarWithStore(): Promise<Record<string, unknown>> {
  const store: Record<string, unknown> = {
    config_selected_session: 1,
    config_sync_key: SYNC_KEY,
    chat_user_config_1: {
      ...DefaultSessionConfig,
      api_token: API_TOKEN,
      sync_key: SYNC_KEY,
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
 * storedApiToken reads the persisted API token of session 1.
 */
function storedApiToken(store: Record<string, unknown>): string {
  return (store.chat_user_config_1 as SessionConfig).api_token
}

describe('ConfigSidebar secret fields vs password-manager autofill', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('keeps the API key when a password manager fills the sync key into it on open', async () => {
    const store = await openSidebarWithStore()
    const apiKeyInput = screen.getByLabelText<HTMLInputElement>('API Key')

    simulatePasswordManagerFill(apiKeyInput, SYNC_KEY)

    await waitFor(() => {
      expect(apiKeyInput.value).toBe(API_TOKEN)
    })
    expect(storedApiToken(store)).toBe(API_TOKEN)
  })

  it('keeps the sync key when a password manager fills it while unfocused', async () => {
    const store = await openSidebarWithStore()
    const syncKeyInput = screen.getByLabelText<HTMLInputElement>('Sync Key')

    simulatePasswordManagerFill(syncKeyInput, 'saved-site-password')

    await waitFor(() => {
      expect(syncKeyInput.value).toBe(SYNC_KEY)
    })
    expect(store.config_sync_key).toBe(SYNC_KEY)
  })

  it('still saves an API key the user types', async () => {
    const user = userEvent.setup()
    const store = await openSidebarWithStore()
    const apiKeyInput = screen.getByLabelText<HTMLInputElement>('API Key')

    await user.clear(apiKeyInput)
    await user.type(apiKeyInput, 'sk-typed')

    await waitFor(() => {
      expect(storedApiToken(store)).toBe('sk-typed')
    })
    expect(apiKeyInput.value).toBe('sk-typed')
  })

  it('still saves an API key the user pastes', async () => {
    const user = userEvent.setup()
    const store = await openSidebarWithStore()
    const apiKeyInput = screen.getByLabelText<HTMLInputElement>('API Key')

    await user.tripleClick(apiKeyInput)
    await user.paste('sk-pasted')

    await waitFor(() => {
      expect(storedApiToken(store)).toBe('sk-pasted')
    })
  })

  it('renders no password-type field that a password manager could target', async () => {
    await openSidebarWithStore()

    expect(document.querySelectorAll('input[type="password"]')).toHaveLength(0)
  })

  it('marks every secret field as a masked non-credential', async () => {
    await openSidebarWithStore()

    const secretInputs = [
      screen.getByLabelText<HTMLInputElement>('API Key'),
      screen.getByLabelText<HTMLInputElement>('Sync Key'),
    ]
    const names = new Set<string>()
    for (const input of secretInputs) {
      expect(input.type).toBe('text')
      expect(input).toHaveClass(SECRET_INPUT_MASK_CLASS)
      expect(input.name).not.toMatch(/pass|pwd/i)
      expect(input.getAttribute('autocomplete')).toBe(
        SECRET_INPUT_OPT_OUT_ATTRIBUTES.autoComplete,
      )
      for (const [attr, value] of Object.entries(
        SECRET_INPUT_OPT_OUT_ATTRIBUTES,
      )) {
        if (attr.startsWith('data-')) {
          expect(input.getAttribute(attr)).toBe(value)
        }
      }
      names.add(input.name)
    }
    expect(names.size).toBe(secretInputs.length)
  })

  it('reveals and hides the API key by masking, never by switching type', async () => {
    const user = userEvent.setup()
    await openSidebarWithStore()
    const apiKeyInput = screen.getByLabelText<HTMLInputElement>('API Key')

    await user.click(screen.getByRole('button', { name: 'Show API key' }))
    expect(apiKeyInput).not.toHaveClass(SECRET_INPUT_MASK_CLASS)
    expect(apiKeyInput.type).toBe('text')
    expect(apiKeyInput.value).toBe(API_TOKEN)

    await user.click(screen.getByRole('button', { name: 'Hide API key' }))
    expect(apiKeyInput).toHaveClass(SECRET_INPUT_MASK_CLASS)
    expect(apiKeyInput.type).toBe('text')
  })
})
