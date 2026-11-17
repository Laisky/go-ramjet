import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'

import { DefaultSessionConfig } from '../../types'
import { ApiKeyResetAlert } from '../api-key-reset-alert'
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
  SessionManager: () => <button type="button">session</button>,
}))

vi.mock('../model-selector', () => ({
  ModelSelector: () => <div data-testid="model-selector" />,
}))

vi.mock('../prompt-shortcut-manager', () => ({
  PromptShortcutManager: () => <div data-testid="prompt-shortcuts" />,
}))

describe('ApiKeyResetAlert', () => {
  it('announces the lost key and explains how to re-enter it', () => {
    render(<ApiKeyResetAlert onResetApiKey={vi.fn()} onDismiss={vi.fn()} />)

    const alert = screen.getByRole('alert', {
      name: 'Your API key needs to be re-entered',
    })
    expect(alert).toHaveTextContent('Settings → API Key')
    expect(alert).toHaveTextContent('switched to the free tier')
    expect(screen.getByRole('link', { name: 'API key guide' })).toHaveAttribute(
      'href',
      'https://wiki.laisky.com/projects/gpt/pay/#page_gpt_pay',
    )
  })

  it('routes the two actions to their callbacks', async () => {
    const user = userEvent.setup()
    const onResetApiKey = vi.fn()
    const onDismiss = vi.fn()
    render(
      <ApiKeyResetAlert onResetApiKey={onResetApiKey} onDismiss={onDismiss} />,
    )

    await user.click(screen.getByRole('button', { name: 'Re-enter API key' }))
    expect(onResetApiKey).toHaveBeenCalledTimes(1)

    await user.click(screen.getByRole('button', { name: 'Keep free tier' }))
    expect(onDismiss).toHaveBeenCalledTimes(1)
  })

  it('offers sibling keys only as masked, explicit choices', async () => {
    const user = userEvent.setup()
    const onUseChoice = vi.fn()
    render(
      <ApiKeyResetAlert
        onResetApiKey={vi.fn()}
        onDismiss={vi.fn()}
        onUseChoice={onUseChoice}
        choices={[
          { sourceSessionId: 2, hint: 'sk-…aaaa', sessionNames: ['Work'] },
          {
            sourceSessionId: 5,
            hint: 'sk-…bbbb',
            sessionNames: ['Home', 'Drafts'],
          },
        ]}
      />,
    )

    expect(screen.getByRole('alert')).toHaveTextContent(
      'Use one only if it belongs to the account this session used',
    )
    await user.click(
      screen.getByRole('button', { name: 'Use sk-…bbbb from Home, Drafts' }),
    )
    expect(onUseChoice).toHaveBeenCalledWith(5)
  })

  it('shows no choice list when no sibling holds a key', () => {
    render(
      <ApiKeyResetAlert
        onResetApiKey={vi.fn()}
        onDismiss={vi.fn()}
        onUseChoice={vi.fn()}
        choices={[]}
      />,
    )
    expect(screen.queryByRole('button', { name: /^Use / })).toBeNull()
  })
})

/**
 * renderSidebar mounts the open ConfigSidebar with the given initial focus.
 */
function renderSidebar(initialFocus?: 'api-key') {
  const onConfigChange = vi.fn()
  render(
    <ConfigSidebar
      isOpen={true}
      onClose={vi.fn()}
      initialFocus={initialFocus}
      config={{ ...DefaultSessionConfig, api_token: 'FREETIER-placeholder' }}
      onConfigChange={onConfigChange}
      onClearChats={vi.fn()}
      onReset={vi.fn()}
      onExportData={vi.fn().mockResolvedValue({})}
      onImportData={vi.fn().mockResolvedValue(undefined)}
    />,
  )
  return { onConfigChange }
}

describe('ConfigSidebar initial focus', () => {
  it('opens an empty, focused API key editor when opened from the alert', async () => {
    const user = userEvent.setup()
    const { onConfigChange } = renderSidebar('api-key')
    const apiKeyInput = await screen.findByLabelText<HTMLInputElement>(
      'API Key',
      { selector: 'input' },
    )

    await waitFor(() => {
      expect(apiKeyInput).toHaveFocus()
    })
    expect(apiKeyInput.value).toBe('')

    await user.paste('sk-re-entered')
    expect(onConfigChange).not.toHaveBeenCalled()

    await user.click(screen.getByRole('button', { name: 'Save' }))
    expect(onConfigChange).toHaveBeenCalledExactlyOnceWith({
      api_token: 'sk-re-entered',
    })
  })

  it('shows only a masked hint when opened normally', async () => {
    renderSidebar()

    await waitFor(() => {
      expect(document.activeElement).not.toBe(document.body)
    })
    expect(document.getElementById('cfg-api-key')).toBeNull()
    expect(screen.getByText('FREETIER-…lder')).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Replace API key' }),
    ).not.toHaveFocus()
  })
})
