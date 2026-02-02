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
})
