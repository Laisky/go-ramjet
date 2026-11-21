import { render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ChatSearch } from '../chat-search'
import '@testing-library/jest-dom'

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

vi.mock('@radix-ui/react-dialog', () => ({
  Root: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  Trigger: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  Portal: () => null,
  Overlay: () => null,
  Content: () => null,
  Title: () => null,
  Description: () => null,
  Close: () => null,
}))

/**
 * renderOnPlatform renders the search trigger as if the browser reported platform.
 */
function renderOnPlatform(platform: string) {
  vi.spyOn(navigator, 'platform', 'get').mockReturnValue(platform)
  render(
    <ChatSearch
      messages={[]}
      sessions={[{ id: 1, name: 'Default', visible: true }]}
      currentSessionId={1}
      onSelectMessage={vi.fn()}
    />,
  )
}

describe('ChatSearch shortcut hint', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('advertises Cmd on Apple platforms, where Cmd+K opens search', () => {
    renderOnPlatform('MacIntel')
    expect(screen.getByTestId('tooltip-content')).toHaveTextContent(
      'Search messages (Cmd+K / Cmd+F)',
    )
  })

  it('keeps advertising Ctrl on other platforms', () => {
    renderOnPlatform('Win32')
    expect(screen.getByTestId('tooltip-content')).toHaveTextContent(
      'Search messages (Ctrl+K / Ctrl+F)',
    )
  })

  it('exposes every accepted shortcut to assistive technology', () => {
    renderOnPlatform('Linux x86_64')
    expect(
      screen.getByRole('button', { name: 'Search messages' }),
    ).toHaveAttribute('aria-keyshortcuts', 'Control+K Meta+K Control+F Meta+F')
  })
})
