import { describe, expect, it } from 'vitest'
import { buildApiMessages } from '../use-chat'
import { SessionConfig, ChatMessageData } from '../../types'

describe('buildApiMessages', () => {
  const config: SessionConfig = {
    selected_model: 'gpt-4o',
    n_contexts: 5,
    system_prompt: 'You are a helpful assistant',
    api_token: 'test-token',
    api_base: 'https://api.openai.com',
  }

  it('should retain only the latest image in the entire history', () => {
    const context: ChatMessageData[] = [
      {
        chatID: '1',
        role: 'user',
        content: 'Message 1',
        attachments: [
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

    // Message 1 should have NO image now
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

  it('should prefer image in userContent over context', () => {
    const context: ChatMessageData[] = [
      {
        chatID: '1',
        role: 'user',
        content: 'Message 1',
        attachments: [
          {
            filename: 'img1.png',
            type: 'image',
            contentB64: 'data:image/png;base64,img1',
          },
        ],
      },
    ]

    const userContent = [
      { type: 'text', text: 'Message 2' },
      { type: 'image_url', image_url: { url: 'data:image/png;base64,img2' } },
    ]

    const result = buildApiMessages(config, context, userContent)

    expect(result).toHaveLength(3)
    // Message 1 should have NO image
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

  it('should retain only the latest image if multiple images are in userContent', () => {
    const context: ChatMessageData[] = []
    const userContent = [
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
        { type: 'image_url', image_url: { url: 'data:image/png;base64,img2' } },
      ],
    })
  })

  it('should handle reconstructed userContent with images', () => {
    const context: ChatMessageData[] = []
    // This is what regenerateMessage/editAndRetry would pass to buildApiMessages
    const userContent = [
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
