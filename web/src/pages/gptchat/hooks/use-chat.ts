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
import { fileToDataUrl } from '../utils/format'
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
  loadingChatId: string | null
  error: string | null
  sendMessage: (
    content: string,
    attachments?: ChatAttachment[],
  ) => Promise<void>
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
export function buildApiMessages(
  config: SessionConfig,
  context: ChatMessageData[],
  userContent: string | ContentPart[],
): ApiChatMessage[] {
  const apiMessages: ApiChatMessage[] = []

  if (config.system_prompt) {
    apiMessages.push({ role: 'system', content: config.system_prompt })
  }

  // Find the latest image in the entire sequence (context + userContent)
  let latestImage: {
    type: 'context' | 'user'
    msgIdx?: number
    attIdx?: number
    partIdx?: number
  } | null = null

  // Check userContent (it's the latest)
  if (Array.isArray(userContent)) {
    for (let i = userContent.length - 1; i >= 0; i--) {
      if (userContent[i].type === 'image_url') {
        latestImage = { type: 'user', partIdx: i }
        break
      }
    }
  }

  // If not in userContent, check context from latest to oldest
  if (!latestImage) {
    for (let i = context.length - 1; i >= 0; i--) {
      const msg = context[i]
      if (msg.attachments) {
        for (let j = msg.attachments.length - 1; j >= 0; j--) {
          const att = msg.attachments[j]
          if (att.type === 'image' && att.contentB64) {
            latestImage = { type: 'context', msgIdx: i, attIdx: j }
            break
          }
        }
      }
      if (latestImage) break
    }
  }

  for (let i = 0; i < context.length; i++) {
    const msg = context[i]
    let content: string | ContentPart[] = msg.content
    if (latestImage?.type === 'context' && latestImage.msgIdx === i) {
      const parts: ContentPart[] = [{ type: 'text', text: msg.content }]
      const att = msg.attachments![latestImage.attIdx!]
      parts.push({
        type: 'image_url',
        image_url: { url: att.contentB64! },
      })
      content = parts
    }
    apiMessages.push({ role: msg.role, content })
  }

  let finalUserContent = userContent
  if (latestImage?.type === 'user') {
    if (Array.isArray(userContent)) {
      const parts: ContentPart[] = []
      for (let i = 0; i < userContent.length; i++) {
        const part = userContent[i]
        if (part.type === 'text') {
          parts.push(part)
        } else if (part.type === 'image_url' && i === latestImage.partIdx) {
          parts.push(part)
        }
      }
      finalUserContent = parts
    }
  } else if (Array.isArray(userContent)) {
    const textParts = userContent.filter((p) => p.type === 'text')
    if (textParts.length === 1) {
      finalUserContent = textParts[0].text || ''
    } else {
      finalUserContent = textParts
    }
  }

  apiMessages.push({ role: 'user', content: finalUserContent })

  return apiMessages
}

/**
 * useChat coordinates persistence, streaming, and special flows (media, deep research) for a
 * single chat session and returns state plus action helpers.
 */
export function useChat({ sessionId, config }: UseChatOptions): UseChatReturn {
  const [messages, setMessages] = useState<ChatMessageData[]>([])
  const [isLoading, _setIsLoading] = useState(false)
  const [loadingChatId, setLoadingChatId] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

  const abortControllerRef = useRef<AbortController | null>(null)
  const currentChatIdRef = useRef<string | null>(null)
  const deepResearchAbortRef = useRef(false)

  const setIsLoading = useCallback((loading: boolean) => {
    _setIsLoading(loading)
    if (!loading) {
      setLoadingChatId(null)
    }
  }, [])

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
   * findMaskPair searches a file list for matching image and mask pairs used by inpainting flows.
   */
  const findMaskPair = useCallback((attachments?: ChatAttachment[]) => {
    if (!attachments || attachments.length === 0) return null
    const normalizeName = (name: string) => name.replace(/\.[^.]+$/, '')

    for (const att of attachments) {
      if (!att.file) continue
      const lower = att.filename.toLowerCase()
      if (!lower.includes('-mask')) continue
      const base = normalizeName(lower.split('-mask')[0])
      const pair = attachments.find((a) => {
        if (a === att || !a.file) return false
        const fname = normalizeName(a.filename.toLowerCase())
        return fname === base || fname.startsWith(base)
      })
      if (pair && pair.file && att.file) {
        return { image: pair.file, mask: att.file }
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
    async (content: string, attachments?: ChatAttachment[]) => {
      const safeContent = String(content || '').trim()
      if (!safeContent) return

      const chatId = generateChatId()
      currentChatIdRef.current = chatId
      setLoadingChatId(chatId)
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
        for (const att of attachments) {
          if (att.type === 'image') {
            const b64 =
              att.contentB64 || (att.file ? await fileToDataUrl(att.file) : '')
            if (b64) {
              attachmentMarkdown.push(`![${att.filename}](${b64})`)
              contentParts.push({ type: 'image_url', image_url: { url: b64 } })
            }
          } else if (att.type === 'file') {
            const note = att.url
              ? `[File uploaded: ${att.filename} (url: ${att.url})]`
              : `[File uploaded: ${att.filename}]`
            attachmentMarkdown.push(note)
            // pushTextPart(`\n\n${note}`) // we don't necessarily need to push it again if it's already in the text from MessageInput
          }
        }
      }

      if (attachmentMarkdown.length > 0) {
        // Only append non-image attachments to finalContent for display.
        // Images are handled via the attachments field in ChatMessageData.
        const nonImageMarkdown = attachmentMarkdown.filter(
          (m) => !m.startsWith('!['),
        )
        if (nonImageMarkdown.length > 0) {
          // Check if any of these notes are already in finalContent to avoid duplication
          const missingNotes = nonImageMarkdown.filter(
            (note) => !finalContent.includes(note),
          )
          if (missingNotes.length > 0) {
            finalContent =
              `${finalContent}\n\n${missingNotes.join('\n\n')}`.trim()
          }
        }
      }

      const userMessage: ChatMessageData = {
        chatID: chatId,
        role: 'user',
        content: finalContent,
        timestamp: Date.now(),
        attachments: attachments,
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
      setLoadingChatId(chatId)

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

      // Reconstruct content parts if there are attachments
      let userContent: string | ContentPart[] = userMsg.content
      if (userMsg.attachments && userMsg.attachments.length > 0) {
        const parts: ContentPart[] = [{ type: 'text', text: userMsg.content }]
        for (const att of userMsg.attachments) {
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

      if (isDeepResearchModel()) {
        await runDeepResearch({
          chatId,
          prompt: userMsg.content,
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
          prompt: userMsg.content,
          config,
          setMessages,
          setIsLoading,
          setError,
          saveMessage,
          currentChatIdRef,
        })
        return
      }

      await streamAssistantReply({ chatId, payload: apiMessages })
    },
    [
      config,
      messages,
      isDeepResearchModel,
      runDeepResearch,
      runImageModelFlow,
      saveMessage,
      streamAssistantReply,
    ],
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
      setLoadingChatId(chatId)

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

      if (isDeepResearchModel()) {
        await runDeepResearch({
          chatId,
          prompt: trimmed,
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
          prompt: trimmed,
          config,
          setMessages,
          setIsLoading,
          setError,
          saveMessage,
          currentChatIdRef,
        })
        return
      }

      await streamAssistantReply({ chatId, payload: apiMessages })
    },
    [
      config,
      messages,
      isDeepResearchModel,
      runDeepResearch,
      runImageModelFlow,
      saveMessage,
      streamAssistantReply,
    ],
  )

  return {
    messages,
    isLoading,
    loadingChatId,
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
