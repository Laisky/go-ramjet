import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'

import { ChatModelGPT5Dot6 } from '../../models'
import { ModelSelector } from '../model-selector'

// A model that is in the OpenAI category but not in the narrow allowlist below.
// Imported rather than hardcoded so catalog updates cannot silently break this.
const otherModel = ChatModelGPT5Dot6

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
      .getByText(otherModel)
      .closest('[role="menuitem"]')
    expect(disallowedItem).toHaveAttribute('data-disabled')

    await user.click(screen.getByText(otherModel))
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
    expect(screen.getAllByText(otherModel).length).toBeGreaterThan(0)

    await user.click(screen.getByText(otherModel))
    expect(onModelChange).toHaveBeenCalledWith(otherModel)
  })
})
