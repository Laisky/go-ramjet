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
  it('focuses and selects the API key field when opened from the alert', async () => {
    const user = userEvent.setup()
    const { onConfigChange } = renderSidebar('api-key')
    const apiKeyInput = screen.getByLabelText<HTMLInputElement>('API Key')

    await waitFor(() => {
      expect(apiKeyInput).toHaveFocus()
    })
    expect(apiKeyInput.selectionStart).toBe(0)
    expect(apiKeyInput.selectionEnd).toBe(apiKeyInput.value.length)

    await user.paste('sk-re-entered')
    expect(onConfigChange).toHaveBeenLastCalledWith({
      api_token: 'sk-re-entered',
    })
  })

  it('keeps the default focus when opened normally', async () => {
    renderSidebar()

    await waitFor(() => {
      expect(document.activeElement).not.toBe(document.body)
    })
    expect(screen.getByLabelText('API Key')).not.toHaveFocus()
  })
})
