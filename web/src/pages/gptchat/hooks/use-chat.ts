import { useCallback, useRef, useState } from 'react'

import {
  type ChatMessage as ApiChatMessage,
  type ContentPart,
} from '@/utils/api'
import { kvDel } from '@/utils/storage'

import {
  ChatModelDeepResearch,
  ChatModelGPTO3Deepresearch,
  ChatModelGPTO4MiniDeepresearch,
  isImageModel,
} from '../models'
import {
  type ChatAttachment,
  type ChatMessageData,
  type SessionConfig,
} from '../types'
import { generateChatId, getChatDataKey } from '../utils/chat-storage'
import { runDeepResearch } from './chat-deep-research'
import { runImageModelFlow, runMaskInpainting } from './chat-media'
import { useChatStorage } from './chat-storage'
import { useChatStreaming } from './chat-streaming'

/**
 * UseChatOptions describes how to configure the useChat hook. The sessionId scopes persisted
 * conversations and config provides the model and UI settings.
 */
export interface UseChatOptions {
  sessionId: number
  config: SessionConfig
}

/**
 * UseChatReturn lists state and handlers exposed by useChat, including helpers for sending and
 * editing messages plus lifecycle actions like loading and clearing history.
 */
export interface UseChatReturn {
  messages: ChatMessageData[]
  isLoading: boolean
  error: string | null
  sendMessage: (content: string, attachments?: File[]) => Promise<void>
  stopGeneration: () => void
  clearMessages: () => Promise<void>
  deleteMessage: (chatId: string) => Promise<void>
  loadMessages: () => Promise<void>
  regenerateMessage: (chatId: string) => Promise<void>
  editAndRetry: (
    chatId: string,
    newContent: string,
    attachments?: ChatAttachment[],
  ) => Promise<void>
}

/**
 * buildApiMessages constructs the chat API payload by applying the system prompt and recent
 * context messages to the current user content.
 */
function buildApiMessages(
  config: SessionConfig,
  context: ChatMessageData[],
  userContent: string | ContentPart[],
): ApiChatMessage[] {
  const apiMessages: ApiChatMessage[] = []

  if (config.system_prompt) {
    apiMessages.push({ role: 'system', content: config.system_prompt })
  }

  for (const msg of context) {
    let content: string | ContentPart[] = msg.content
    if (msg.attachments && msg.attachments.length > 0) {
      const parts: ContentPart[] = [{ type: 'text', text: msg.content }]
      for (const att of msg.attachments) {
        if (att.type === 'image' && att.contentB64) {
          parts.push({
            type: 'image_url',
            image_url: { url: att.contentB64 },
          })
        }
      }
      if (parts.length > 1) {
        content = parts
      }
    }
    apiMessages.push({ role: msg.role, content })
  }

  apiMessages.push({ role: 'user', content: userContent })

  return apiMessages
}

/**
 * useChat coordinates persistence, streaming, and special flows (media, deep research) for a
 * single chat session and returns state plus action helpers.
 */
