import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'

import { DefaultSessionConfig } from '../../types'
import { ConfigSidebar } from '../config-sidebar'

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

vi.mock('../dataset-manager', () => ({
  DatasetManager: () => <div data-testid="dataset-manager" />,
}))

vi.mock('../mcp-server-manager', () => ({
  McpServerManager: () => <div data-testid="mcp-manager" />,
}))

vi.mock('../data-sync-manager', () => ({
  DataSyncManager: () => <div data-testid="sync-manager" />,
}))

describe('ConfigSidebar destructive actions', () => {
  it('confirms before clearing current session chats', async () => {
    const onClearChats = vi.fn()
    const user = userEvent.setup()

    render(
      <ConfigSidebar
        isOpen={true}
        onClose={vi.fn()}
        config={DefaultSessionConfig}
        onConfigChange={vi.fn()}
        onClearChats={onClearChats}
        onReset={vi.fn()}
        onExportData={vi.fn().mockResolvedValue({})}
        onImportData={vi.fn().mockResolvedValue(undefined)}
      />,
    )

    await user.click(screen.getByRole('button', { name: 'Clear Chats' }))

    expect(screen.getByText('Clear Chat History')).toBeInTheDocument()
    expect(onClearChats).not.toHaveBeenCalled()

    await user.click(screen.getByRole('button', { name: 'Confirm' }))
    expect(onClearChats).toHaveBeenCalledTimes(1)
  })

  it('confirms before purging all sessions', async () => {
    const onPurgeAllSessions = vi.fn()
    const user = userEvent.setup()

    render(
      <ConfigSidebar
        isOpen={true}
        onClose={vi.fn()}
        config={DefaultSessionConfig}
        onConfigChange={vi.fn()}
        onClearChats={vi.fn()}
        onReset={vi.fn()}
        onExportData={vi.fn().mockResolvedValue({})}
        onImportData={vi.fn().mockResolvedValue(undefined)}
        onPurgeAllSessions={onPurgeAllSessions}
      />,
    )

    await user.click(screen.getByRole('button', { name: 'Purge All' }))

    expect(screen.getByText('Purge All Sessions')).toBeInTheDocument()
    expect(onPurgeAllSessions).not.toHaveBeenCalled()

    await user.click(screen.getByRole('button', { name: 'Confirm' }))
    expect(onPurgeAllSessions).toHaveBeenCalledTimes(1)
  })
})
