import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import { ChatMessage } from '../chat-message'
import type { ChatMessageData } from '../../types'
import '@testing-library/jest-dom'

describe('ChatMessage UI', () => {
  it('renders image attachments with performance attributes', () => {
    const message: ChatMessageData = {
      chatID: '1',
      role: 'user',
      content: 'hello',
      attachments: [
        {
          filename: 'test.png',
          type: 'image',
          contentB64:
            'data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==',
        },
      ],
    }

    render(<ChatMessage message={message} />)
    const img = screen.getByAltText('test.png')
    expect(img).toBeInTheDocument()
    expect(img).toHaveAttribute('loading', 'lazy')
    expect(img).toHaveAttribute('decoding', 'async')
  })

  it('shows the default loading animation for an empty streaming assistant message', () => {
    const message: ChatMessageData = {
      chatID: 'c1',
      role: 'assistant',
      content: '',
    }
    const { container } = render(<ChatMessage message={message} isStreaming />)
    const dots = container.querySelectorAll('.animate-bounce')
    expect(dots.length).toBe(3)
    expect(screen.getByText('Generating…')).toBeInTheDocument()
  })

  it('shows the loadingLabel alongside the animation for image generation', () => {
    const message: ChatMessageData = {
      chatID: 'c2',
      role: 'assistant',
      content: '',
      loadingLabel: 'Generating image…',
    }
    const { container } = render(<ChatMessage message={message} isStreaming />)
    const dots = container.querySelectorAll('.animate-bounce')
    expect(dots.length).toBe(3)
    const label = screen.getByText('Generating image…')
    expect(label).toBeInTheDocument()
    // The label must be visible at all times (not hidden behind motion-reduce).
    expect(label.className).not.toContain('hidden')
  })

  it('shows the loading animation during a retry even when loadingLabel is unset', () => {
    // Regenerate clears the assistant message to content='' with no loadingLabel.
    // The animation must still show while isStreaming is true.
    const message: ChatMessageData = {
      chatID: 'c3',
      role: 'assistant',
      content: '',
    }
    const { container } = render(<ChatMessage message={message} isStreaming />)
    expect(container.querySelectorAll('.animate-bounce').length).toBe(3)
  })

  it('stops showing the loading animation once content has been produced', () => {
    const message: ChatMessageData = {
      chatID: 'c4',
      role: 'assistant',
      content: '![Image](https://example.com/x.png)',
    }
    const { container } = render(<ChatMessage message={message} isStreaming />)
    expect(container.querySelectorAll('.animate-bounce').length).toBe(0)
  })

  it('collapses long user messages and toggles the full content', async () => {
    const user = userEvent.setup()
    const message: ChatMessageData = {
      chatID: 'long-user-message',
      role: 'user',
      content: Array.from(
        { length: 12 },
        (_, index) => `Line ${index + 1}`,
      ).join('\n'),
    }

    render(<ChatMessage message={message} />)

    const expandButton = screen.getByRole('button', {
      name: 'Expand full message',
    })
    const content = document.getElementById(
      expandButton.getAttribute('aria-controls') ?? '',
    )

    expect(content).toBeInTheDocument()
    expect(expandButton).toHaveAttribute('aria-expanded', 'false')
    expect(content).toHaveClass('max-h-64')

    await user.click(expandButton)

    const collapseButton = screen.getByRole('button', {
      name: 'Collapse message',
    })
    expect(collapseButton).toHaveAttribute('aria-expanded', 'true')
    expect(content).not.toHaveClass('max-h-64')

    await user.click(collapseButton)
    expect(
      screen.getByRole('button', { name: 'Expand full message' }),
    ).toHaveAttribute('aria-expanded', 'false')
  })

  it('keeps a long streaming response expanded after generation completes', () => {
    const message: ChatMessageData = {
      chatID: 'streaming-long-message',
      role: 'assistant',
      content: Array.from(
        { length: 12 },
        (_, index) => `Line ${index + 1}`,
      ).join('\n'),
    }

    const { container, rerender } = render(
      <ChatMessage message={message} isStreaming />,
    )

    expect(
      screen.queryByRole('button', { name: 'Expand full message' }),
    ).not.toBeInTheDocument()
    expect(container.querySelector('.max-h-64')).not.toBeInTheDocument()

    rerender(<ChatMessage message={message} />)

    expect(
      screen.queryByRole('button', { name: 'Expand full message' }),
    ).not.toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Collapse message' }),
    ).toBeInTheDocument()
    expect(container.querySelector('.max-h-64')).not.toBeInTheDocument()
  })

  it('collapses a completed long assistant response by default', () => {
    const message: ChatMessageData = {
      chatID: 'completed-long-message',
      role: 'assistant',
      content: Array.from(
        { length: 12 },
        (_, index) => `Line ${index + 1}`,
      ).join('\n'),
    }

    const { container } = render(<ChatMessage message={message} />)

    expect(
      screen.getByRole('button', { name: 'Expand full message' }),
    ).toBeInTheDocument()
    expect(container.querySelector('.max-h-64')).toBeInTheDocument()
  })

  it('measures only assistant body content, not reasoning and tools', () => {
    const message: ChatMessageData = {
      chatID: 'reasoning-only-long-message',
      role: 'assistant',
      content: 'A short final answer.',
      reasoningContent: Array.from(
        { length: 40 },
        (_, index) => `Reasoning step ${index + 1}`,
      ).join('\n'),
    }

    render(<ChatMessage message={message} />)

    expect(
      screen.queryByRole('button', { name: 'Expand full message' }),
    ).not.toBeInTheDocument()
  })
})
