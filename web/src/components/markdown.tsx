import { useEffect, useRef } from 'react'
import ReactMarkdown from 'react-markdown'
import type { Components } from 'react-markdown'
import rehypeHighlight from 'rehype-highlight'
import rehypeKatex from 'rehype-katex'
import remarkGfm from 'remark-gfm'
import remarkMath from 'remark-math'

import 'katex/dist/katex.min.css'
import 'highlight.js/styles/github-dark.min.css'

/**
 * MermaidDiagram renders a Mermaid diagram from the provided code.
 */
function MermaidDiagram({ code }: { code: string }) {
  const containerRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    let cancelled = false

    async function render() {
      const mermaid = (await import('mermaid')).default
      mermaid.initialize({
        startOnLoad: false,
        theme: document.documentElement.classList.contains('dark') ? 'dark' : 'default',
      })

      if (cancelled || !containerRef.current) return

      try {
        const { svg } = await mermaid.render(`mermaid-${Math.random().toString(36).slice(2)}`, code)
        if (!cancelled && containerRef.current) {
          containerRef.current.innerHTML = svg
        }
      } catch {
        if (!cancelled && containerRef.current) {
          containerRef.current.textContent = code
        }
      }
    }

    render()
    return () => {
      cancelled = true
    }
  }, [code])

  return <div ref={containerRef} className="my-4 overflow-x-auto" />
}

const components: Components = {
  code({ className, children, ...props }) {
    const match = /language-(\w+)/.exec(className || '')
    const lang = match?.[1]

    // Check if it's a Mermaid block
    if (lang === 'mermaid') {
      return <MermaidDiagram code={String(children).replace(/\n$/, '')} />
    }

    // Inline code
    if (!className) {
      return (
        <code
          className="rounded bg-black/10 px-1 py-0.5 text-sm dark:bg-white/10"
          {...props}
        >
          {children}
        </code>
      )
    }

    // Block code - let rehype-highlight handle it
    return (
      <code className={className} {...props}>
        {children}
      </code>
    )
  },
  pre({ children, ...props }) {
    return (
      <pre
        className="my-4 overflow-x-auto rounded-lg bg-[#0d1117] p-4 text-sm"
        {...props}
      >
        {children}
      </pre>
    )
  },
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
        rehypePlugins={[rehypeHighlight, rehypeKatex]}
        components={components}
      >
        {children}
      </ReactMarkdown>
    </div>
  )
}
