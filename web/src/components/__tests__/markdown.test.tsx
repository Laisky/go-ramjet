import { render } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { Markdown } from '../markdown'

describe('Markdown code blocks', () => {
  it('keeps only one trailing blank line in code blocks', () => {
    const content = [
      '```go',
      'package main',
      '',
      '',
      '',
      '```',
    ].join('\n')

    const { container } = render(<Markdown>{content}</Markdown>)

    const lines = container.querySelectorAll('.code-shell__gutter span')
    expect(lines.length).toBe(2)
  })

  it('does not add more than one trailing blank line', () => {
    const content = ['```go', 'package main', '```'].join('\n')

    const { container } = render(<Markdown>{content}</Markdown>)

    const code = container.querySelector('.code-shell__content pre code')
    const text = code?.textContent || ''
    const lines = text.replace(/\r\n/g, '\n').split('\n')
    let trailingEmpty = 0
    for (let i = lines.length - 1; i >= 0; i -= 1) {
      if (lines[i].trim() === '') {
        trailingEmpty += 1
      } else {
        break
      }
    }
    expect(trailingEmpty).toBeLessThanOrEqual(1)
  })
})
