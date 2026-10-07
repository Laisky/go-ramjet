import { render } from '@testing-library/react'
import DOMPurify from 'dompurify'
import katex from 'katex'
import { describe, expect, it } from 'vitest'

import { Markdown } from './markdown'

/** collectClasses returns the class attributes emitted by a math renderer. */
function collectClasses(root: Element): Array<string | null> {
  return Array.from(root.querySelectorAll('[class]'), (node) =>
    node.getAttribute('class'),
  )
}

const mathCases = [
  ['inline fraction', '\\frac{a}{b}', false],
  ['display fraction', '\\frac{a}{b}', true],
  ['square root', '\\sqrt{x^2 + y^2}', false],
  [
    'aligned equations',
    '\\begin{aligned}a &= b + c\\\\d &= e\\end{aligned}',
    true,
  ],
] as const

describe('Markdown math dependency compatibility', () => {
  it.each(mathCases)(
    'uses the stylesheet package renderer classes for %s',
    (_name, expression, displayMode) => {
      const source = displayMode ? `$$\n${expression}\n$$` : `$${expression}$`
      const { container } = render(<Markdown>{source}</Markdown>)
      const actual = container.querySelector('.katex')
      const reference = document.createElement('div')
      reference.innerHTML = katex.renderToString(expression, { displayMode })
      const expected = reference.querySelector('.katex')

      expect(actual).not.toBeNull()
      expect(expected).not.toBeNull()
      if (!actual || !expected) {
        throw new Error(
          'Both Markdown and the stylesheet package must render math',
        )
      }
      // The CSS import in Markdown resolves to this direct KaTeX package.
      // Comparing real output detects a nested rehype-katex renderer whose old
      // class names are no longer supported by the imported stylesheet.
      expect(collectClasses(actual)).toEqual(collectClasses(expected))
      expect(actual.querySelector('math')).not.toBeNull()
      expect(actual.querySelector('annotation')?.textContent).toBe(expression)
      expect(container.querySelector('.katex-display') !== null).toBe(
        displayMode,
      )
    },
  )

  it('keeps missing font metrics non-fatal with the default strict policy', () => {
    const { container } = render(<Markdown>{'$\\text{🙂}$'}</Markdown>)
    expect(container.querySelector('.katex')).not.toBeNull()
    expect(container.querySelector('.katex-error')).toBeNull()
    expect(container.textContent).toContain('🙂')
  })

  it('keeps untrusted math commands from creating executable links', () => {
    const { container } = render(
      <Markdown>{'$\\href{javascript:alert(1)}{unsafe}$'}</Markdown>,
    )
    expect(container.querySelector('a[href^="javascript:"]')).toBeNull()
    expect(container.querySelector('script')).toBeNull()
    expect(container.textContent).toContain('unsafe')
  })

  it('retains the readable fallback for malformed math', () => {
    const { container } = render(<Markdown>{'$\\frac{$'}</Markdown>)
    expect(container.querySelector('.katex-error')).not.toBeNull()
    expect(container.textContent).toContain('\\frac{')
  })
})

describe('DOMPurify dependency compatibility', () => {
  it('removes active HTML while preserving ordinary content', () => {
    const clean = DOMPurify.sanitize(
      '<p>safe</p><img src="x" onerror="alert(1)"><script>alert(1)</script>',
    )
    const root = document.createElement('div')
    root.innerHTML = clean
    expect(root.querySelector('p')?.textContent).toBe('safe')
    expect(root.querySelector('script')).toBeNull()
    expect(root.querySelector('img')?.getAttribute('onerror')).toBeNull()
  })

  it('preserves diagram SVG geometry but removes executable attributes', () => {
    const clean = DOMPurify.sanitize(
      '<svg viewBox="0 0 100 100"><g><path d="M0 0 L10 10" onclick="alert(1)" /></g><script>alert(1)</script></svg>',
    )
    const root = document.createElement('div')
    root.innerHTML = clean
    expect(root.querySelector('svg')?.getAttribute('viewBox')).toBe(
      '0 0 100 100',
    )
    expect(root.querySelector('path')?.getAttribute('d')).toBe('M0 0 L10 10')
    expect(root.querySelector('[onclick], script')).toBeNull()
  })

  it('preserves KaTeX classes and accessible MathML without allowing annotations', () => {
    const html = katex.renderToString('\\frac{a}{b}')
    const original = document.createElement('div')
    original.innerHTML = html
    const clean = document.createElement('div')
    clean.innerHTML = DOMPurify.sanitize(html)
    expect(collectClasses(clean)).toEqual(collectClasses(original))
    expect(clean.querySelector('math mfrac')).not.toBeNull()
    expect(
      Array.from(clean.querySelectorAll('math mi'), (node) => node.textContent),
    ).toEqual(['a', 'b'])
    // DOMPurify deliberately excludes annotation elements by default. Keep
    // that policy rather than weakening sanitization just to preserve TeX text.
    expect(clean.querySelector('annotation, annotation-xml')).toBeNull()
  })
})