export function useChat({ sessionId, config }: UseChatOptions): UseChatReturn {
  const [messages, setMessages] = useState<ChatMessageData[]>([])
  const [isLoading, setIsLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const abortControllerRef = useRef<AbortController | null>(null)
  const currentChatIdRef = useRef<string | null>(null)
  const deepResearchAbortRef = useRef(false)

  const { loadMessages, saveMessage, clearMessages, deleteMessage } =
    useChatStorage({ sessionId, setMessages, setError })

  const { streamAssistantReply } = useChatStreaming({
    config,
    setMessages,
    setIsLoading,
    setError,
    saveMessage,
    abortControllerRef,
    currentChatIdRef,
  })

  /**
   * fileToDataUrl converts a File object to a base64 data URL string for inline usage.
   */
  const fileToDataUrl = useCallback(async (file: File): Promise<string> => {
    return new Promise<string>((resolve, reject) => {
      const reader = new FileReader()
      reader.onloadend = () => resolve(reader.result as string)
      reader.onerror = () => reject(new Error('Failed to read file'))
      reader.readAsDataURL(file)
    })
  }, [])

  /**
   * findMaskPair searches a file list for matching image and mask pairs used by inpainting flows.
   */
  const findMaskPair = useCallback((files?: File[]) => {
    if (!files || files.length === 0) return null
    const normalizeName = (name: string) => name.replace(/\.[^.]+$/, '')

    for (const file of files) {
      const lower = file.name.toLowerCase()
      if (!lower.includes('-mask')) continue
      const base = normalizeName(lower.split('-mask')[0])
      const image = files.find((f) => {
        if (f === file) return false
        const fname = normalizeName(f.name.toLowerCase())
        return fname === base || fname.startsWith(base)
      })
      if (image) {
        return { image, mask: file }
      }
    }
    return null
  }, [])

  /**
   * isDeepResearchModel determines whether the selected model triggers the deep research flow.
   */
  const isDeepResearchModel = useCallback(() => {
    const model = config.selected_model
    return (
      model === ChatModelDeepResearch ||
      model === ChatModelGPTO3Deepresearch ||
      model === ChatModelGPTO4MiniDeepresearch
    )
  }, [config.selected_model])

  /**
   * sendMessage handles user submission, attachment processing, and dispatches assistant streams.
   */
  const sendMessage = useCallback(
    async (content: string, attachments?: File[]) => {
      const safeContent = String(content || '').trim()
      if (!safeContent) return

      const chatId = generateChatId()
      currentChatIdRef.current = chatId
      setError(null)

      const contentParts: ContentPart[] = []
      const attachmentMarkdown: string[] = []

      let finalContent = safeContent

      const pushTextPart = (text: string) => {
        if (!text) return
        contentParts.push({ type: 'text', text })
      }

      pushTextPart(finalContent)

      if (attachments && attachments.length > 0) {
        try {
          const { uploadFiles } = await import('@/utils/api')

          for (const file of attachments) {
            if (file.type.startsWith('image/')) {
              const b64 = await fileToDataUrl(file)
              attachmentMarkdown.push(`![${file.name}](${b64})`)
              contentParts.push({ type: 'image_url', image_url: { url: b64 } })
            } else {
              const { cache_keys } = await uploadFiles([file], config.api_token)
              const note = cache_keys?.[0]
                ? `[File uploaded: ${file.name} (key: ${cache_keys[0]})]`
                : `[File uploaded: ${file.name}]`
              attachmentMarkdown.push(note)
              pushTextPart(`\n\n${note}`)
            }
          }
        } catch (err) {
          console.error('File upload failed', err)
          setError('Failed to process attachments')
          return
        }
      }

      if (attachmentMarkdown.length > 0) {
        // Only append non-image attachments to finalContent for display.
        // Images are handled via the attachments field in ChatMessageData.
        const nonImageMarkdown = attachmentMarkdown.filter(
          (m) => !m.startsWith('!['),
        )
        if (nonImageMarkdown.length > 0) {
          finalContent =
            `${finalContent}\n\n${nonImageMarkdown.join('\n\n')}`.trim()
        }
      }

      const userMessage: ChatMessageData = {
        chatID: chatId,
        role: 'user',
        content: finalContent,
        timestamp: Date.now(),
        attachments: attachments?.map((file) => ({
          filename: file.name,
          type: file.type.startsWith('image/') ? 'image' : 'file',
          url: file.type.startsWith('image/') ? undefined : undefined, // will be handled by upload if needed
        })),
      }

      // For images, we want to store the base64 in the attachment so it can be rendered
      if (attachments && userMessage.attachments) {
        for (let i = 0; i < attachments.length; i++) {
          if (attachments[i].type.startsWith('image/')) {
            userMessage.attachments[i].contentB64 = await fileToDataUrl(
              attachments[i],
            )
          }
        }
      }

      setMessages((prev) => [...prev, userMessage])
      await saveMessage(userMessage)

      const maskPair = findMaskPair(attachments)
      if (maskPair) {
        await runMaskInpainting({
          chatId,
          prompt: content,
          maskPair,
          config,
          fileToDataUrl,
          setMessages,
          setIsLoading,
          setError,
          saveMessage,
          currentChatIdRef,
        })
        return
      }

      if (isDeepResearchModel()) {
        await runDeepResearch({
          chatId,
          prompt: content,
          config,
          setMessages,
          setIsLoading,
          setError,
          saveMessage,
          deepResearchAbortRef,
          currentChatIdRef,
        })
        return
      }

      if (isImageModel(config.selected_model)) {
        await runImageModelFlow({
          chatId,
          prompt: content,
          config,
          setMessages,
          setIsLoading,
          setError,
          saveMessage,
          currentChatIdRef,
        })
        return
      }

      const assistantMessage: ChatMessageData = {
        chatID: chatId,
        role: 'assistant',
        content: '',
        model: config.selected_model,
        timestamp: Date.now(),
      }

      setMessages((prev) => [...prev, assistantMessage])

      const contextMessages = messages.slice(-config.n_contexts * 2)
      const apiMessages = buildApiMessages(
        config,
        contextMessages,
        contentParts.length > 1 ? contentParts : finalContent,
      )

      await streamAssistantReply({ chatId, payload: apiMessages })
    },
    [
      config,
      currentChatIdRef,
      findMaskPair,
      fileToDataUrl,
      isDeepResearchModel,
      messages,
      runDeepResearch,
      runImageModelFlow,
      runMaskInpainting,
      saveMessage,
      streamAssistantReply,
    ],
  )

  /**
   * stopGeneration cancels any in-flight assistant generation or deep research runs.
   */
  const stopGeneration = useCallback(() => {
    if (abortControllerRef.current) {
      abortControllerRef.current.abort()
      abortControllerRef.current = null
      setIsLoading(false)
    }
    if (!deepResearchAbortRef.current) {
      deepResearchAbortRef.current = true
    }
  }, [])

  /**
   * regenerateMessage replays the assistant response for an existing user turn.
   */
  const regenerateMessage = useCallback(
    async (chatId: string) => {
      const userIndex = messages.findIndex(
        (m) => m.chatID === chatId && m.role === 'user',
      )
      if (userIndex === -1) {
        return
      }

      const assistantIndex = messages.findIndex(
        (m) => m.chatID === chatId && m.role === 'assistant',
      )

      const userMsg = messages[userIndex]

      await kvDel(getChatDataKey(chatId, 'assistant'))

      setMessages((prev) => {
        const next = [...prev]
        if (assistantIndex >= 0) {
          next[assistantIndex] = {
            chatID: chatId,
            role: 'assistant',
            content: '',
            model: config.selected_model,
            timestamp: Date.now(),
          }
        } else {
          next.splice(userIndex + 1, 0, {
            chatID: chatId,
            role: 'assistant',
            content: '',
            model: config.selected_model,
            timestamp: Date.now(),
          })
        }
        return next
      })

      const priorMessages = messages.slice(0, userIndex)
      const contextMessages = priorMessages.slice(-config.n_contexts * 2)
      const apiMessages = buildApiMessages(
        config,
        contextMessages,
        userMsg.content,
      )

      await streamAssistantReply({ chatId, payload: apiMessages })
    },
    [config, messages, streamAssistantReply],
  )

  /**
   * editAndRetry updates a user message then streams a replacement assistant response.
   */
  const editAndRetry = useCallback(
    async (
      chatId: string,
      newContent: string,
      attachments?: ChatAttachment[],
    ) => {
      const trimmed = String(newContent || '').trim()
      if (!trimmed) {
        return
      }

      const userIndex = messages.findIndex(
        (m) => m.chatID === chatId && m.role === 'user',
      )
      if (userIndex === -1) {
        return
      }

      const assistantIndex = messages.findIndex(
        (m) => m.chatID === chatId && m.role === 'assistant',
      )

      const updatedUser: ChatMessageData = {
        ...messages[userIndex],
        content: trimmed,
        attachments: attachments || messages[userIndex].attachments,
        timestamp: Date.now(),
      }

      await saveMessage(updatedUser)
      await kvDel(getChatDataKey(chatId, 'assistant'))

      setMessages((prev) => {
        const next = [...prev]
        next[userIndex] = updatedUser

        if (assistantIndex >= 0) {
          next[assistantIndex] = {
            chatID: chatId,
            role: 'assistant',
            content: '',
            model: config.selected_model,
            timestamp: Date.now(),
          }
        } else {
          next.splice(userIndex + 1, 0, {
            chatID: chatId,
            role: 'assistant',
            content: '',
            model: config.selected_model,
            timestamp: Date.now(),
          })
        }

        return next
      })

      const priorMessages = messages.slice(0, userIndex)
      const contextMessages = priorMessages.slice(-config.n_contexts * 2)

      // Reconstruct content parts if there are attachments
      let userContent: string | ContentPart[] = trimmed
      if (updatedUser.attachments && updatedUser.attachments.length > 0) {
        const parts: ContentPart[] = [{ type: 'text', text: trimmed }]
        for (const att of updatedUser.attachments) {
          if (att.type === 'image' && att.contentB64) {
            parts.push({
              type: 'image_url',
              image_url: { url: att.contentB64 },
            })
          }
        }
        if (parts.length > 1) {
          userContent = parts
        }
      }

      const apiMessages = buildApiMessages(config, contextMessages, userContent)

      await streamAssistantReply({ chatId, payload: apiMessages })
    },
    [config, messages, saveMessage, streamAssistantReply],
  )

  return {
    messages,
    isLoading,
    error,
    sendMessage,
    stopGeneration,
    clearMessages,
    deleteMessage,
    loadMessages,
    regenerateMessage,
    editAndRetry,
  }
}
