import type { ReactNode } from 'react'
import ReactMarkdown, { type Components } from 'react-markdown'
import remarkGfm from 'remark-gfm'

import { slugify } from './cv-helpers'

/**
 * extractPlainText collects plain text from a ReactNode tree.
 */
function extractPlainText(node: ReactNode): string {
  if (typeof node === 'string' || typeof node === 'number') {
    return String(node)
  }

  if (Array.isArray(node)) {
    return node.map((child) => extractPlainText(child)).join(' ')
  }

  if (node && typeof node === 'object' && 'props' in node) {
    const props = node.props as { children?: ReactNode }
    return extractPlainText(props.children)
  }

  return ''
}

/**
 * CvMarkdown renders markdown with heading anchors for the CV preview.
 */
export function CvMarkdown({ content }: { content: string }) {
  const renderHeading =
    (level: 1 | 2 | 3 | 4 | 5 | 6) =>
    ({ children }: { children?: ReactNode }) => {
      const text = extractPlainText(children ?? '')
      const id = text ? slugify(text) : undefined
      const Tag = `h${level}` as const
      const className =
        level === 1
          ? 'text-4xl font-semibold tracking-tight'
          : level === 2
            ? 'scroll-mt-28 text-2xl font-semibold tracking-tight'
            : 'scroll-mt-24 text-xl font-semibold'

      return (
        <Tag id={id} className={className}>
          {children}
        </Tag>
      )
    }

  const components: Components = {
    h1: renderHeading(1),
    h2: renderHeading(2),
    h3: renderHeading(3),
    h4: renderHeading(4),
    h5: renderHeading(5),
    h6: renderHeading(6),
    a: ({ href, children }) => (
      <a href={href} className="cv-link">
        {children}
      </a>
    ),
  }

  return (
    <ReactMarkdown remarkPlugins={[remarkGfm]} components={components}>
      {content}
    </ReactMarkdown>
  )
}
