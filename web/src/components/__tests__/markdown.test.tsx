import { render } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { Markdown } from '../markdown'

describe('Markdown code blocks', () => {
  const isBlankLine = (line: string) => {
    const withoutAnsi = line.replace(/\x1B\[[0-9;]*[A-Za-z]/g, '')
    return withoutAnsi.replace(/[\p{White_Space}\p{Cf}\p{Cc}]/gu, '') === ''
  }

  it('keeps only one trailing blank line in code blocks', () => {
    const content = ['```go', 'package main', '', '', '', '```'].join('\n')

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
      if (isBlankLine(lines[i])) {
        trailingEmpty += 1
      } else {
        break
      }
    }
    expect(trailingEmpty).toBeLessThanOrEqual(1)
  })

  it('trims trailing blank lines with zero-width whitespace', () => {
    const content = ['```go', 'package main', '\u200B', '\u200B', '```'].join(
      '\n',
    )

    const { container } = render(<Markdown>{content}</Markdown>)

    const code = container.querySelector('.code-shell__content pre code')
    const text = code?.textContent || ''
    const lines = text.replace(/\r\n/g, '\n').split('\n')
    let trailingEmpty = 0
    for (let i = lines.length - 1; i >= 0; i -= 1) {
      if (isBlankLine(lines[i])) {
        trailingEmpty += 1
      } else {
        break
      }
    }
    expect(trailingEmpty).toBeLessThanOrEqual(1)
  })

  it('renders data image URLs in markdown images', () => {
    const dataUrl = 'data:image/png;base64,AAA'
    const content = `![Image](${dataUrl})`

    const { container } = render(<Markdown>{content}</Markdown>)

    const img = container.querySelector('img') as HTMLImageElement | null
    expect(img).not.toBeNull()
    expect(img?.getAttribute('src')).toBe(dataUrl)
  })
})
