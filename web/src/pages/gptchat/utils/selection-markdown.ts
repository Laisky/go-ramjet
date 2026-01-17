/**
 * rangeToMarkdown converts a DOM Range to markdown while preserving formatting.
 *
 * @param range - DOM selection range to convert.
 * @returns Markdown representation of the selection.
 */
export function rangeToMarkdown(range: Range): string {
  const fragment = range.cloneContents()
  const markdown = serializeNodes(Array.from(fragment.childNodes))
  return normalizeMarkdown(markdown)
}

/**
 * serializeNodes converts a list of DOM nodes into markdown.
 *
 * @param nodes - Nodes to serialize.
 * @returns Markdown string for all nodes.
 */
function serializeNodes(nodes: Node[]): string {
  return nodes.map((node) => serializeNode(node)).join('')
}

/**
 * serializeNode converts a single DOM node into markdown.
 *
 * @param node - Node to serialize.
 * @returns Markdown string for the node.
 */
function serializeNode(node: Node): string {
  if (node.nodeType === Node.TEXT_NODE) {
    return node.textContent || ''
  }

  if (!(node instanceof HTMLElement)) {
    return ''
  }

  const tag = node.tagName.toLowerCase()
  const children = serializeNodes(Array.from(node.childNodes))

  switch (tag) {
    case 'strong':
    case 'b':
      return wrapInline(children, '**')
    case 'em':
    case 'i':
      return wrapInline(children, '*')
    case 'del':
      return wrapInline(children, '~~')
    case 'code':
      if (node.parentElement?.tagName.toLowerCase() === 'pre') {
        return children
      }
      return wrapInlineCode(children)
    case 'pre':
      return serializeCodeBlock(node)
    case 'a':
      return serializeLink(node, children)
    case 'br':
      return '  \n'
    case 'p':
      return `${children}\n\n`
    case 'ul':
      return serializeList(node, false)
    case 'ol':
      return serializeList(node, true)
    case 'li':
      return `${children}\n`
    case 'blockquote':
      return `${prefixLines(children.trim(), '> ')}\n\n`
    case 'h1':
    case 'h2':
    case 'h3':
    case 'h4':
    case 'h5':
    case 'h6':
      return `${'#'.repeat(parseInt(tag.replace('h', ''), 10))} ${children}\n\n`
    case 'img':
      return serializeImage(node)
    default:
      return children
  }
}

/**
 * serializeLink converts an anchor element into markdown.
 *
 * @param node - Anchor element.
 * @param text - Link text.
 * @returns Markdown link string.
 */
function serializeLink(node: HTMLElement, text: string): string {
  const href = node.getAttribute('href') || ''
  const safeText = text || href
  return href ? `[${safeText}](${href})` : safeText
}

/**
 * serializeImage converts an image element into markdown.
 *
 * @param node - Image element.
 * @returns Markdown image string.
 */
function serializeImage(node: HTMLElement): string {
  const alt = node.getAttribute('alt') || ''
  const src = node.getAttribute('src') || ''
  if (!src) {
    return alt
  }
  return `![${alt}](${src})`
}

/**
 * serializeList converts a list element into markdown.
 *
 * @param node - List element.
 * @param ordered - Whether the list is ordered.
 * @returns Markdown list string.
 */
function serializeList(node: HTMLElement, ordered: boolean): string {
  const items = Array.from(node.children).filter(
    (child) => child.tagName.toLowerCase() === 'li',
  )
  const lines = items.map((item, index) => {
    const content = serializeNodes(Array.from(item.childNodes)).trimEnd()
    const prefix = ordered ? `${index + 1}. ` : '- '
    return `${prefix}${content}`
  })
  return `${lines.join('\n')}\n\n`
}

/**
 * serializeCodeBlock converts a preformatted block into markdown.
 *
 * @param node - Pre element.
 * @returns Markdown code block string.
 */
function serializeCodeBlock(node: HTMLElement): string {
  const code = node.querySelector('code')
  const language = extractLanguage(code?.className)
  const content = (node.textContent || '').replace(/\n$/, '')
  const fence = buildFence(content, '`')
  const langLabel = language ? language : ''
  return `${fence}${langLabel}\n${content}\n${fence}\n\n`
}

/**
 * extractLanguage reads the language from a className string.
 *
 * @param className - Code element className.
 * @returns Language identifier or empty string.
 */
function extractLanguage(className?: string | null): string {
  if (!className) {
    return ''
  }
  const match = className.match(/language-([\w-]+)/)
  return match ? match[1] : ''
}

/**
 * buildFence creates a code fence that does not collide with content.
 *
 * @param text - Code block content.
 * @param marker - Fence marker character.
 * @returns Fence string.
 */
function buildFence(text: string, marker: string): string {
  const matches = text.match(new RegExp(`${marker}+`, 'g')) || []
  const max = matches.reduce((acc, value) => Math.max(acc, value.length), 2)
  return marker.repeat(max + 1)
}

/**
 * wrapInline wraps text with a markdown marker.
 *
 * @param text - Content to wrap.
 * @param marker - Markdown marker.
 * @returns Wrapped markdown string.
 */
function wrapInline(text: string, marker: string): string {
  if (!text) {
    return ''
  }
  return `${marker}${text}${marker}`
}

/**
 * wrapInlineCode wraps inline code with a safe backtick fence.
 *
 * @param text - Inline code content.
 * @returns Wrapped markdown string.
 */
function wrapInlineCode(text: string): string {
  if (!text) {
    return ''
  }
  const matches = text.match(/`+/g) || []
  const max = matches.reduce((acc, value) => Math.max(acc, value.length), 0)
  const fence = '`'.repeat(Math.max(1, max + 1))
  return `${fence}${text}${fence}`
}

/**
 * prefixLines applies a prefix to each line of text.
 *
 * @param text - Input text.
 * @param prefix - Prefix to apply.
 * @returns Prefixed text.
 */
function prefixLines(text: string, prefix: string): string {
  if (!text) {
    return ''
  }
  return text
    .split('\n')
    .map((line) => `${prefix}${line}`)
    .join('\n')
}

/**
 * normalizeMarkdown cleans up whitespace and blank lines in markdown.
 *
 * @param markdown - Raw markdown.
 * @returns Normalized markdown.
 */
function normalizeMarkdown(markdown: string): string {
  return markdown
    .replace(/[ \t]+\n/g, '\n')
    .replace(/\n{3,}/g, '\n\n')
    .trim()
}
