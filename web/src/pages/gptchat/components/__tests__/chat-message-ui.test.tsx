import { render, screen } from '@testing-library/react'
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
})
