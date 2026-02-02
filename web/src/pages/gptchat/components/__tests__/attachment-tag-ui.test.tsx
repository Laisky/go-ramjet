import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { AttachmentTag } from '../attachment-tag'
import '@testing-library/jest-dom'

describe('AttachmentTag UI', () => {
  it('renders image with performance attributes', () => {
    render(
      <AttachmentTag
        filename="test.png"
        type="image"
        contentB64="data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="
      />
    )
    const img = screen.getByAltText('test.png')
    expect(img).toBeInTheDocument()
    expect(img).toHaveAttribute('loading', 'lazy')
    expect(img).toHaveAttribute('decoding', 'async')
  })
})
