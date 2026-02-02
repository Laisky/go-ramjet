import hljs from 'highlight.js/lib/common'
import { Check, Copy, Eye, EyeOff } from 'lucide-react'
import type { HTMLAttributes, ReactNode } from 'react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { Components, ExtraProps } from 'react-markdown'
import ReactMarkdown from 'react-markdown'
import rehypeKatex from 'rehype-katex'
import rehypeRaw from 'rehype-raw'
import remarkGfm from 'remark-gfm'
import remarkMath from 'remark-math'

import { cn } from '@/utils/cn'
import 'katex/dist/katex.min.css'

type ColorScheme = 'dark' | 'light'

type CodeRendererProps = HTMLAttributes<HTMLElement> &
  ExtraProps & {
    inline?: boolean
    children?: ReactNode
  }

const SAFE_PROTOCOLS = new Set(['http:', 'https:', 'mailto:', 'tel:'])
const DATA_IMAGE_PREFIX = /^data:image\/[a-z0-9.+-]+;base64,/i

/**
 * sanitizeMarkdownUrl ensures only safe URLs are rendered by ReactMarkdown.
 * It allows data:image base64 sources for images while blocking other data URLs.
 */
function sanitizeMarkdownUrl(url: string, key?: string): string {
  const trimmed = String(url || '').trim()
  if (!trimmed) {
    return ''
  }

  if (DATA_IMAGE_PREFIX.test(trimmed) && key !== 'href') {
    return trimmed
  }

  if (
    trimmed.startsWith('#') ||
    trimmed.startsWith('/') ||
    trimmed.startsWith('./') ||
    trimmed.startsWith('../')
  ) {
    return trimmed
  }

  try {
    const parsed = new URL(trimmed)
    if (SAFE_PROTOCOLS.has(parsed.protocol)) {
      return trimmed
    }
  } catch (err) {
    if (!trimmed.includes(':')) {
      return trimmed
    }
  }

  console.debug('[Markdown] blocked unsafe url', {
    key,
    length: trimmed.length,
    hasProtocol: trimmed.includes(':'),
  })
  return ''
}

/**
 * useColorScheme watches the root element and media query to determine the active theme.
 */
function useColorScheme(): ColorScheme {
  const [scheme, setScheme] = useState<ColorScheme>(() => {
    if (typeof window === 'undefined') {
      return 'light'
    }
    if (document.documentElement.classList.contains('dark')) {
      return 'dark'
    }
    return window.matchMedia('(prefers-color-scheme: dark)').matches
      ? 'dark'
      : 'light'
  })

  useEffect(() => {
    if (typeof window === 'undefined') {
      return
    }

    const mediaQuery = window.matchMedia('(prefers-color-scheme: dark)')
    const handleMediaChange = (event: MediaQueryListEvent) => {
      if (document.documentElement.classList.contains('dark')) {
        return
      }
      setScheme(event.matches ? 'dark' : 'light')
    }
    mediaQuery.addEventListener('change', handleMediaChange)

    const observer = new MutationObserver(() => {
      setScheme(
        document.documentElement.classList.contains('dark')
          ? 'dark'
          : mediaQuery.matches
            ? 'dark'
            : 'light',
      )
    })
    observer.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ['class'],
    })

    return () => {
      mediaQuery.removeEventListener('change', handleMediaChange)
      observer.disconnect()
    }
  }, [])

  return scheme
}

interface CodeBlockProps {
  code: string
  language?: string
}

/**
 * highlightCode returns Highlight.js markup for the provided code sample.
 */
function highlightCode(source: string, language?: string): string {
  const lang = language?.toLowerCase()
  const trimmed = source ?? ''
  if (lang && hljs.getLanguage(lang)) {
    return hljs.highlight(trimmed, { language: lang }).value
  }
  // If no language or unrecognized language, use plaintext to avoid auto-highlighting
  return hljs.highlight(trimmed, { language: 'plaintext' }).value
}

/**
 * normalizeCodeBlockContent trims trailing blank lines to at most one without
 * introducing extra whitespace when none existed.
 */
