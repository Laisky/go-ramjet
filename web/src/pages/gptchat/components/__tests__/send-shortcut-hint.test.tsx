import { render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import '@testing-library/jest-dom'

import { DefaultSessionConfig } from '../../types'
import { ChatInput } from '../chat-input'
import { EditMessageModal } from '../edit-message-modal'

// Render tooltip content inline so the advertised shortcut can be asserted.
vi.mock('@/components/ui/tooltip-wrapper', () => ({
  TooltipWrapper: ({
    children,
    content,
  }: {
    children: React.ReactNode
    content: React.ReactNode
  }) => (
    <>
      {children}
      <span data-testid="tooltip-content">{content}</span>
    </>
  ),
}))

vi.mock('../../hooks/use-user', () => ({
  useUser: () => ({ user: { is_free: false } }),
}))

vi.mock('../message-input', () => ({
  MessageInput: () => <textarea aria-label="Message" />,
}))

/**
 * mockPlatform makes the browser report platform for the current test.
 */
function mockPlatform(platform: string) {
  vi.spyOn(navigator, 'platform', 'get').mockReturnValue(platform)
}

describe('send shortcut hints', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it.each([
    ['MacIntel', 'Cmd'],
    ['Win32', 'Ctrl'],
  ])('ChatInput on %s advertises %s+Enter to send', (platform, modifier) => {
    mockPlatform(platform)
    render(
      <ChatInput
        config={DefaultSessionConfig}
        onSend={vi.fn()}
        sessionId={1}
      />,
    )
    expect(
      screen.getAllByTestId('tooltip-content').map((node) => node.textContent),
    ).toContain(`Send message (${modifier}+Enter)`)
  })

  it.each([
    ['MacIntel', 'Cmd'],
    ['Linux x86_64', 'Ctrl'],
  ])(
    'EditMessageModal on %s advertises %s+Enter to submit',
    (platform, modifier) => {
      mockPlatform(platform)
      render(
        <EditMessageModal
          content="draft"
          onClose={vi.fn()}
          onConfirm={vi.fn()}
          apiToken=""
        />,
      )
      expect(
        screen.getByText(`${modifier}+Enter to submit • Esc to cancel`),
      ).toBeInTheDocument()
    },
  )
})
