import { describe, expect, it } from 'vitest'
import { rangeToMarkdown } from '../selection-markdown'

describe('rangeToMarkdown', () => {
  it('serializes strong text', () => {
    const container = document.createElement('div')
    container.innerHTML = '<strong>Bold</strong>'
    const range = document.createRange()
    range.selectNodeContents(container)

    expect(rangeToMarkdown(range)).toBe('**Bold**')
  })

  it('serializes links', () => {
    const container = document.createElement('div')
    container.innerHTML = '<a href="https://example.com">Link</a>'
    const range = document.createRange()
    range.selectNodeContents(container)

    expect(rangeToMarkdown(range)).toBe('[Link](https://example.com)')
  })

  it('serializes lists', () => {
    const container = document.createElement('div')
    container.innerHTML = '<ul><li>First</li><li>Second</li></ul>'
    const range = document.createRange()
    range.selectNodeContents(container)

    expect(rangeToMarkdown(range)).toBe('- First\n- Second')
  })

  it('serializes inline code', () => {
    const container = document.createElement('div')
    container.innerHTML = '<code>const x = 1</code>'
    const range = document.createRange()
    range.selectNodeContents(container)

    expect(rangeToMarkdown(range)).toBe('`const x = 1`')
  })

  it('serializes code blocks with language', () => {
    const container = document.createElement('div')
    container.innerHTML =
      '<pre><code class="language-js">const x = 1;\n</code></pre>'
    const range = document.createRange()
    range.selectNodeContents(container)

    expect(rangeToMarkdown(range)).toBe('```js\nconst x = 1;\n```')
  })

  it('serializes blockquotes', () => {
    const container = document.createElement('div')
    container.innerHTML = '<blockquote><p>Quote</p></blockquote>'
    const range = document.createRange()
    range.selectNodeContents(container)

    expect(rangeToMarkdown(range)).toBe('> Quote')
  })
})
