/**
 * Sanitization policy for markdown rendered from untrusted sources.
 *
 * Assistant answers can repeat text from crawled pages, MCP tools, or shared
 * prompts, so any raw HTML they carry is attacker-controlled. The Markdown
 * component parses that HTML with rehype-raw and must pass the resulting tree
 * through rehype-sanitize with this schema before any plugin that emits
 * trusted markup (rehype-katex) runs.
 */
import type { Options as MarkdownOptions } from 'react-markdown'
import { defaultSchema, type Options as SanitizeSchema } from 'rehype-sanitize'

/**
 * CLOBBER_PREFIX is the prefix rehype-sanitize adds to every `id` and `name`
 * so untrusted markup cannot shadow page globals through DOM clobbering.
 */
const CLOBBER_PREFIX = defaultSchema.clobberPrefix ?? 'user-content-'

/**
 * SAFE_LINK_PROTOCOLS lists the URL schemes a rendered link may use.
 */
const SAFE_LINK_PROTOCOLS = ['http', 'https', 'mailto', 'tel'] as const

/**
 * SAFE_URL_PROTOCOLS is the same allow list in URL-parser form.
 */
const SAFE_URL_PROTOCOLS: ReadonlySet<string> = new Set(
  SAFE_LINK_PROTOCOLS.map((protocol) => `${protocol}:`),
)

/**
 * DATA_IMAGE_PREFIX matches the base64 data URLs of raster image formats.
 * SVG is excluded on purpose: it is a document format that can carry script
 * and links, even though an <img> does not run them.
 */
const DATA_IMAGE_PREFIX = /^data:image\/(?:png|jpe?g|gif|webp|avif);base64,/i

/**
 * EXTRA_TAG_NAMES lists benign inline and figure elements that assistants
 * commonly emit and that the GitHub-style default schema would unwrap.
 * None of them can run script, load a document, or accept input.
 */
const EXTRA_TAG_NAMES = [
  'abbr',
  'bdi',
  'bdo',
  'caption',
  'cite',
  'dfn',
  'figcaption',
  'figure',
  'mark',
  'small',
  'time',
  'u',
  'wbr',
]

/**
 * DROPPED_TAG_NAMES lists default-schema elements the renderer removes.
 * `source` carries a `srcset` that neither the schema's protocol check nor
 * the urlTransform sees, so it could load any image, SVG data URLs included,
 * outside the `img` sink. A `<picture>` keeps rendering its `img` fallback.
 */
const DROPPED_TAG_NAMES: ReadonlySet<string> = new Set(['source'])

/**
 * markdownSanitizeSchema extends the GitHub-style default schema only with
 * what the chat renderer legitimately needs: remark-math classes that
 * rehype-katex consumes after sanitization, a few benign formatting tags,
 * `tel:` links and `data:` image sources (narrowed to base64 raster images by
 * sanitizeMarkdownUrl). Everything else, including scripts, frames, forms,
 * styles, event handlers, arbitrary classes, and `source` elements, is
 * removed, so `img` is the only element that loads an image.
 */
export const markdownSanitizeSchema: SanitizeSchema = {
  ...defaultSchema,
  tagNames: [
    ...(defaultSchema.tagNames ?? []).filter(
      (tagName) => !DROPPED_TAG_NAMES.has(tagName),
    ),
    ...EXTRA_TAG_NAMES,
  ],
  attributes: {
    ...Object.fromEntries(
      Object.entries(defaultSchema.attributes ?? {}).filter(
        ([tagName]) => !DROPPED_TAG_NAMES.has(tagName),
      ),
    ),
    // `language-*` drives code highlighting; remark-math marks math with
    // `math-inline` / `math-display` so rehype-katex can render it afterwards.
    code: [['className', /^language-./, 'math-inline', 'math-display']],
  },
  protocols: {
    ...defaultSchema.protocols,
    href: [...SAFE_LINK_PROTOCOLS],
    src: ['http', 'https', 'data'],
  },
  // Drop the text of elements whose content is never meant to be read.
  strip: ['script', 'style', 'template', 'noscript'],
}

/**
 * markdownRemarkRehypeOptions turns off remark-rehype's own footnote id prefix
 * because rehype-sanitize applies CLOBBER_PREFIX to every id afterwards;
 * keeping both would double-prefix footnote targets.
 */
export const markdownRemarkRehypeOptions: NonNullable<
  MarkdownOptions['remarkRehypeOptions']
> = {
  clobberPrefix: '',
}

/**
 * namespaceFragmentHref rewrites a same-document fragment link into the
 * prefixed id namespace the sanitizer moved every element id into, so
 * footnotes and in-answer anchors keep working. It returns other URLs as-is.
 */
export function namespaceFragmentHref(url: string): string {
  if (url.length <= 1 || !url.startsWith('#')) {
    return url
  }
  return `#${CLOBBER_PREFIX}${url.slice(1)}`
}

/**
 * sanitizeMarkdownUrl is the urlTransform for rendered markdown. It keeps
 * http(s), mailto, tel, relative and fragment URLs, allows base64 raster
 * data:image sources only outside `href`, namespaces fragment links, and
 * returns an empty string for every other URL.
 *
 * @param url - The raw URL from the markdown or HTML attribute.
 * @param key - The attribute name, such as `href` or `src`.
 * @returns The URL to render, or an empty string when it is unsafe.
 */
export function sanitizeMarkdownUrl(url: string, key?: string): string {
  const trimmed = String(url || '').trim()
  if (!trimmed) {
    return ''
  }

  if (DATA_IMAGE_PREFIX.test(trimmed) && key !== 'href') {
    return trimmed
  }

  if (trimmed.startsWith('#')) {
    return key === 'href' ? namespaceFragmentHref(trimmed) : trimmed
  }

  if (
    trimmed.startsWith('/') ||
    trimmed.startsWith('./') ||
    trimmed.startsWith('../')
  ) {
    return trimmed
  }

  try {
    const parsed = new URL(trimmed)
    if (SAFE_URL_PROTOCOLS.has(parsed.protocol)) {
      return trimmed
    }
  } catch {
    if (!trimmed.includes(':')) {
      return trimmed
    }
  }

  console.debug(
    `[Markdown] blocked unsafe url key=${key ?? ''} length=${trimmed.length}`,
  )
  return ''
}
