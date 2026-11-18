import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { kvSet, SYNC_KEY } from '../../__tests__/fixtures/credential-store'
import {
  openSidebar,
  storedSession1,
} from '../../__tests__/fixtures/sidebar-harness'
import { McpServerManager } from '../mcp-server-manager'

vi.mock(
  '@/utils/storage',
  () => import('../../__tests__/fixtures/storage-mock'),
)

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

/**
 * openApiKeyEditor clicks "Replace API key" and returns the editor input.
 */
async function openApiKeyEditor(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole('button', { name: 'Replace API key' }))
  return screen.getByLabelText<HTMLInputElement>('API Key', {
    selector: 'input',
  })
}

/**
 * configWrites lists the values persisted for session 1, in order.
 */
function configWrites(): Array<Record<string, unknown>> {
  return kvSet.mock.calls
    .filter(([key]) => key === 'chat_user_config_1')
    .map(([, value]) => value as Record<string, unknown>)
}

describe('ConfigSidebar API key editor', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('stores a typed key only on Save, then shows its hint', async () => {
    const user = userEvent.setup()
    const store = await openSidebar({ api_token: 'sk-original' })
    const editor = await openApiKeyEditor(user)

    await user.type(editor, 'sk-typed-new-key')
    expect(storedSession1(store).api_token).toBe('sk-original')

    await user.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => {
      expect(storedSession1(store).api_token).toBe('sk-typed-new-key')
    })
    expect(screen.getByText('sk-…-key')).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Replace API key' }),
    ).toHaveFocus()
  })

  it('saves a pasted key on Enter and discards a draft on Escape', async () => {
    const user = userEvent.setup()
    const store = await openSidebar({ api_token: 'sk-original' })

    await openApiKeyEditor(user)
    await user.paste('sk-discarded')
    await user.keyboard('{Escape}')
    expect(storedSession1(store).api_token).toBe('sk-original')

    const editor = await openApiKeyEditor(user)
    expect(editor.value).toBe('')
    await user.paste('sk-pasted')
    await user.keyboard('{Enter}')

    await waitFor(() => {
      expect(storedSession1(store).api_token).toBe('sk-pasted')
    })
  })

  it('explains, instead of silently dropping, a pasted sync key', async () => {
    const user = userEvent.setup()
    const store = await openSidebar({ api_token: 'sk-original' })
    const editor = await openApiKeyEditor(user)

    await user.paste(SYNC_KEY)
    await user.click(screen.getByRole('button', { name: 'Save' }))

    expect(await screen.findByRole('alert')).toHaveTextContent(/sync key/i)
    expect(editor).toHaveAttribute('aria-invalid', 'true')
    expect(storedSession1(store).api_token).toBe('sk-original')

    await user.clear(editor)
    await user.paste('sk-fixed')
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => {
      expect(storedSession1(store).api_token).toBe('sk-fixed')
    })
  })

  it('cancels the edit, not the settings, on Escape from the Save button', async () => {
    const user = userEvent.setup()
    const store = await openSidebar({ api_token: 'sk-original' })
    await openApiKeyEditor(user)
    await user.paste(SYNC_KEY)
    const save = screen.getByRole('button', { name: 'Save' })
    await user.click(save)
    expect(await screen.findByRole('alert')).toBeInTheDocument()
    expect(save).toHaveFocus()

    await user.keyboard('{Escape}')

    expect(document.getElementById('cfg-api-key')).toBeNull()
    expect(
      document.querySelector('[role="dialog"][aria-label="Settings"]'),
    ).not.toBeNull()
    expect(
      screen.getByRole('button', { name: 'Replace API key' }),
    ).toHaveFocus()
    expect(storedSession1(store).api_token).toBe('sk-original')
  })

  it('completes a pending reset in the same write that stores the key', async () => {
    const user = userEvent.setup()
    const store = await openSidebar({
      api_token: 'FREETIER-placeholder',
      api_token_reset_required: true,
    })
    const editor = await openApiKeyEditor(user)
    await user.type(editor, 'sk-new-key')
    expect(screen.getByTestId('reset-required')).toHaveTextContent('true')
    const writesBefore = configWrites().length

    await user.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => {
      expect(screen.getByTestId('reset-required')).toHaveTextContent('false')
    })
    const newWrites = configWrites().slice(writesBefore)
    expect(newWrites).toHaveLength(1)
    expect(newWrites[0]).toMatchObject({
      api_token: 'sk-new-key',
      api_token_reset_required: false,
    })
    expect(storedSession1(store).api_token_reset_required).toBe(false)
  })

  it('keeps the reset pending when the editor is cancelled or closed', async () => {
    const user = userEvent.setup()
    const store = await openSidebar({
      api_token: 'FREETIER-placeholder',
      api_token_reset_required: true,
    })
    const editor = await openApiKeyEditor(user)
    await user.type(editor, 'sk-half')
    await user.click(screen.getByRole('button', { name: 'Cancel' }))
    await openApiKeyEditor(user)
    await user.click(screen.getByRole('button', { name: 'Close settings' }))

    await waitFor(() => {
      expect(
        screen.queryByRole('button', { name: 'Replace API key' }),
      ).not.toBeInTheDocument()
    })
    expect(storedSession1(store).api_token).toBe('FREETIER-placeholder')
    expect(storedSession1(store).api_token_reset_required).toBe(true)
  })
})

describe('ConfigSidebar sync key editor', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('refuses an API key as the sync key, whatever its shape', async () => {
    const user = userEvent.setup()
    const store = await openSidebar({ api_token: 'sk-live-key-12345' })

    await user.click(screen.getByRole('button', { name: 'Replace sync key' }))
    await user.paste('sk-live-key-12345')
    await user.click(screen.getByRole('button', { name: 'Save' }))

    expect(await screen.findByRole('alert')).toHaveTextContent(/API key/)
    expect(store.config_sync_key).toBe(SYNC_KEY)
  })

  it('refuses the API key of another session as the sync key', async () => {
    const user = userEvent.setup()
    const store = await openSidebar({ api_token: 'sk-session-one-key' })
    store.chat_user_config_2 = {
      ...storedSession1(store),
      api_token: 'other-session-token',
    }

    await user.click(screen.getByRole('button', { name: 'Replace sync key' }))
    await user.paste('other-session-token')
    await user.click(screen.getByRole('button', { name: 'Save' }))

    expect(await screen.findByRole('alert')).toHaveTextContent(/API key/)
    expect(store.config_sync_key).toBe(SYNC_KEY)
  })

  it('saves a sync key pasted from another device', async () => {
    const user = userEvent.setup()
    const store = await openSidebar({ api_token: 'sk-live-key-12345' })

    await user.click(screen.getByRole('button', { name: 'Replace sync key' }))
    await user.paste('sync-0ther0device0key0123')
    await user.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => {
      expect(store.config_sync_key).toBe('sync-0ther0device0key0123')
    })
  })

  it('warns about a legacy sync key that doubles as an API key', async () => {
    await openSidebar(
      { api_token: 'sk-reused-key-12345' },
      'sk-reused-key-12345',
    )

    expect(
      await screen.findByText(/Your sync key is also an API key/),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Rotate sync key' }),
    ).toBeInTheDocument()
  })

  it('does not warn when the keys are distinct', async () => {
    await openSidebar({ api_token: 'sk-live-key-12345' })

    await waitFor(() => {
      expect(screen.getByText('sync-…z8m3')).toBeInTheDocument()
    })
    expect(screen.queryByText(/Your sync key is also an API key/)).toBeNull()
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
