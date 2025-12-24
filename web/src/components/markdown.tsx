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
  return hljs.highlightAuto(trimmed).value
}

/**
 * CodeBlock renders multi-line code with line numbers, syntax colors, and copy controls.
 */
function CodeBlock({ code, language }: CodeBlockProps) {
  const normalized = useMemo(() => code.replace(/\r\n/g, '\n'), [code])
  const highlighted = useMemo(
    () => highlightCode(normalized, language),
    [language, normalized],
  )
  const [copied, setCopied] = useState(false)
  const lines = useMemo(() => {
    if (!normalized) {
      return ['']
    }
    return normalized.split('\n')
  }, [normalized])

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
      <div className="code-shell__surface">
        <div className="code-shell__gutter" aria-hidden="true">
          {lines.map((_, index) => (
            <span key={`line-${index + 1}`}>{index + 1}</span>
          ))}
        </div>
        <div className="code-shell__content">
          <pre className="hljs" tabIndex={0}>
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
      <div className="code-shell__surface">
        {showSource ? (
          <pre className="code-shell__raw" tabIndex={0}>
            {code}
          </pre>
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
  const content = String(children).replace(/\n$/, '')

  // Mermaid diagrams
  if (lang === 'mermaid') {
    return <MermaidDiagram code={content} />
  }

  // Only render as code block if:
  // 1. Has a language class AND not inline
  // 2. OR has multiple lines
  const hasMultipleLines = content.includes('\n')
  const shouldRenderAsBlock = !inline && (className || hasMultipleLines)

  if (shouldRenderAsBlock) {
    return <CodeBlock code={content} language={lang} />
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

const components: Components = {
  code: renderCode,
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
        rehypePlugins={[rehypeRaw, rehypeKatex]}
        components={components}
      >
        {children}
      </ReactMarkdown>
    </div>
  )
}
