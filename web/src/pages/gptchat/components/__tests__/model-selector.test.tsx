import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'

import { ModelSelector } from '../model-selector'

describe('ModelSelector availability states', () => {
  it('shows disallowed models as disabled instead of hiding them', async () => {
    const user = userEvent.setup()
    const onModelChange = vi.fn()

    render(
      <ModelSelector
        label="Chat"
        categories={['OpenAI']}
        selectedModel="gpt-4o-mini"
        onModelChange={onModelChange}
        allowedModels={['gpt-4o-mini']}
      />,
    )

    await user.click(screen.getByRole('button', { name: /chat/i }))

    expect(screen.getAllByText('gpt-4o-mini').length).toBeGreaterThan(0)
    const disallowedItem = screen
      .getByText('gpt-5.5')
      .closest('[role="menuitem"]')
    expect(disallowedItem).toHaveAttribute('data-disabled')

    await user.click(screen.getByText('gpt-5.5'))
    expect(onModelChange).not.toHaveBeenCalled()
  })

  it('keeps full model list when wildcard allowlist is used', async () => {
    const user = userEvent.setup()
    const onModelChange = vi.fn()

    render(
      <ModelSelector
        label="Chat"
        categories={['OpenAI']}
        selectedModel="gpt-4o-mini"
        onModelChange={onModelChange}
        allowedModels={['*']}
      />,
    )

    await user.click(screen.getByRole('button', { name: /chat/i }))

    expect(screen.getAllByText('gpt-4o-mini').length).toBeGreaterThan(0)
    expect(screen.getAllByText('gpt-5.5').length).toBeGreaterThan(0)

    await user.click(screen.getByText('gpt-5.5'))
    expect(onModelChange).toHaveBeenCalledWith('gpt-5.5')
  })
})
