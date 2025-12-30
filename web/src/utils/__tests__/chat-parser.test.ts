import { describe, expect, it } from 'vitest'
import {
  isToolEventLine,
  splitReasoningContent,
  splitToolEventsChunk,
  extractReferencesFromAnnotations,
  ToolEventPrefixes,
} from '../chat-parser'

describe('isToolEventLine', () => {
  it('should return true for tool event lines', () => {
    expect(isToolEventLine('Upstream tool_call: web_search')).toBe(true)
    expect(isToolEventLine('args: {"query":"test"}')).toBe(true)
    expect(
      isToolEventLine('exec MCP tool: web_search @ https://mcp.example.com'),
    ).toBe(true)
    expect(isToolEventLine('exec local tool: calculator')).toBe(true)
    expect(isToolEventLine('tool ok')).toBe(true)
    expect(isToolEventLine('tool error: Something went wrong')).toBe(true)
    expect(isToolEventLine('tool loop limit reached')).toBe(true)
  })

  it('should return true for lines with [[TOOLS]] marker', () => {
    expect(isToolEventLine('[[TOOLS]] Upstream tool_call: web_search')).toBe(
      true,
    )
    expect(isToolEventLine('[[TOOLS]] args: {"query":"test"}')).toBe(true)
    expect(isToolEventLine('[[TOOLS]] exec MCP tool: web_search')).toBe(true)
    expect(isToolEventLine('[[TOOLS]] tool ok')).toBe(true)
    expect(isToolEventLine('[[TOOLS]] tool error: Error')).toBe(true)
  })

  it('should return false for non-tool event lines', () => {
    expect(isToolEventLine('Hello world')).toBe(false)
    expect(isToolEventLine('')).toBe(false)
    expect(isToolEventLine('   ')).toBe(false)
    expect(isToolEventLine('Some random text')).toBe(false)
    expect(isToolEventLine('thinking about the problem...')).toBe(false)
  })

  it('should handle undefined and null', () => {
    expect(isToolEventLine(undefined as unknown as string)).toBe(false)
    expect(isToolEventLine(null as unknown as string)).toBe(false)
  })
})

describe('splitReasoningContent', () => {
  it('should split reasoning content into thinking and tool events', () => {
    const content = `I need to search the web.
Upstream tool_call: web_search
args: {"query":"test"}
tool ok
The search returned results.`

    const result = splitReasoningContent(content)

    expect(result.thinking).toBe(
      `I need to search the web.\nThe search returned results.`,
    )
    expect(result.toolEvents).toHaveLength(3)
    expect(result.toolEvents[0]).toBe('Upstream tool_call: web_search')
    expect(result.toolEvents[1]).toBe('args: {"query":"test"}')
    expect(result.toolEvents[2]).toBe('tool ok')
  })

  it('should strip [[TOOLS]] marker from tool events', () => {
    const content = `[[TOOLS]] Upstream tool_call: web_search
[[TOOLS]] args: {"query":"test"}
[[TOOLS]] tool ok`

    const result = splitReasoningContent(content)

    expect(result.toolEvents).toHaveLength(3)
    expect(result.toolEvents[0]).toBe('Upstream tool_call: web_search')
    expect(result.toolEvents[1]).toBe('args: {"query":"test"}')
    expect(result.toolEvents[2]).toBe('tool ok')
  })

  it('should handle empty content', () => {
    expect(splitReasoningContent('')).toEqual({ thinking: '', toolEvents: [] })
    expect(splitReasoningContent('   ')).toEqual({
      thinking: '',
      toolEvents: [],
    })
  })

  it('should handle content with only tool events', () => {
    const content = `Upstream tool_call: web_search
tool ok`

    const result = splitReasoningContent(content)

    expect(result.thinking).toBe('')
    expect(result.toolEvents).toHaveLength(2)
  })

  it('should handle content with only thinking', () => {
    const content = `I need to think about this.
Let me analyze the problem.
This is my conclusion.`

    const result = splitReasoningContent(content)

    expect(result.thinking).toBe(content)
    expect(result.toolEvents).toHaveLength(0)
  })
})

describe('splitToolEventsChunk', () => {
  it('should split concatenated tool events', () => {
    const chunk =
      'Upstream tool_call: web_search args: {"query":"test"} tool ok'
    const result = splitToolEventsChunk(chunk)

    expect(result.length).toBeGreaterThan(1)
  })

  it('should handle empty input', () => {
    expect(splitToolEventsChunk('')).toEqual([])
    expect(splitToolEventsChunk('   ')).toEqual([])
  })

  it('should return single item for non-tool content', () => {
    const chunk = 'Hello world'
    const result = splitToolEventsChunk(chunk)

    expect(result).toEqual(['Hello world'])
  })
})

describe('extractReferencesFromAnnotations', () => {
  it('should extract URL citations from annotations', () => {
    const annotations = [
      {
        type: 'url_citation',
        url_citation: {
          url: 'https://example.com',
          title: 'Example Site',
        },
      },
      {
        type: 'url_citation',
        url_citation: {
          url: 'https://test.com',
          title: 'Test Site',
        },
      },
    ]

    const refs = extractReferencesFromAnnotations(annotations)

    expect(refs).toHaveLength(2)
    expect(refs[0]).toEqual({
      url: 'https://example.com',
      title: 'Example Site',
      index: 1,
    })
    expect(refs[1]).toEqual({
      url: 'https://test.com',
      title: 'Test Site',
      index: 2,
    })
  })

  it('should deduplicate URLs', () => {
    const annotations = [
      {
        type: 'url_citation',
        url_citation: {
          url: 'https://example.com',
          title: 'Example Site',
        },
      },
      {
        type: 'url_citation',
        url_citation: {
          url: 'https://example.com',
          title: 'Duplicate',
        },
      },
    ]

    const refs = extractReferencesFromAnnotations(annotations)

    expect(refs).toHaveLength(1)
    expect(refs[0].url).toBe('https://example.com')
  })

  it('should skip non-url_citation annotations', () => {
    const annotations = [
      {
        type: 'other_type',
        data: 'something',
      },
      {
        type: 'url_citation',
        url_citation: {
          url: 'https://example.com',
          title: 'Example',
        },
      },
    ]

    const refs = extractReferencesFromAnnotations(annotations)

    expect(refs).toHaveLength(1)
  })

  it('should handle empty input', () => {
    expect(extractReferencesFromAnnotations(undefined)).toEqual([])
    expect(extractReferencesFromAnnotations([])).toEqual([])
  })
})

describe('ToolEventPrefixes', () => {
  it('should contain expected prefixes', () => {
    expect(ToolEventPrefixes).toContain('Upstream tool_call:')
    expect(ToolEventPrefixes).toContain('args:')
    expect(ToolEventPrefixes).toContain('exec MCP tool:')
    expect(ToolEventPrefixes).toContain('exec local tool:')
    expect(ToolEventPrefixes).toContain('tool ok')
    expect(ToolEventPrefixes).toContain('tool error:')
    expect(ToolEventPrefixes).toContain('tool loop limit reached')
  })

  it('should contain prefixes with [[TOOLS]] marker', () => {
    expect(ToolEventPrefixes).toContain('[[TOOLS]] Upstream tool_call:')
    expect(ToolEventPrefixes).toContain('[[TOOLS]] tool ok')
  })
})
