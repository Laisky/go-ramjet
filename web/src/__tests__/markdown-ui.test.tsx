import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { Markdown } from '@/components/markdown'
import '@testing-library/jest-dom'

describe('Markdown UI', () => {
  it('renders images with decoding="async"', () => {
    const content = '![alt text](https://example.com/image.png)'
    render(<Markdown>{content}</Markdown>)
    const img = screen.getByAltText('alt text')
    expect(img).toBeInTheDocument()
    expect(img).toHaveAttribute('loading', 'lazy')
    expect(img).toHaveAttribute('decoding', 'async')
  })
})
