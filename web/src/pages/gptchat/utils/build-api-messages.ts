import type { ChatMessage as ApiChatMessage, ContentPart } from '@/utils/api'

import type { ChatAttachment, ChatMessageData, SessionConfig } from '../types'

/**
 * buildApiMessages constructs the chat API payload by applying the system prompt and recent
 * context messages to the current user content.
 */
export function buildApiMessages(
  config: SessionConfig,
  context: ChatMessageData[],
  userContent: string | ContentPart[],
): ApiChatMessage[] {
  const apiMessages: ApiChatMessage[] = []
  const isFreeTier = config.api_token.startsWith('FREETIER')

  if (config.system_prompt) {
    apiMessages.push({ role: 'system', content: config.system_prompt })
  }

  const userParts = Array.isArray(userContent) ? userContent : null
  const userHasImages = !!userParts?.some((part) => part.type === 'image_url')
  const fileNotePrefix = '[File uploaded:'

  const contentHasFileNotes = (content: string | ContentPart[]): boolean => {
    if (typeof content === 'string') {
      return content.includes(fileNotePrefix)
    }
    return content.some(
      (part) => part.type === 'text' && !!part.text?.includes(fileNotePrefix),
    )
  }

  const userHasFileNotes = contentHasFileNotes(userContent)
  const userHasMedia = userHasImages || userHasFileNotes

  const hasImageAttachment = (msg: ChatMessageData) =>
    !!msg.attachments?.some((att) => att.type === 'image' && !!att.contentB64)

  const hasFileAttachment = (msg: ChatMessageData) =>
    !!msg.attachments?.some((att) => att.type === 'file')

  const hasMediaAttachment = (msg: ChatMessageData) =>
    hasImageAttachment(msg) || hasFileAttachment(msg)

  const stripFileNotes = (
    text: string,
    attachments?: ChatAttachment[],
  ): string => {
    if (!attachments || attachments.length === 0) {
      return text
    }

    let cleaned = text
    for (const att of attachments) {
      if (att.type !== 'file') continue
      const note = att.url
        ? `[File uploaded: ${att.filename} (url: ${att.url})]`
        : `[File uploaded: ${att.filename}]`
      cleaned = cleaned.replaceAll(note, '')
    }

    cleaned = cleaned.replace(/\n{3,}/g, '\n\n').trim()
    return cleaned
  }

  let latestContextMediaIdx: number | null = null
  if (!userHasMedia) {
    for (let i = context.length - 1; i >= 0; i -= 1) {
      const msg = context[i]
      if (!hasMediaAttachment(msg)) {
        continue
      }

      if (msg.role === 'user') {
        const next = context[i + 1]
        if (
          next &&
          next.role === 'assistant' &&
          next.chatID === msg.chatID &&
          hasMediaAttachment(next)
        ) {
          latestContextMediaIdx = i + 1
          break
        }
      }

      latestContextMediaIdx = i
      break
    }
  }

  for (let i = 0; i < context.length; i += 1) {
    const msg = context[i]
    let content: string | ContentPart[] = msg.content
    if (isFreeTier) {
      // Free tier: strip all images from history context messages
      content = stripFileNotes(msg.content, msg.attachments)
    } else if (latestContextMediaIdx === i) {
      const parts: ContentPart[] = [{ type: 'text', text: msg.content }]
      if (msg.attachments) {
        for (const att of msg.attachments) {
          if (att.type === 'image' && att.contentB64) {
            parts.push({
              type: 'image_url',
              image_url: { url: att.contentB64 },
            })
          }
        }
      }
      content = parts
    } else {
      content = stripFileNotes(msg.content, msg.attachments)
    }
    apiMessages.push({ role: msg.role, content })
  }

  let finalUserContent = userContent
  if (userHasImages && userParts) {
    const parts: ContentPart[] = []
    for (const part of userParts) {
      if (part.type === 'text' || part.type === 'image_url') {
        parts.push(part)
      }
    }
    // Free tier: keep only the last image in the current message
    if (isFreeTier) {
      const nonImageParts = parts.filter((p) => p.type !== 'image_url')
      const imageParts = parts.filter((p) => p.type === 'image_url')
      if (imageParts.length > 1) {
        parts.length = 0
        parts.push(...nonImageParts, imageParts[imageParts.length - 1])
      }
    }
    finalUserContent = parts
  } else if (userParts) {
    const textParts = userParts.filter((p) => p.type === 'text')
    if (textParts.length === 1) {
      finalUserContent = textParts[0].text || ''
    } else {
      finalUserContent = textParts
    }
  }

  apiMessages.push({ role: 'user', content: finalUserContent })

  return apiMessages
}
