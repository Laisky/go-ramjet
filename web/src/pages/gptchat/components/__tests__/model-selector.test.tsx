import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'

import { ModelSelector } from '../model-selector'

describe('ModelSelector allowed-model filtering', () => {
  it('shows only allowlisted models when allowedModels is provided', async () => {
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
    expect(screen.queryByText('gpt-5.4')).not.toBeInTheDocument()
  })

  it('keeps full model list when wildcard allowlist is used', async () => {
    const user = userEvent.setup()

    render(
      <ModelSelector
        label="Chat"
        categories={['OpenAI']}
        selectedModel="gpt-4o-mini"
        onModelChange={vi.fn()}
        allowedModels={['*']}
      />,
    )

    await user.click(screen.getByRole('button', { name: /chat/i }))

    expect(screen.getAllByText('gpt-4o-mini').length).toBeGreaterThan(0)
    expect(screen.getAllByText('gpt-5.4').length).toBeGreaterThan(0)
  })
})
