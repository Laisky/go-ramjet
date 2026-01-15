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
})
