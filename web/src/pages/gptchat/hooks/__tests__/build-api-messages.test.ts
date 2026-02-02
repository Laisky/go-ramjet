import { type ContentPart } from '@/utils/api'
import { describe, expect, it } from 'vitest'
import {
  DefaultSessionConfig,
  type ChatMessageData,
  type SessionConfig,
} from '../../types'
import { buildApiMessages } from '../use-chat'

describe('buildApiMessages', () => {
  const config: SessionConfig = {
    ...DefaultSessionConfig,
    selected_model: 'gpt-4o',
    n_contexts: 5,
    system_prompt: 'You are a helpful assistant',
    api_token: 'test-token',
    api_base: 'https://api.openai.com',
  }

  it('should retain only the latest media in the entire history', () => {
    const context: ChatMessageData[] = [
      {
        chatID: '1',
        role: 'user',
        content:
          'Message 1\n\n[File uploaded: report.pdf (url: https://example.com/report.pdf)]',
        attachments: [
          {
            filename: 'report.pdf',
            type: 'file',
            url: 'https://example.com/report.pdf',
          },
          {
            filename: 'img1.png',
            type: 'image',
            contentB64: 'data:image/png;base64,img1',
          },
        ],
      },
      {
        chatID: '2',
        role: 'assistant',
        content: 'I see image 1',
      },
      {
        chatID: '3',
        role: 'user',
        content: 'Message 2',
        attachments: [
          {
            filename: 'img2.png',
            type: 'image',
            contentB64: 'data:image/png;base64,img2',
          },
        ],
      },
    ]

    const userContent = 'Message 3'

    const result = buildApiMessages(config, context, userContent)

    // System prompt + 3 context messages + 1 user message = 5
    expect(result).toHaveLength(5)
    expect(result[0]).toEqual({
      role: 'system',
      content: 'You are a helpful assistant',
    })

    // Message 1 should have NO media now
    expect(result[1]).toEqual({ role: 'user', content: 'Message 1' })

    // Message 2 should have NO image
    expect(result[2]).toEqual({ role: 'assistant', content: 'I see image 1' })

    // Message 3 should have the image (it's the latest in context)
    expect(result[3]).toEqual({
      role: 'user',
      content: [
        { type: 'text', text: 'Message 2' },
        { type: 'image_url', image_url: { url: 'data:image/png;base64,img2' } },
      ],
    })

    // Current user message
    expect(result[4]).toEqual({ role: 'user', content: 'Message 3' })
  })

  it('should prefer user content media over context', () => {
    const context: ChatMessageData[] = [
      {
        chatID: '1',
        role: 'user',
        content: 'Message 1\n\n[File uploaded: legacy.csv]',
        attachments: [
          {
            filename: 'legacy.csv',
            type: 'file',
          },
          {
            filename: 'img1.png',
            type: 'image',
            contentB64: 'data:image/png;base64,img1',
          },
        ],
      },
    ]

    const userContent: ContentPart[] = [
      { type: 'text', text: 'Message 2' },
      { type: 'image_url', image_url: { url: 'data:image/png;base64,img2' } },
    ]

    const result = buildApiMessages(config, context, userContent)

    expect(result).toHaveLength(3)
    // Message 1 should have NO media
    expect(result[1]).toEqual({ role: 'user', content: 'Message 1' })
    // Current user message should have the image
    expect(result[2]).toEqual({
      role: 'user',
      content: [
        { type: 'text', text: 'Message 2' },
        { type: 'image_url', image_url: { url: 'data:image/png;base64,img2' } },
      ],
    })
  })

  it('should retain all images if multiple images are in userContent', () => {
    const context: ChatMessageData[] = []
    const userContent: ContentPart[] = [
      { type: 'text', text: 'Message 1' },
      { type: 'image_url', image_url: { url: 'data:image/png;base64,img1' } },
      { type: 'image_url', image_url: { url: 'data:image/png;base64,img2' } },
    ]

    const result = buildApiMessages(config, context, userContent)

    expect(result).toHaveLength(2)
    expect(result[1]).toEqual({
      role: 'user',
      content: [
        { type: 'text', text: 'Message 1' },
        { type: 'image_url', image_url: { url: 'data:image/png;base64,img1' } },
        { type: 'image_url', image_url: { url: 'data:image/png;base64,img2' } },
      ],
    })
  })

  it('should prefer assistant images when both user and assistant have images', () => {
    const context: ChatMessageData[] = [
      {
        chatID: '1',
        role: 'user',
        content: 'User with image',
        attachments: [
          {
            filename: 'img-user.png',
            type: 'image',
            contentB64: 'data:image/png;base64,user',
          },
        ],
      },
      {
        chatID: '1',
        role: 'assistant',
        content: 'Assistant with image',
        attachments: [
          {
            filename: 'img-assistant.png',
            type: 'image',
            contentB64: 'data:image/png;base64,assistant',
          },
        ],
      },
    ]

    const result = buildApiMessages(config, context, 'New prompt')

    expect(result).toHaveLength(4)
    expect(result[1]).toEqual({ role: 'user', content: 'User with image' })
    expect(result[2]).toEqual({
      role: 'assistant',
      content: [
        { type: 'text', text: 'Assistant with image' },
        {
          type: 'image_url',
          image_url: { url: 'data:image/png;base64,assistant' },
        },
      ],
    })
  })

  it('should drop historical media when user prompt has file notes', () => {
    const context: ChatMessageData[] = [
      {
        chatID: '1',
        role: 'user',
        content: 'Old message',
        attachments: [
          {
            filename: 'old.png',
            type: 'image',
            contentB64: 'data:image/png;base64,old',
          },
        ],
      },
      {
        chatID: '2',
        role: 'assistant',
        content: 'Old reply\n\n[File uploaded: notes.txt]',
        attachments: [
          {
            filename: 'notes.txt',
            type: 'file',
          },
        ],
      },
    ]

    const userContent = 'New prompt\n\n[File uploaded: input.csv]'

    const result = buildApiMessages(config, context, userContent)

    expect(result).toHaveLength(4)
    expect(result[1]).toEqual({ role: 'user', content: 'Old message' })
    expect(result[2]).toEqual({ role: 'assistant', content: 'Old reply' })
    expect(result[3]).toEqual({ role: 'user', content: userContent })
  })

  it('should handle reconstructed userContent with images', () => {
    const context: ChatMessageData[] = []
    // This is what regenerateMessage/editAndRetry would pass to buildApiMessages
    const userContent: ContentPart[] = [
      { type: 'text', text: 'Regenerated message' },
      { type: 'image_url', image_url: { url: 'data:image/png;base64,img1' } },
    ]

    const result = buildApiMessages(config, context, userContent)

    expect(result[1]).toEqual({
      role: 'user',
      content: [
        { type: 'text', text: 'Regenerated message' },
        { type: 'image_url', image_url: { url: 'data:image/png;base64,img1' } },
      ],
    })
  })
})