function normalizeCodeBlockContent(source: string): string {
  const normalized = (source ?? '').replace(/\r\n/g, '\n')
  const lines = normalized.split('\n')

  const isBlankLine = (line: string) => {
    const withoutAnsi = line.replace(/\x1B\[[0-9;]*[A-Za-z]/g, '')
    return withoutAnsi.replace(/[\p{White_Space}\p{Cf}\p{Cc}]/gu, '') === ''
  }

  let trailingEmpty = 0
  for (let i = lines.length - 1; i >= 0; i -= 1) {
    if (isBlankLine(lines[i])) {
      trailingEmpty += 1
    } else {
      break
    }
  }

  if (trailingEmpty > 1) {
    lines.splice(lines.length - (trailingEmpty - 1), trailingEmpty - 1)
    console.debug('[Markdown] trimmed trailing blank lines', {
      removed: trailingEmpty - 1,
    })
  }

  const joined = lines.join('\n')
  if (joined.trim() === '') {
    return ''
  }
  return joined
}

/**
 * trimTrailingBlankLines ensures at most one trailing blank line for display.
 */
function trimTrailingBlankLines(lines: string[]): string[] {
  if (lines.length === 0) {
    return ['']
  }

  const isBlankLine = (line: string) => {
    const withoutAnsi = line.replace(/\x1B\[[0-9;]*[A-Za-z]/g, '')
    return withoutAnsi.replace(/[\p{White_Space}\p{Cf}\p{Cc}]/gu, '') === ''
  }

  let trailingEmpty = 0
  for (let i = lines.length - 1; i >= 0; i -= 1) {
    if (isBlankLine(lines[i])) {
      trailingEmpty += 1
    } else {
      break
    }
  }

  if (trailingEmpty > 1) {
    return lines.slice(0, lines.length - (trailingEmpty - 1))
  }

  return lines
}

/**
 * CodeBlock renders multi-line code with line numbers, syntax colors, and copy controls.
 */
function CodeBlock({ code, language }: CodeBlockProps) {
  // Normalize line endings and keep a single trailing newline to prevent extra blank lines
  const normalized = useMemo(() => normalizeCodeBlockContent(code), [code])
  const displayLines = useMemo(
    () => trimTrailingBlankLines(normalized.split('\n')),
    [normalized],
  )
  const highlighted = useMemo(() => {
    const highlightedText = highlightCode(displayLines.join('\n'), language)
    const highlightedLines = highlightedText.split('\n')
    if (highlightedLines.length > displayLines.length) {
      return highlightedLines.slice(0, displayLines.length).join('\n')
    }
    return highlightedText
  }, [language, displayLines])
  const [copied, setCopied] = useState(false)
  const lines = useMemo(() => {
    return displayLines.length > 0 ? displayLines : ['']
  }, [displayLines])

  const handleCopy = useCallback(async () => {
    if (typeof navigator === 'undefined' || !navigator.clipboard) {
      return
    }
    try {
      await navigator.clipboard.writeText(normalized)
      setCopied(true)
      setTimeout(() => setCopied(false), 1600)
    } catch (err) {
      console.error('Failed to copy code block:', err)
    }
  }, [normalized])

  const languageLabel = (language || 'plain text').toUpperCase()

  return (
    <figure className="code-shell not-prose">
      <div className="code-shell__toolbar">
        <span className="font-mono text-[11px] uppercase tracking-[0.2em]">
          {languageLabel}
        </span>
        <button
          type="button"
          onClick={handleCopy}
          className="code-shell__action"
          aria-label="Copy code block"
        >
          {copied ? (
            <span className="inline-flex items-center gap-1 text-success">
              <Check className="h-3 w-3" /> Copied
            </span>
          ) : (
            <span className="inline-flex items-center gap-1">
              <Copy className="h-3 w-3" /> Copy
            </span>
          )}
        </button>
      </div>
      <div className="code-shell__surface" tabIndex={0}>
        <div className="code-shell__gutter" aria-hidden="true">
          {lines.map((_, index) => (
            <span key={`line-${index + 1}`}>{index + 1}</span>
          ))}
        </div>
        <div className="code-shell__content">
          <pre className="hljs">
            <code
              className="hljs"
              dangerouslySetInnerHTML={{ __html: highlighted }}
            />
          </pre>
        </div>
      </div>
    </figure>
  )
}

interface MermaidDiagramProps {
  code: string
}

/**
 * MermaidDiagram renders charts with copy + raw view controls and theme-aware output.
 */
function MermaidDiagram({ code }: MermaidDiagramProps) {
  const containerRef = useRef<HTMLDivElement>(null)
  const [showSource, setShowSource] = useState(false)
  const [copied, setCopied] = useState(false)
  const [renderError, setRenderError] = useState<string | null>(null)
  const scheme = useColorScheme()

  useEffect(() => {
    if (showSource) {
      return
    }
    let cancelled = false
    setRenderError(null)
    if (containerRef.current) {
      containerRef.current.innerHTML = ''
    }

    ;(async () => {
      try {
        const mermaid = (await import('mermaid')).default

        // Initialize with proper config
        mermaid.initialize({
          startOnLoad: false,
          theme: scheme === 'dark' ? 'dark' : 'default',
          securityLevel: 'loose',
          fontFamily: 'inherit',
          flowchart: {
            useMaxWidth: true,
            htmlLabels: true,
            curve: 'basis',
          },
        })

        if (!containerRef.current || cancelled) {
          return
        }

        // Parse to validate syntax first
        const trimmedCode = code.trim()
        if (!trimmedCode) {
          setRenderError('Empty diagram code')
          return
        }

        const isValid = await mermaid.parse(trimmedCode)
        if (!isValid) {
          setRenderError('Invalid Mermaid syntax')
          return
        }

        // Render the diagram
        const { svg } = await mermaid.render(
          `mermaid-${Date.now()}-${Math.random().toString(36).slice(2)}`,
          trimmedCode,
        )

        if (!cancelled && containerRef.current) {
          containerRef.current.innerHTML = svg
        }
      } catch (err) {
        console.error('Mermaid render failed:', err)
        if (!cancelled) {
          const errorMsg = err instanceof Error ? err.message : String(err)
          setRenderError(`Mermaid error: ${errorMsg}`)
        }
      }
    })()

    return () => {
      cancelled = true
    }
  }, [code, scheme, showSource])

  const handleCopy = useCallback(async () => {
    if (typeof navigator === 'undefined' || !navigator.clipboard) {
      return
    }
    try {
      await navigator.clipboard.writeText(code)
      setCopied(true)
      setTimeout(() => setCopied(false), 1600)
    } catch (err) {
      console.error('Failed to copy mermaid source:', err)
    }
  }, [code])

  return (
    <figure className="code-shell not-prose">
      <div className="code-shell__toolbar">
        <span className="font-mono text-[11px] uppercase tracking-[0.2em]">
          Mermaid Diagram
        </span>
        <div className="flex items-center gap-2">
          <button
            type="button"
            className="code-shell__action"
            onClick={() => setShowSource((prev) => !prev)}
          >
            {showSource ? (
              <span className="inline-flex items-center gap-1">
                <Eye className="h-3 w-3" /> View Diagram
              </span>
            ) : (
              <span className="inline-flex items-center gap-1">
                <EyeOff className="h-3 w-3" /> View Source
              </span>
            )}
          </button>
          <button
            type="button"
            className="code-shell__action"
            onClick={handleCopy}
          >
            {copied ? (
              <span className="inline-flex items-center gap-1 text-success">
                <Check className="h-3 w-3" /> Copied
              </span>
            ) : (
              <span className="inline-flex items-center gap-1">
                <Copy className="h-3 w-3" /> Copy Source
              </span>
            )}
          </button>
        </div>
      </div>
      <div className="code-shell__surface" tabIndex={0}>
        {showSource ? (
          <pre className="code-shell__raw">{code}</pre>
        ) : renderError ? (
          <div className="code-shell__error">{renderError}</div>
        ) : (
          <div ref={containerRef} className="code-shell__diagram" />
        )}
      </div>
    </figure>
  )
}

const renderCode = ({
  inline,
  className,
  children,
  ...props
}: CodeRendererProps) => {
  const match = /language-(\w+)/.exec(className || '')
  const lang = match?.[1]
  const rawContent = Array.isArray(children)
    ? children.map((child) => String(child ?? '')).join('')
    : String(children ?? '')

  // Mermaid diagrams
  if (lang === 'mermaid') {
    return <MermaidDiagram code={normalizeCodeBlockContent(rawContent)} />
  }

  // Only render as code block if:
  // 1. Has a language class AND not inline
  // 2. OR has multiple lines
  const hasMultipleLines = rawContent.includes('\n')
  const shouldRenderAsBlock = !inline && (className || hasMultipleLines)

  if (shouldRenderAsBlock) {
    return (
      <CodeBlock code={normalizeCodeBlockContent(rawContent)} language={lang} />
    )
  }

  // Inline code
  return (
    <code
      className={cn(
        'rounded bg-muted px-1 py-0.5 text-[0.9em] font-mono text-foreground',
        className,
      )}
      {...props}
    >
      {children}
    </code>
  )
}

const renderImage = ({ src, alt }: { src?: string; alt?: string }) => {
  return (
    <img
      src={src}
      alt={alt}
      className="max-w-full h-auto rounded-lg my-2 border border-border"
      loading="lazy"
      decoding="async"
    />
  )
}

const components: Components = {
  code: renderCode,
  img: renderImage,
}

export type MarkdownProps = {
  children: string
  className?: string
}

/**
 * Markdown renders markdown content with syntax highlighting, math, and Mermaid diagrams.
 */
export function Markdown({ children, className }: MarkdownProps) {
  return (
    <div className={className}>
      <ReactMarkdown
        remarkPlugins={[remarkGfm, remarkMath]}
        rehypePlugins={[rehypeKatex, rehypeRaw]}
        components={components}
        urlTransform={(url, key) => sanitizeMarkdownUrl(url, key)}
      >
        {children}
      </ReactMarkdown>
    </div>
  )
}
