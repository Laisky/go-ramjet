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
    expect(disallowedItem).toHaveTextContent('Unavailable')

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

describe('ModelSelector retired research', () => {
  it('offers ordinary models without the retired custom research option', async () => {
    const user = userEvent.setup()
    render(
      <ModelSelector
        label="Chat"
        selectedModel="gpt-4o-mini"
        onModelChange={vi.fn()}
      />,
    )
    await user.click(screen.getByRole('button', { name: /chat/i }))
    expect(screen.queryByText('deep-research')).not.toBeInTheDocument()
    expect(screen.getAllByText('gpt-4o-mini').length).toBeGreaterThan(0)
  })
})

describe('ModelSelector current One API models', () => {
  it('selects the current Sol ID and preserves the previous selection', async () => {
    const user = userEvent.setup()
    const onModelChange = vi.fn()
    render(
      <ModelSelector
        label="Chat"
        categories={['OpenAI']}
        selectedModel="gpt-6.1-sol"
        onModelChange={onModelChange}
        allowedModels={['gpt-6.1-sol', 'gpt-6.2-sol']}
      />,
    )

    expect(screen.getByRole('button', { name: /chat/i })).toHaveTextContent(
      'gpt-6.1-sol',
    )
    expect(onModelChange).not.toHaveBeenCalled()

    await user.click(screen.getByRole('button', { name: /chat/i }))
    await user.click(screen.getByRole('menuitem', { name: 'gpt-6.2-sol' }))
    expect(onModelChange).toHaveBeenCalledExactlyOnceWith('gpt-6.2-sol')
  })

  it('does not grant the successor to an account restricted to the previous ID', async () => {
    const user = userEvent.setup()
    const onModelChange = vi.fn()
    render(
      <ModelSelector
        label="Chat"
        categories={['Anthropic']}
        selectedModel="claude-haiku-4-5"
        onModelChange={onModelChange}
        allowedModels={['claude-haiku-4-5']}
      />,
    )

    await user.click(screen.getByRole('button', { name: /chat/i }))
    const successor = screen.getByRole('menuitem', { name: 'claude-haiku-5-5' })
    expect(successor).toHaveAttribute('data-disabled')
    await user.click(successor)
    expect(onModelChange).not.toHaveBeenCalled()
  })
})
