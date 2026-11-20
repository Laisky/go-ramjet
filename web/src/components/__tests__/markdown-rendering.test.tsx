import { render } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { Markdown } from '../markdown'

/**
 * renderMarkdown renders source through the production Markdown component and
 * returns its container for structural assertions.
 */
function renderMarkdown(source: string): HTMLElement {
  return render(<Markdown>{source}</Markdown>).container
}

describe('Markdown trusted rendering', () => {
  it('keeps GFM tables with column alignment and inline breaks', () => {
    const container = renderMarkdown(
      ['| a | b | c |', '|:--|--:|:-:|', '| 1<br>x | 2 | 3 |'].join('\n'),
    )
    const headers = container.querySelectorAll('table thead th')
    expect(headers).toHaveLength(3)
    expect((headers[0] as HTMLElement).style.textAlign).toBe('left')
    expect((headers[1] as HTMLElement).style.textAlign).toBe('right')
    expect((headers[2] as HTMLElement).style.textAlign).toBe('center')
    expect(container.querySelector('table tbody td br')).not.toBeNull()
  })

  it('keeps task lists as disabled checkboxes', () => {
    const container = renderMarkdown('- [x] done\n- [ ] todo')
    expect(container.querySelector('ul.contains-task-list')).not.toBeNull()
    expect(container.querySelectorAll('li.task-list-item')).toHaveLength(2)
    const boxes = container.querySelectorAll('input[type="checkbox"]')
    expect(boxes).toHaveLength(2)
    expect((boxes[0] as HTMLInputElement).checked).toBe(true)
    expect((boxes[0] as HTMLInputElement).disabled).toBe(true)
    expect((boxes[1] as HTMLInputElement).checked).toBe(false)
  })

  it('keeps footnote references pointing at rendered footnote ids', () => {
    const container = renderMarkdown('Claim[^1].\n\n[^1]: Source note.')
    const ref = container.querySelector('a[data-footnote-ref]')
    expect(ref).not.toBeNull()
    const href = ref?.getAttribute('href') ?? ''
    expect(href).toBe('#user-content-fn-1')
    expect(container.querySelector(href)?.textContent).toContain('Source note.')

    const backref = container.querySelector('a[data-footnote-backref]')
    const backHref = backref?.getAttribute('href') ?? ''
    expect(backHref).toBe('#user-content-fnref-1')
    expect(container.querySelector(backHref)).toBe(ref)

    const section = container.querySelector('section.footnotes')
    expect(section).not.toBeNull()
    const labelId = ref?.getAttribute('aria-describedby') ?? ''
    expect(container.querySelector(`#${labelId}`)?.textContent).toBe(
      'Footnotes',
    )
    expect(container.querySelector('h2.sr-only')).not.toBeNull()
  })

  it('keeps raw anchors that link to raw headings in the same answer', () => {
    const container = renderMarkdown(
      '<a href="#usage">Jump</a>\n\n<h2 id="usage">Usage</h2>',
    )
    const href = container.querySelector('a')?.getAttribute('href') ?? ''
    expect(href.startsWith('#')).toBe(true)
    expect(container.querySelector(href)?.textContent).toBe('Usage')
  })

  it('keeps strikethrough, ordered list starts, blockquotes and autolinks', () => {
    const container = renderMarkdown(
      '~~gone~~ https://example.com\n\n3. three\n4. four\n\n> quoted',
    )
    expect(container.querySelector('del')?.textContent).toBe('gone')
    expect(container.querySelector('ol')?.getAttribute('start')).toBe('3')
    expect(container.querySelector('blockquote')?.textContent).toContain(
      'quoted',
    )
    expect(
      container.querySelector('a[href="https://example.com"]'),
    ).not.toBeNull()
  })

  it('keeps raw details and summary with nested markdown', () => {
    const container = renderMarkdown(
      '<details open><summary>More</summary>\n\nHidden **bold**\n\n</details>',
    )
    const details = container.querySelector('details')
    expect(details).not.toBeNull()
    expect(details?.hasAttribute('open')).toBe(true)
    expect(details?.querySelector('summary')?.textContent).toBe('More')
    expect(details?.querySelector('strong')?.textContent).toBe('bold')
  })

  it('keeps benign inline formatting tags', () => {
    const container = renderMarkdown(
      '<kbd>Ctrl</kbd> <sup>2</sup> <sub>3</sub> <mark>m</mark> <abbr title="tooltip">ab</abbr> <u>u</u> <ins>i</ins> <small>s</small> <s>x</s> <q>q</q> <samp>o</samp> <var>v</var>',
    )
    for (const tag of [
      'kbd',
      'sup',
      'sub',
      'mark',
      'abbr',
      'u',
      'ins',
      'small',
      's',
      'q',
      'samp',
      'var',
    ]) {
      expect(container.querySelector(tag), tag).not.toBeNull()
    }
    expect(container.querySelector('abbr')?.getAttribute('title')).toBe(
      'tooltip',
    )
  })

  it('keeps centered raw paragraphs with images', () => {
    const container = renderMarkdown(
      '<p align="center"><img src="https://example.com/a.png" alt="pic"></p>',
    )
    expect(container.querySelector('p')?.getAttribute('align')).toBe('center')
    const img = container.querySelector('img')
    expect(img?.getAttribute('src')).toBe('https://example.com/a.png')
    expect(img?.getAttribute('alt')).toBe('pic')
  })

  it('keeps safe link protocols and relative links', () => {
    const container = renderMarkdown(
      '<a href="https://example.com" title="tt">raw</a> [md](https://example.com "tip") [mail](mailto:a@example.com) [tel](tel:+123) [rel](/gptchat) [http](http://example.com)',
    )
    const hrefs = Array.from(container.querySelectorAll('a'), (a) =>
      a.getAttribute('href'),
    )
    expect(hrefs).toEqual([
      'https://example.com',
      'https://example.com',
      'mailto:a@example.com',
      'tel:+123',
      '/gptchat',
      'http://example.com',
    ])
    expect(container.querySelector('a[title="tt"]')).not.toBeNull()
    expect(container.querySelector('a[title="tip"]')).not.toBeNull()
  })

  it('keeps markdown and raw images with https and data:image sources', () => {
    const dataUrl = 'data:image/png;base64,AAA'
    const container = renderMarkdown(
      `![a](https://example.com/a.png) ![b](${dataUrl}) <img src="${dataUrl}" alt="c">`,
    )
    const sources = Array.from(container.querySelectorAll('img'), (img) =>
      img.getAttribute('src'),
    )
    expect(sources).toEqual(['https://example.com/a.png', dataUrl, dataUrl])
  })

  it('keeps highlighted fenced code blocks with their language label', () => {
    const container = renderMarkdown('```go\nfunc main() {}\n```')
    expect(
      container.querySelector('.code-shell__toolbar')?.textContent,
    ).toContain('GO')
    expect(
      container.querySelector('.code-shell__content .hljs-keyword'),
    ).not.toBeNull()
  })

  it('keeps raw code elements with a language as highlighted blocks', () => {
    const container = renderMarkdown(
      '<pre><code class="language-python">print(1)\n</code></pre>',
    )
    expect(
      container.querySelector('.code-shell__toolbar')?.textContent,
    ).toContain('PYTHON')
  })

  it('keeps inline code', () => {
    const container = renderMarkdown('Use `npm ci` here.')
    expect(container.querySelector('p code')?.textContent).toBe('npm ci')
  })

  it('routes mermaid fences to the diagram renderer', () => {
    const container = renderMarkdown('```mermaid\ngraph TD; A-->B\n```')
    expect(
      container.querySelector('.code-shell__toolbar')?.textContent,
    ).toContain('Mermaid Diagram')
  })

  it('keeps inline and display math rendered by KaTeX', () => {
    const container = renderMarkdown('Inline $x^2$ math.\n\n$$\ny = 1\n$$')
    expect(container.querySelectorAll('.katex')).toHaveLength(2)
    expect(container.querySelectorAll('.katex-display')).toHaveLength(1)
    expect(container.querySelector('.katex-html [style]')).not.toBeNull()
  })
})
