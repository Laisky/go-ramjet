/**
 * Tests for chat-storage.ts utility functions
 */
import { describe, it, expect } from 'vitest'
import { sanitizeChatMessageData } from '../chat-storage'
import type { ChatMessageData } from '../../types'

describe('sanitizeChatMessageData', () => {
  const baseMessage: ChatMessageData = {
    chatID: 'test-chat-id',
    role: 'assistant',
    content: 'Hello, world!',
  }

  it('should return a valid message unchanged', () => {
    const input: ChatMessageData = {
      ...baseMessage,
      costUsd: 0.0012,
      timestamp: 1704067200000,
    }
    const result = sanitizeChatMessageData(input)

    expect(result.chatID).toBe('test-chat-id')
    expect(result.role).toBe('assistant')
    expect(result.content).toBe('Hello, world!')
    expect(result.costUsd).toBe(0.0012)
    expect(result.timestamp).toBe(1704067200000)
  })

  it('should convert costUsd string to number (backward compatibility)', () => {
    const input = {
      ...baseMessage,
      costUsd: '0.0012' as unknown as number,
    }
    const result = sanitizeChatMessageData(input)

    expect(result.costUsd).toBe(0.0012)
    expect(typeof result.costUsd).toBe('number')
  })

  it('should convert timestamp string to number', () => {
    const input = {
      ...baseMessage,
      timestamp: '1704067200000' as unknown as number,
    }
    const result = sanitizeChatMessageData(input)

    expect(result.timestamp).toBe(1704067200000)
    expect(typeof result.timestamp).toBe('number')
  })

  it('should set costUsd to undefined for invalid string', () => {
    const input = {
      ...baseMessage,
      costUsd: 'invalid' as unknown as number,
    }
    const result = sanitizeChatMessageData(input)

    expect(result.costUsd).toBeUndefined()
  })

  it('should set timestamp to undefined for invalid string', () => {
    const input = {
      ...baseMessage,
      timestamp: 'invalid' as unknown as number,
    }
    const result = sanitizeChatMessageData(input)

    expect(result.timestamp).toBeUndefined()
  })

  it('should handle null costUsd', () => {
    const input = {
      ...baseMessage,
      costUsd: null as unknown as number,
    }
    const result = sanitizeChatMessageData(input)

    // null should pass through the undefined/null check and not be processed
    expect(result.costUsd).toBeUndefined()
  })

  it('should convert non-string content to string', () => {
    const input = {
      ...baseMessage,
      content: 12345 as unknown as string,
    }
    const result = sanitizeChatMessageData(input)

    expect(result.content).toBe('12345')
    expect(typeof result.content).toBe('string')
  })

  it('should handle undefined content gracefully', () => {
    const input = {
      ...baseMessage,
      content: undefined as unknown as string,
    }
    const result = sanitizeChatMessageData(input)

    expect(result.content).toBe('')
    expect(typeof result.content).toBe('string')
  })

  it('should preserve other properties unchanged', () => {
    const input: ChatMessageData = {
      ...baseMessage,
      model: 'gpt-4',
      reasoningContent: 'Some reasoning...',
      error: 'Some error',
      references: [{ index: 1, url: 'https://example.com', title: 'Example' }],
    }
    const result = sanitizeChatMessageData(input)

    expect(result.model).toBe('gpt-4')
    expect(result.reasoningContent).toBe('Some reasoning...')
    expect(result.error).toBe('Some error')
    expect(result.references).toEqual([
      { index: 1, url: 'https://example.com', title: 'Example' },
    ])
  })

  it('should not mutate the original object', () => {
    const input = {
      ...baseMessage,
      costUsd: '0.0012' as unknown as number,
    }
    const originalCostUsd = input.costUsd

    sanitizeChatMessageData(input)

    // Original should remain unchanged
    expect(input.costUsd).toBe(originalCostUsd)
  })

  it('should handle costUsd of 0 correctly', () => {
    const input: ChatMessageData = {
      ...baseMessage,
      costUsd: 0,
    }
    const result = sanitizeChatMessageData(input)

    expect(result.costUsd).toBe(0)
    expect(typeof result.costUsd).toBe('number')
  })

  it('should handle costUsd string "0" correctly', () => {
    const input = {
      ...baseMessage,
      costUsd: '0' as unknown as number,
    }
    const result = sanitizeChatMessageData(input)

    expect(result.costUsd).toBe(0)
    expect(typeof result.costUsd).toBe('number')
  })

  it('should handle empty string costUsd', () => {
    const input = {
      ...baseMessage,
      costUsd: '' as unknown as number,
    }
    const result = sanitizeChatMessageData(input)

    // Empty string converts to 0 via Number('')
    expect(result.costUsd).toBe(0)
  })

  it('should handle scientific notation in costUsd string', () => {
    const input = {
      ...baseMessage,
      costUsd: '1.5e-3' as unknown as number,
    }
    const result = sanitizeChatMessageData(input)

    expect(result.costUsd).toBeCloseTo(0.0015)
  })
})
