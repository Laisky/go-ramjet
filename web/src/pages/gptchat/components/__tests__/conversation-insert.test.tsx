import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'

import { ConversationInsert } from '../conversation-insert'

describe('ConversationInsert', () => {
  it('shows an accessible insertion action and invokes it', async () => {
    const user = userEvent.setup()
    const onInsert = vi.fn()

    render(<ConversationInsert onInsert={onInsert} />)

    const button = screen.getByRole('button', { name: 'Insert message here' })
    expect(button).toHaveAttribute('title', 'Insert a message at this point')

    await user.click(button)

    expect(onInsert).toHaveBeenCalledOnce()
  })

  it('disables insertion while a response is running', () => {
    render(<ConversationInsert onInsert={vi.fn()} disabled />)

    expect(
      screen.getByRole('button', { name: 'Insert message here' }),
    ).toBeDisabled()
  })
})
