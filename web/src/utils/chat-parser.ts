import type { Annotation, ChatReference } from '@/pages/gptchat/types'

export const ToolEventPrefixes = [
  'Upstream tool_call:',
  'args:',
  'exec MCP tool:',
  'tool ok',
  'tool error:',
  'tool loop limit reached',
]

export const ReasoningStageThinking = 'thinking'
export const ReasoningStageTools = 'tools'

export interface ParsedContent {
  thinking: string
  toolEvents: string[]
  content: string
}

/**
 * Check if a line looks like a tool event
 */
export function isToolEventLine(line: string): boolean {
  const s = String(line || '').trim()
  if (!s) return false
  return ToolEventPrefixes.some((p) => s.startsWith(p))
}

/**
 * Split tool events chunk into individual lines
 */
export function splitToolEventsChunk(chunk: string): string[] {
  let text = String(chunk || '')
  if (!text.trim()) return []

  // Normalize newlines
  text = text.replace(/\r\n/g, '\n').replace(/\r/g, '\n')

  // Insert newline before known prefixes if they are concatenated
  ToolEventPrefixes.forEach((prefix) => {
    text = text.replaceAll(` ${prefix}`, `\n${prefix}`)
    text = text.replaceAll(`\t${prefix}`, `\n${prefix}`)
  })

  const lines = text
    .split('\n')
    .map((l) => String(l || '').trim())
    .filter(Boolean)

  if (!lines.some(isToolEventLine)) {
    return [text.trim()]
  }

  return lines
}

/**
 * Split reasoning content into thinking and tool events
 */
export function splitReasoningContent(reasoningContent: string): {
  thinking: string
  toolEvents: string[]
} {
  const raw = String(reasoningContent || '')
  if (!raw.trim()) {
    return { thinking: '', toolEvents: [] }
  }

  const lines = raw.replace(/\r\n/g, '\n').replace(/\r/g, '\n').split('\n')
  const thinkingLines: string[] = []
  const toolEvents: string[] = []

  lines.forEach((line) => {
    const s = String(line || '').trim()
    if (!s) return

    if (isToolEventLine(s)) {
      toolEvents.push(s)
    } else {
      thinkingLines.push(line)
    }
  })

  return {
    thinking: thinkingLines.join('\n').trim(),
    toolEvents,
  }
}

export function extractReferencesFromAnnotations(
  annotations?: Annotation[],
): ChatReference[] {
  if (!annotations || annotations.length === 0) {
    return []
  }

  const refs = new Map<string, { index: number; title?: string }>()
  let counter = 1

  for (const annotation of annotations) {
    if (annotation?.type !== 'url_citation') {
      continue
    }
    const citation = annotation.url_citation
    if (!citation?.url) {
      continue
    }
    if (!refs.has(citation.url)) {
      refs.set(citation.url, {
        index: counter++,
        title: citation.title || citation.url,
      })
    }
  }

  return Array.from(refs.entries()).map(([url, meta]) => ({
    url,
    title: meta.title,
    index: meta.index,
  }))
}
