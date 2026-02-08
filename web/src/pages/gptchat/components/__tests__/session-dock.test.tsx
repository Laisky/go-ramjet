import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'

import { SessionDock } from '../session-dock'

describe('SessionDock', () => {
  it('opens a confirmation dialog before clearing chats from dock trash', async () => {
    const onSwitchSession = vi.fn()
    const onCreateSession = vi.fn()
    const onClearChats = vi.fn()
    const user = userEvent.setup()

    render(
      <SessionDock
        sessions={[
          { id: 1, name: 'Session A', visible: true },
          { id: 2, name: 'Session B', visible: true },
        ]}
        activeSessionId={1}
        onSwitchSession={onSwitchSession}
        onCreateSession={onCreateSession}
        onClearChats={onClearChats}
      />,
    )

    await user.click(screen.getByRole('button', { name: 'Clear Chat History' }))

    expect(screen.getByText('Clear Chat History')).toBeInTheDocument()
    expect(onClearChats).not.toHaveBeenCalled()

    await user.click(screen.getByRole('button', { name: 'Confirm' }))
    expect(onClearChats).toHaveBeenCalledTimes(1)
  })
})
