/**
 * React hook for managing chat state and interactions.
 */
import { useCallback, useRef, useState } from 'react'

import {
  createDeepResearchTask,
  editImageWithMask,
  fetchDeepResearchStatus,
  sendStreamingChatRequest,
  type ChatMessage as ApiChatMessage,
  type ContentPart,
  type ToolCallDelta,
} from '@/utils/api'
import { extractReferencesFromAnnotations } from '@/utils/chat-parser'
import { kvDel, kvGet, kvSet } from '@/utils/storage'
import {
  ChatModelDeepResearch,
  ChatModelGPTO3Deepresearch,
  ChatModelGPTO4MiniDeepresearch,
  isImageModel,
} from '../models'
import {
  DefaultSessionConfig,
  type Annotation,
  type ChatMessageData,
  type SessionConfig,
  type SessionHistoryItem,
} from '../types'
import {
  generateChatId,
  getChatDataKey,
  getSessionHistoryKey,
} from '../utils/chat-storage'
import { callMCPTool } from '../utils/mcp'

export interface UseChatOptions {
  sessionId: number
  config: SessionConfig
}

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
  editAndRetry: (chatId: string, newContent: string) => Promise<void>
}

/**
 * normalizePositiveInteger coerces unknown values into a finite positive integer.
 */
function normalizePositiveInteger(value: unknown, fallback: number): number {
  if (typeof value === 'number' && Number.isFinite(value) && value > 0) {
    return value
  }
  if (typeof value === 'string') {
    const parsed = Number.parseInt(value.trim(), 10)
    if (Number.isFinite(parsed) && parsed > 0) {
      return parsed
    }
    return fallback
  }
  if (value !== null && value !== undefined) {
    const parsed = Number.parseInt(String(value), 10)
    if (Number.isFinite(parsed) && parsed > 0) {
      return parsed
    }
  }
  return fallback
}

/**
 * Hook for managing chat messages and interactions
 */
export function useChat({ sessionId, config }: UseChatOptions): UseChatReturn {
  const [messages, setMessages] = useState<ChatMessageData[]>([])
  const [isLoading, setIsLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const abortControllerRef = useRef<AbortController | null>(null)
  const currentChatIdRef = useRef<string | null>(null)
  const deepResearchAbortRef = useRef(false)

  /**
   * Load messages from storage
   */
  const loadMessages = useCallback(async () => {
    try {
      const key = getSessionHistoryKey(sessionId)
      const history = await kvGet<SessionHistoryItem[]>(key)

      if (!history || history.length === 0) {
        setMessages([])
        return
      }

      // Load full message data for each history item
      const loadedMessages: ChatMessageData[] = []

      for (const item of history) {
        // Try to load from individual storage first
        const userKey = getChatDataKey(item.chatID, 'user')
        const assistantKey = getChatDataKey(item.chatID, 'assistant')

        const userData = await kvGet<ChatMessageData>(userKey)
        const assistantData = await kvGet<ChatMessageData>(assistantKey)

        if (userData) {
          loadedMessages.push(userData)
        }
        if (assistantData) {
          loadedMessages.push(assistantData)
        }
      }

      // Sort by chatID (which contains timestamp)
      loadedMessages.sort((a, b) => a.chatID.localeCompare(b.chatID))

      setMessages(loadedMessages)
    } catch (err) {
      console.error('Failed to load messages:', err)
      setError('Failed to load messages')
    }
  }, [sessionId])

  const fileToDataUrl = useCallback(async (file: File): Promise<string> => {
    return new Promise<string>((resolve, reject) => {
      const reader = new FileReader()
      reader.onloadend = () => resolve(reader.result as string)
      reader.onerror = () => reject(new Error('Failed to read file'))
      reader.readAsDataURL(file)
    })
  }, [])

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

  const isDeepResearchModel = useCallback(() => {
    const model = config.selected_model
    return (
      model === ChatModelDeepResearch ||
      model === ChatModelGPTO3Deepresearch ||
      model === ChatModelGPTO4MiniDeepresearch
    )
  }, [config.selected_model])

  /**
   * Save a message to storage
   */
  const saveMessage = useCallback(
    async (message: ChatMessageData) => {
      // Save individual message data
      const key = getChatDataKey(
        message.chatID,
        message.role as 'user' | 'assistant',
      )
      await kvSet(key, message)

      // Update session history
      const historyKey = getSessionHistoryKey(sessionId)
      const history = (await kvGet<SessionHistoryItem[]>(historyKey)) || []

      // Check if this chatID already exists
      const existingIndex = history.findIndex(
        (h) => h.chatID === message.chatID && h.role === message.role,
      )

      const historyItem: SessionHistoryItem = {
        chatID: message.chatID,
        role: message.role as 'user' | 'assistant',
        content: message.content.substring(0, 100), // Preview only
        model: message.model,
        timestamp: message.timestamp,
      }

      if (existingIndex >= 0) {
        history[existingIndex] = historyItem
      } else {
        history.push(historyItem)
      }

      await kvSet(historyKey, history)
    },
    [sessionId],
  )

  /**
   * Send a message and get AI response
   */
  const sendMessage = useCallback(
    async (content: string, _attachments?: File[]) => {
      if (!content.trim()) return

      const chatId = generateChatId()
      currentChatIdRef.current = chatId
      setError(null)

      // Build attachment markdown + content parts for API
      const contentParts: ContentPart[] = []
      const attachmentMarkdown: string[] = []

      let finalContent = content.trim()

      const pushTextPart = (text: string) => {
        if (!text) return
        contentParts.push({ type: 'text', text })
      }

      pushTextPart(finalContent)

      // Handle file attachments if present
      if (typeof _attachments !== 'undefined' && _attachments.length > 0) {
        try {
          const { uploadFiles } = await import('@/utils/api')

          for (const file of _attachments) {
            if (file.type.startsWith('image/')) {
              const b64 = await new Promise<string>((resolve) => {
                const reader = new FileReader()
                reader.onloadend = () => resolve(reader.result as string)
                reader.readAsDataURL(file)
              })
              // Append markdown for local render and push image part for API
              attachmentMarkdown.push(`![${file.name}](${b64})`)
              contentParts.push({
                type: 'image_url',
                image_url: { url: b64 },
              })
            } else {
              // Non-image files, upload to get cache key reference
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

      // Build user-visible content that mirrors legacy markdown rendering
      if (attachmentMarkdown.length > 0) {
        finalContent =
          `${finalContent}\n\n${attachmentMarkdown.join('\n\n')}`.trim()
      }

      // Create user message
      const userMessage: ChatMessageData = {
        chatID: chatId,
        role: 'user',
        content: finalContent,
        timestamp: Date.now(),
      }

      // Add user message to state
      setMessages((prev) => [...prev, userMessage])

      // Save user message
      await saveMessage(userMessage)

      // If mask & image are provided, run inpainting flow
      const maskPair = findMaskPair(_attachments)
      if (maskPair) {
        setIsLoading(true)
        const assistantMessage: ChatMessageData = {
          chatID: chatId,
          role: 'assistant',
          content: 'Editing image...',
          model: config.selected_model,
          timestamp: Date.now(),
        }
        setMessages((prev) => [...prev, assistantMessage])

        try {
          const [imageDataUrl, maskDataUrl] = await Promise.all([
            fileToDataUrl(maskPair.image),
            fileToDataUrl(maskPair.mask),
          ])

          const resp = await editImageWithMask(
            'flux-fill-pro',
            {
              prompt: content.trim(),
              image: imageDataUrl,
              mask: maskDataUrl,
            },
            config.api_token,
            config.api_base !== 'https://api.openai.com'
              ? config.api_base
              : undefined,
          )

          const imgContent = resp.image_urls
            .map((url) => `![Image](${url})`)
            .join('\n\n')

          const finalAssist: ChatMessageData = {
            ...assistantMessage,
            content: imgContent,
          }

          setMessages((prev) =>
            prev.map((m) =>
              m.chatID === chatId && m.role === 'assistant' ? finalAssist : m,
            ),
          )

          await saveMessage(finalAssist)
        } catch (err) {
          const msg = err instanceof Error ? err.message : String(err)
          setError(msg)
          setMessages((prev) => prev.filter((m) => m.chatID !== chatId))
        } finally {
          setIsLoading(false)
          currentChatIdRef.current = null
        }
        return
      }

      // Deep research task flow
      if (isDeepResearchModel()) {
        setIsLoading(true)
        deepResearchAbortRef.current = false

        const assistantMessage: ChatMessageData = {
          chatID: chatId,
          role: 'assistant',
          content: 'Researching... ⏳',
          model: config.selected_model,
          timestamp: Date.now(),
        }
        setMessages((prev) => [...prev, assistantMessage])

        const apiBase =
          config.api_base !== 'https://api.openai.com'
            ? config.api_base
            : undefined

        const updateAssistant = (text: string) => {
          setMessages((prev) =>
            prev.map((m) =>
              m.chatID === chatId && m.role === 'assistant'
                ? { ...m, content: text }
                : m,
            ),
          )
        }

        try {
          const { task_id } = await createDeepResearchTask(
            content.trim(),
            config.api_token,
            apiBase,
          )

          let finalText = ''
          for (let i = 0; i < 60; i += 1) {
            if (deepResearchAbortRef.current) {
              throw new Error('Deep research aborted')
            }
            const status = await fetchDeepResearchStatus(
              task_id,
              config.api_token,
              apiBase,
            )
            const statusText = (status.status || '').toLowerCase()

            if (
              [
                'succeeded',
                'success',
                'completed',
                'done',
                'finished',
              ].includes(statusText)
            ) {
              finalText =
                status.result ||
                status.output ||
                status.content ||
                status.summary ||
                JSON.stringify(status)
              break
            }

            if (
              ['failed', 'error', 'canceled', 'cancelled'].includes(statusText)
            ) {
              throw new Error(`Deep research failed: ${status.status}`)
            }

            updateAssistant(`Researching... (${status.status || 'pending'})`)
            await new Promise((resolve) => setTimeout(resolve, 3000))
          }

          const finalAssistant: ChatMessageData = {
            ...assistantMessage,
            content:
              finalText ||
              'Research completed but no content was returned by the service.',
            timestamp: Date.now(),
          }

          setMessages((prev) =>
            prev.map((m) =>
              m.chatID === chatId && m.role === 'assistant'
                ? finalAssistant
                : m,
            ),
          )

          await saveMessage(finalAssistant)
        } catch (err) {
          const msg = err instanceof Error ? err.message : String(err)
          setError(msg)
          setMessages((prev) => prev.filter((m) => m.chatID !== chatId))
        } finally {
          setIsLoading(false)
          currentChatIdRef.current = null
          deepResearchAbortRef.current = false
        }
        return
      }

      // Check if this is an image model
      if (isImageModel(config.selected_model)) {
        setIsLoading(true)
        // Add minimal assistant message placeholder
        const assistantMessage: ChatMessageData = {
          chatID: chatId,
          role: 'assistant',
          content: 'Generating image...',
          model: config.selected_model,
          timestamp: Date.now(),
        }
        setMessages((prev) => [...prev, assistantMessage])

        try {
          const { generateImage } = await import('@/utils/api')
          const resp = await generateImage(
            {
              model: config.selected_model,
              prompt: content,
              n: config.chat_switch.draw_n_images,
              size: '1024x1024', // Default
            },
            config.api_token,
            config.api_base !== 'https://api.openai.com'
              ? config.api_base
              : undefined,
          )

          const newContent = resp.data
            .map((d) =>
              d.url
                ? `![Image](${d.url})`
                : `![Image](data:image/png;base64,${d.b64_json})`,
            )
            .join('\n\n')

          setMessages((prev) =>
            prev.map((m) =>
              m.chatID === chatId && m.role === 'assistant'
                ? { ...m, content: newContent }
                : m,
            ),
          )

          // Save final message
          await saveMessage({ ...assistantMessage, content: newContent })
        } catch (err: unknown) {
          const errMsg = err instanceof Error ? err.message : String(err)
          setError(errMsg)
          setMessages((prev) => prev.filter((m) => m.chatID !== chatId)) // Remove failed message
        } finally {
          setIsLoading(false)
          currentChatIdRef.current = null
        }
        return
      }

      // Prepare assistant message placeholder
      const assistantMessage: ChatMessageData = {
        chatID: chatId,
        role: 'assistant',
        content: '',
        model: config.selected_model,
        timestamp: Date.now(),
      }

      setMessages((prev) => [...prev, assistantMessage])
      setIsLoading(true)

      // Build messages for API
      const apiMessages: ApiChatMessage[] = []

      // Add system prompt
      if (config.system_prompt) {
        apiMessages.push({
          role: 'system',
          content: config.system_prompt,
        })
      }

      // Add context messages (last n_contexts)
      const contextMessages = messages.slice(-config.n_contexts * 2)
      for (const msg of contextMessages) {
        apiMessages.push({
          role: msg.role,
          content: msg.content,
        })
      }

      // Add current user message (array content when attachments exist)
      apiMessages.push({
        role: 'user',
        content: contentParts.length > 1 ? contentParts : finalContent,
      })

      // Stream response
      let fullContent = ''
      let fullReasoning = ''
      const toolEventLog: string[] = []
      let fullAnnotations: Annotation[] = []
      const toolCallAccumulator = new Map<string, ToolCallDelta>()
      const finishReasonRef = { current: null as string | null }

      const updateReasoning = (thinkingChunk?: string) => {
        const combined = [
          ...toolEventLog,
          thinkingChunk ? thinkingChunk : fullReasoning,
        ]
          .filter(Boolean)
          .join('\n')

        setMessages((prev) =>
          prev.map((m) =>
            m.chatID === chatId && m.role === 'assistant'
              ? { ...m, reasoningContent: combined }
              : m,
          ),
        )
      }

      const appendToolEvent = (line: string) => {
        if (!line) return
        toolEventLog.push(line)
        updateReasoning()
      }

      const accumulateToolCalls = (deltas: ToolCallDelta[]) => {
        deltas.forEach((delta) => {
          const id = delta.id || `tool_${toolCallAccumulator.size + 1}`
          const existing = toolCallAccumulator.get(id) || {
            id,
            type: delta.type || 'function',
            function: { name: '', arguments: '' },
          }

          if (!existing.function) {
            existing.function = { name: '', arguments: '' }
          }

          if (delta.function?.name) {
            existing.function.name = delta.function.name
          }
          if (delta.function?.arguments) {
            existing.function.arguments =
              (existing.function.arguments || '') + delta.function.arguments
          }

          existing.type = delta.type || existing.type
          toolCallAccumulator.set(id, existing)

          if (existing.function?.name) {
            appendToolEvent(`Upstream tool_call: ${existing.function.name}`)
          }
          if (delta.function?.arguments) {
            appendToolEvent(`args: ${delta.function.arguments}`)
          }
        })
      }

      const resolveServerForTool = (toolName: string) => {
        if (!toolName) return null
        const servers = config.mcp_servers || []
        return (
          servers.find((server) => {
            if (!server.enabled) return false
            if (
              server.enabled_tool_names &&
              server.enabled_tool_names.length > 0
            ) {
              if (!server.enabled_tool_names.includes(toolName)) {
                return false
              }
            }
            if (!server.tools || server.tools.length === 0) {
              return true
            }
            return server.tools.some((tool: any) => tool?.name === toolName)
          }) || null
        )
      }

      const buildToolContinuationMessages = async (
        baseMessages: ApiChatMessage[],
        toolCalls: ToolCallDelta[],
      ): Promise<ApiChatMessage[]> => {
        const assistantToolCalls = toolCalls.map((call) => ({
          id: call.id || crypto.randomUUID(),
          type: 'function' as const,
          function: {
            name: call.function?.name || '',
            arguments: call.function?.arguments || '',
          },
        }))

        const toolMessages: ApiChatMessage[] = []

        for (const call of assistantToolCalls) {
          const toolName = call.function.name
          if (!toolName) {
            toolMessages.push({
              role: 'tool',
              content: 'Tool name missing in call.',
              tool_call_id: call.id,
            })
            continue
          }

          const server = resolveServerForTool(toolName)
          if (!server) {
            toolMessages.push({
              role: 'tool',
              content: `Tool ${toolName} is not enabled in this session.`,
              tool_call_id: call.id,
              name: toolName,
            })
            continue
          }

          try {
            const output = await callMCPTool(
              server,
              toolName,
              call.function.arguments,
            )
            appendToolEvent(`tool ok: ${toolName}`)
            toolMessages.push({
              role: 'tool',
              content: output,
              tool_call_id: call.id,
              name: toolName,
            })
          } catch (err) {
            const message = err instanceof Error ? err.message : String(err)
            appendToolEvent(`tool error: ${toolName}: ${message}`)
            toolMessages.push({
              role: 'tool',
              content: `Tool ${toolName} failed: ${message}`,
              tool_call_id: call.id,
              name: toolName,
            })
          }
        }

        return baseMessages.concat(
          {
            role: 'assistant',
            content: '',
            tool_calls: assistantToolCalls,
          },
          ...toolMessages,
        )
      }

      const runStream = async (payload: ApiChatMessage[]) => {
        finishReasonRef.current = null
        toolCallAccumulator.clear()

        const safeMaxTokens = normalizePositiveInteger(
          config.max_tokens,
          DefaultSessionConfig.max_tokens,
        )

        await new Promise<void>((resolve, reject) => {
          abortControllerRef.current = sendStreamingChatRequest(
            {
              model: config.selected_model,
              messages: payload,
              max_tokens: safeMaxTokens,
              temperature: config.temperature,
              presence_penalty: config.presence_penalty,
              frequency_penalty: config.frequency_penalty,
              stream: true,
              enable_mcp: config.chat_switch.enable_mcp,
              mcp_servers: config.mcp_servers
                ?.filter((s: { enabled: boolean }) => s.enabled)
                .map((s: { name: string; url: string; api_key?: string }) => ({
                  name: s.name,
                  url: s.url,
                  api_key: s.api_key,
                })),
              laisky_extra: {
                chat_switch: {
                  disable_https_crawler:
                    config.chat_switch.disable_https_crawler,
                  all_in_one: config.chat_switch.all_in_one,
                },
              },
            },
            config.api_token,
            {
              onContent: (chunk) => {
                fullContent += chunk
                setMessages((prev) =>
                  prev.map((m) =>
                    m.chatID === chatId && m.role === 'assistant'
                      ? { ...m, content: fullContent }
                      : m,
                  ),
                )
              },
              onReasoning: (chunk) => {
                fullReasoning += chunk
                updateReasoning(fullReasoning)
              },
              onAnnotations: (annotations) => {
                fullAnnotations = annotations
                const references = extractReferencesFromAnnotations(annotations)
                setMessages((prev) =>
                  prev.map((m) =>
                    m.chatID === chatId && m.role === 'assistant'
                      ? { ...m, annotations, references }
                      : m,
                  ),
                )
              },
              onToolCallDelta: accumulateToolCalls,
              onFinish: (reason) => {
                finishReasonRef.current = reason ?? null
              },
              onDone: resolve,
              onError: (err) => {
                reject(err)
              },
            },
            config.api_base !== 'https://api.openai.com'
              ? config.api_base
              : undefined,
          )
          currentChatIdRef.current = chatId
        })

        return finishReasonRef.current
      }

      try {
        let payload = apiMessages
        while (true) {
          const finishReason = await runStream(payload)
          if (finishReason === 'tool_calls') {
            if (!config.chat_switch.enable_mcp) {
              throw new Error(
                'Model requested MCP tools but they are disabled for this session.',
              )
            }
            if (toolCallAccumulator.size === 0) {
              throw new Error(
                'Model requested tool calls but none were parsed.',
              )
            }
            const toolCalls = Array.from(toolCallAccumulator.values())
            payload = await buildToolContinuationMessages(payload, toolCalls)
            continue
          }
          break
        }

        const finalMessage: ChatMessageData = {
          chatID: chatId,
          role: 'assistant',
          content: fullContent,
          model: config.selected_model,
          reasoningContent:
            [...toolEventLog, fullReasoning].filter(Boolean).join('\n') ||
            undefined,
          annotations: fullAnnotations,
          references: extractReferencesFromAnnotations(fullAnnotations),
          timestamp: Date.now(),
        }
        await saveMessage(finalMessage)
      } catch (error) {
        if (error instanceof Error && error.name === 'AbortError') {
          // User aborted generation; no error UI.
        } else {
          const message = error instanceof Error ? error.message : String(error)
          setError(message)
        }
      } finally {
        setIsLoading(false)
        abortControllerRef.current = null
        currentChatIdRef.current = null
      }
    },
    [config, messages, saveMessage],
  )

  /**
   * Stop the current generation
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
   * Clear all messages in the current session
   */
  const clearMessages = useCallback(async () => {
    // Get all message IDs from history
    const historyKey = getSessionHistoryKey(sessionId)
    const history = await kvGet<SessionHistoryItem[]>(historyKey)

    if (history) {
      // Delete individual message data
      const chatIds = new Set(history.map((h) => h.chatID))
      for (const chatId of chatIds) {
        await kvDel(getChatDataKey(chatId, 'user'))
        await kvDel(getChatDataKey(chatId, 'assistant'))
      }
    }

    // Clear history
    await kvSet(historyKey, [])

    setMessages([])
  }, [sessionId])

  /**
   * Delete a specific message pair (user + assistant)
   */
  const deleteMessage = useCallback(
    async (chatId: string) => {
      // Delete from storage
      await kvDel(getChatDataKey(chatId, 'user'))
      await kvDel(getChatDataKey(chatId, 'assistant'))

      // Update history
      const historyKey = getSessionHistoryKey(sessionId)
      const history = await kvGet<SessionHistoryItem[]>(historyKey)
      if (history) {
        const newHistory = history.filter((h) => h.chatID !== chatId)
        await kvSet(historyKey, newHistory)
      }

      // Update state
      setMessages((prev) => prev.filter((m) => m.chatID !== chatId))
    },
    [sessionId],
  )

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
      const userContent = userMsg.content

      // Remove old assistant from storage; we will stream a fresh one in place.
      await kvDel(getChatDataKey(chatId, 'assistant'))

      // Update UI state: keep history, but reset/insert assistant slot.
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

      // Build context up to this turn (respect n_contexts window)
      const priorMessages = messages.slice(0, userIndex)
      const contextMessages = priorMessages.slice(-config.n_contexts * 2)

      const apiMessages: ApiChatMessage[] = []
      if (config.system_prompt) {
        apiMessages.push({ role: 'system', content: config.system_prompt })
      }
      for (const msg of contextMessages) {
        apiMessages.push({ role: msg.role, content: msg.content })
      }
      apiMessages.push({ role: 'user', content: userContent })

      // Stream assistant reply into existing slot
      setIsLoading(true)
      currentChatIdRef.current = chatId

      let fullContent = ''
      let fullReasoning = ''
      const toolEventLog: string[] = []
      let fullAnnotations: Annotation[] = []
      const toolCallAccumulator = new Map<string, ToolCallDelta>()
      const finishReasonRef = { current: null as string | null }

      const updateReasoning = (thinkingChunk?: string) => {
        const combined = [
          ...toolEventLog,
          thinkingChunk ? thinkingChunk : fullReasoning,
        ]
          .filter(Boolean)
          .join('\n')

        setMessages((prev) =>
          prev.map((m) =>
            m.chatID === chatId && m.role === 'assistant'
              ? { ...m, reasoningContent: combined }
              : m,
          ),
        )
      }

      const appendToolEvent = (line: string) => {
        if (!line) return
        toolEventLog.push(line)
        updateReasoning()
      }

      const accumulateToolCalls = (deltas: ToolCallDelta[]) => {
        deltas.forEach((delta) => {
          const id = delta.id || `tool_${toolCallAccumulator.size + 1}`
          const existing = toolCallAccumulator.get(id) || {
            id,
            type: delta.type || 'function',
            function: { name: '', arguments: '' },
          }

          if (!existing.function) {
            existing.function = { name: '', arguments: '' }
          }

          if (delta.function?.name) {
            existing.function.name = delta.function.name
          }
          if (delta.function?.arguments) {
            existing.function.arguments =
              (existing.function.arguments || '') + delta.function.arguments
          }

          existing.type = delta.type || existing.type
          toolCallAccumulator.set(id, existing)

          if (existing.function?.name) {
            appendToolEvent(`Upstream tool_call: ${existing.function.name}`)
          }
          if (delta.function?.arguments) {
            appendToolEvent(`args: ${delta.function.arguments}`)
          }
        })
      }

      const resolveServerForTool = (toolName: string) => {
        if (!toolName) return null
        const servers = config.mcp_servers || []
        return (
          servers.find((server) => {
            if (!server.enabled) return false
            if (
              server.enabled_tool_names &&
              server.enabled_tool_names.length > 0
            ) {
              if (!server.enabled_tool_names.includes(toolName)) {
                return false
              }
            }
            if (!server.tools || server.tools.length === 0) {
              return true
            }
            return server.tools.some((tool: any) => tool?.name === toolName)
          }) || null
        )
      }

      const buildToolContinuationMessages = async (
        baseMessages: ApiChatMessage[],
        toolCalls: ToolCallDelta[],
      ): Promise<ApiChatMessage[]> => {
        const assistantToolCalls = toolCalls.map((call) => ({
          id: call.id || crypto.randomUUID(),
          type: 'function' as const,
          function: {
            name: call.function?.name || '',
            arguments: call.function?.arguments || '',
          },
        }))

        const toolMessages: ApiChatMessage[] = []

        for (const call of assistantToolCalls) {
          const toolName = call.function.name
          if (!toolName) {
            toolMessages.push({
              role: 'tool',
              content: 'Tool name missing in call.',
              tool_call_id: call.id,
            })
            continue
          }

          const server = resolveServerForTool(toolName)
          if (!server) {
            toolMessages.push({
              role: 'tool',
              content: `Tool ${toolName} is not enabled in this session.`,
              tool_call_id: call.id,
              name: toolName,
            })
            continue
          }

          try {
            const output = await callMCPTool(
              server,
              toolName,
              call.function.arguments,
            )
            appendToolEvent(`tool ok: ${toolName}`)
            toolMessages.push({
              role: 'tool',
              content: output,
              tool_call_id: call.id,
              name: toolName,
            })
          } catch (err) {
            const message = err instanceof Error ? err.message : String(err)
            appendToolEvent(`tool error: ${toolName}: ${message}`)
            toolMessages.push({
              role: 'tool',
              content: `Tool ${toolName} failed: ${message}`,
              tool_call_id: call.id,
              name: toolName,
            })
          }
        }

        return baseMessages.concat(
          {
            role: 'assistant',
            content: '',
            tool_calls: assistantToolCalls,
          },
          ...toolMessages,
        )
      }

      const runStream = async (payload: ApiChatMessage[]) => {
        finishReasonRef.current = null
        toolCallAccumulator.clear()

        const safeMaxTokens = normalizePositiveInteger(
          config.max_tokens,
          DefaultSessionConfig.max_tokens,
        )

        await new Promise<void>((resolve, reject) => {
          abortControllerRef.current = sendStreamingChatRequest(
            {
              model: config.selected_model,
              messages: payload,
              max_tokens: safeMaxTokens,
              temperature: config.temperature,
              presence_penalty: config.presence_penalty,
              frequency_penalty: config.frequency_penalty,
              stream: true,
              enable_mcp: config.chat_switch.enable_mcp,
              mcp_servers: config.mcp_servers
                ?.filter((s: { enabled: boolean }) => s.enabled)
                .map((s: { name: string; url: string; api_key?: string }) => ({
                  name: s.name,
                  url: s.url,
                  api_key: s.api_key,
                })),
              laisky_extra: {
                chat_switch: {
                  disable_https_crawler:
                    config.chat_switch.disable_https_crawler,
                  all_in_one: config.chat_switch.all_in_one,
                },
              },
            },
            config.api_token,
            {
              onContent: (chunk) => {
                fullContent += chunk
                setMessages((prev) =>
                  prev.map((m) =>
                    m.chatID === chatId && m.role === 'assistant'
                      ? { ...m, content: fullContent }
                      : m,
                  ),
                )
              },
              onReasoning: (chunk) => {
                fullReasoning += chunk
                updateReasoning(fullReasoning)
              },
              onAnnotations: (annotations) => {
                fullAnnotations = annotations
                const references = extractReferencesFromAnnotations(annotations)
                setMessages((prev) =>
                  prev.map((m) =>
                    m.chatID === chatId && m.role === 'assistant'
                      ? { ...m, annotations, references }
                      : m,
                  ),
                )
              },
              onToolCallDelta: accumulateToolCalls,
              onFinish: (reason) => {
                finishReasonRef.current = reason ?? null
              },
              onDone: resolve,
              onError: (err) => {
                reject(err)
              },
            },
            config.api_base !== 'https://api.openai.com'
              ? config.api_base
              : undefined,
          )
          currentChatIdRef.current = chatId
        })

        return finishReasonRef.current
      }

      try {
        let payload = apiMessages
        while (true) {
          const finishReason = await runStream(payload)
          if (finishReason === 'tool_calls') {
            if (!config.chat_switch.enable_mcp) {
              throw new Error(
                'Model requested MCP tools but they are disabled for this session.',
              )
            }
            if (toolCallAccumulator.size === 0) {
              throw new Error(
                'Model requested tool calls but none were parsed.',
              )
            }
            const toolCalls = Array.from(toolCallAccumulator.values())
            payload = await buildToolContinuationMessages(payload, toolCalls)
            continue
          }
          break
        }

        const finalMessage: ChatMessageData = {
          chatID: chatId,
          role: 'assistant',
          content: fullContent,
          model: config.selected_model,
          reasoningContent:
            [...toolEventLog, fullReasoning].filter(Boolean).join('\n') ||
            undefined,
          annotations: fullAnnotations,
          references: extractReferencesFromAnnotations(fullAnnotations),
          timestamp: Date.now(),
        }
        await saveMessage(finalMessage)
      } catch (error) {
        if (error instanceof Error && error.name === 'AbortError') {
          // User aborted generation; no error UI.
        } else {
          const message = error instanceof Error ? error.message : String(error)
          setError(message)
        }
      } finally {
        setIsLoading(false)
        abortControllerRef.current = null
        currentChatIdRef.current = null
      }
    },
    [config, messages, saveMessage],
  )

  /**
   * Edit a message and retry in place without dropping later turns.
   * 1) Update the target user message content.
   * 2) Remove only the paired assistant response.
   * 3) Stream a new assistant reply at the same position.
   * Later messages remain visible so the conversation is not cleared.
   */
  const editAndRetry = useCallback(
    async (chatId: string, newContent: string) => {
      const trimmed = newContent.trim()
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
        timestamp: Date.now(),
      }

      // Persist updated user message
      await saveMessage(updatedUser)

      // Remove old assistant from storage (will be replaced by streaming one)
      await kvDel(getChatDataKey(chatId, 'assistant'))

      // Prepare state updates: keep all messages, update user, reset/insert assistant
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

      // Build payload using context before the edited turn (respect n_contexts window)
      const priorMessages = messages.slice(0, userIndex)
      const contextMessages = priorMessages.slice(-config.n_contexts * 2)

      const apiMessages: ApiChatMessage[] = []
      if (config.system_prompt) {
        apiMessages.push({ role: 'system', content: config.system_prompt })
      }
      for (const msg of contextMessages) {
        apiMessages.push({ role: msg.role, content: msg.content })
      }
      apiMessages.push({ role: 'user', content: trimmed })

      // Stream the assistant reply into the existing assistant slot
      setIsLoading(true)

      let fullContent = ''
      let fullReasoning = ''
      const toolEventLog: string[] = []
      let fullAnnotations: Annotation[] = []
      const toolCallAccumulator = new Map<string, ToolCallDelta>()
      const finishReasonRef = { current: null as string | null }

      const updateReasoning = (thinkingChunk?: string) => {
        const combined = [
          ...toolEventLog,
          thinkingChunk ? thinkingChunk : fullReasoning,
        ]
          .filter(Boolean)
          .join('\n')

        setMessages((prev) =>
          prev.map((m) =>
            m.chatID === chatId && m.role === 'assistant'
              ? { ...m, reasoningContent: combined }
              : m,
          ),
        )
      }

      const appendToolEvent = (line: string) => {
        if (!line) return
        toolEventLog.push(line)
        updateReasoning()
      }

      const accumulateToolCalls = (deltas: ToolCallDelta[]) => {
        deltas.forEach((delta) => {
          const id = delta.id || `tool_${toolCallAccumulator.size + 1}`
          const existing = toolCallAccumulator.get(id) || {
            id,
            type: delta.type || 'function',
            function: { name: '', arguments: '' },
          }

          if (!existing.function) {
            existing.function = { name: '', arguments: '' }
          }

          if (delta.function?.name) {
            existing.function.name = delta.function.name
          }
          if (delta.function?.arguments) {
            existing.function.arguments =
              (existing.function.arguments || '') + delta.function.arguments
          }

          existing.type = delta.type || existing.type
          toolCallAccumulator.set(id, existing)

          if (existing.function?.name) {
            appendToolEvent(`Upstream tool_call: ${existing.function.name}`)
          }
          if (delta.function?.arguments) {
            appendToolEvent(`args: ${delta.function.arguments}`)
          }
        })
      }

      const resolveServerForTool = (toolName: string) => {
        if (!toolName) return null
        const servers = config.mcp_servers || []
        return (
          servers.find((server) => {
            if (!server.enabled) return false
            if (
              server.enabled_tool_names &&
              server.enabled_tool_names.length > 0
            ) {
              if (!server.enabled_tool_names.includes(toolName)) {
                return false
              }
            }
            if (!server.tools || server.tools.length === 0) {
              return true
            }
            return server.tools.some((tool: any) => tool?.name === toolName)
          }) || null
        )
      }

      const buildToolContinuationMessages = async (
        baseMessages: ApiChatMessage[],
        toolCalls: ToolCallDelta[],
      ): Promise<ApiChatMessage[]> => {
        const assistantToolCalls = toolCalls.map((call) => ({
          id: call.id || crypto.randomUUID(),
          type: 'function' as const,
          function: {
            name: call.function?.name || '',
            arguments: call.function?.arguments || '',
          },
        }))

        const toolMessages: ApiChatMessage[] = []

        for (const call of assistantToolCalls) {
          const toolName = call.function.name
          if (!toolName) {
            toolMessages.push({
              role: 'tool',
              content: 'Tool name missing in call.',
              tool_call_id: call.id,
            })
            continue
          }

          const server = resolveServerForTool(toolName)
          if (!server) {
            toolMessages.push({
              role: 'tool',
              content: `Tool ${toolName} is not enabled in this session.`,
              tool_call_id: call.id,
              name: toolName,
            })
            continue
          }

          try {
            const output = await callMCPTool(
              server,
              toolName,
              call.function.arguments,
            )
            appendToolEvent(`tool ok: ${toolName}`)
            toolMessages.push({
              role: 'tool',
              content: output,
              tool_call_id: call.id,
              name: toolName,
            })
          } catch (err) {
            const message = err instanceof Error ? err.message : String(err)
            appendToolEvent(`tool error: ${toolName}: ${message}`)
            toolMessages.push({
              role: 'tool',
              content: `Tool ${toolName} failed: ${message}`,
              tool_call_id: call.id,
              name: toolName,
            })
          }
        }

        return baseMessages.concat(
          {
            role: 'assistant',
            content: '',
            tool_calls: assistantToolCalls,
          },
          ...toolMessages,
        )
      }

      const runStream = async (payload: ApiChatMessage[]) => {
        finishReasonRef.current = null
        toolCallAccumulator.clear()

        const safeMaxTokens = normalizePositiveInteger(
          config.max_tokens,
          DefaultSessionConfig.max_tokens,
        )

        await new Promise<void>((resolve, reject) => {
          abortControllerRef.current = sendStreamingChatRequest(
            {
              model: config.selected_model,
              messages: payload,
              max_tokens: safeMaxTokens,
              temperature: config.temperature,
              presence_penalty: config.presence_penalty,
              frequency_penalty: config.frequency_penalty,
              stream: true,
              enable_mcp: config.chat_switch.enable_mcp,
              mcp_servers: config.mcp_servers
                ?.filter((s: { enabled: boolean }) => s.enabled)
                .map((s: { name: string; url: string; api_key?: string }) => ({
                  name: s.name,
                  url: s.url,
                  api_key: s.api_key,
                })),
              laisky_extra: {
                chat_switch: {
                  disable_https_crawler:
                    config.chat_switch.disable_https_crawler,
                  all_in_one: config.chat_switch.all_in_one,
                },
              },
            },
            config.api_token,
            {
              onContent: (chunk) => {
                fullContent += chunk
                setMessages((prev) =>
                  prev.map((m) =>
                    m.chatID === chatId && m.role === 'assistant'
                      ? { ...m, content: fullContent }
                      : m,
                  ),
                )
              },
              onReasoning: (chunk) => {
                fullReasoning += chunk
                updateReasoning(fullReasoning)
              },
              onAnnotations: (annotations) => {
                fullAnnotations = annotations
                const references = extractReferencesFromAnnotations(annotations)
                setMessages((prev) =>
                  prev.map((m) =>
                    m.chatID === chatId && m.role === 'assistant'
                      ? { ...m, annotations, references }
                      : m,
                  ),
                )
              },
              onToolCallDelta: accumulateToolCalls,
              onFinish: (reason) => {
                finishReasonRef.current = reason ?? null
              },
              onDone: resolve,
              onError: (err) => {
                reject(err)
              },
            },
            config.api_base !== 'https://api.openai.com'
              ? config.api_base
              : undefined,
          )
          currentChatIdRef.current = chatId
        })

        return finishReasonRef.current
      }

      try {
        let payload = apiMessages
        while (true) {
          const finishReason = await runStream(payload)
          if (finishReason === 'tool_calls') {
            if (!config.chat_switch.enable_mcp) {
              throw new Error(
                'Model requested MCP tools but they are disabled for this session.',
              )
            }
            if (toolCallAccumulator.size === 0) {
              throw new Error(
                'Model requested tool calls but none were parsed.',
              )
            }
            const toolCalls = Array.from(toolCallAccumulator.values())
            payload = await buildToolContinuationMessages(payload, toolCalls)
            continue
          }
          break
        }

        const finalMessage: ChatMessageData = {
          chatID: chatId,
          role: 'assistant',
          content: fullContent,
          model: config.selected_model,
          reasoningContent:
            [...toolEventLog, fullReasoning].filter(Boolean).join('\n') ||
            undefined,
          annotations: fullAnnotations,
          references: extractReferencesFromAnnotations(fullAnnotations),
          timestamp: Date.now(),
        }
        await saveMessage(finalMessage)
      } catch (error) {
        if (error instanceof Error && error.name === 'AbortError') {
          // User aborted generation; no error UI.
        } else {
          const message = error instanceof Error ? error.message : String(error)
          setError(message)
        }
      } finally {
        setIsLoading(false)
        abortControllerRef.current = null
        currentChatIdRef.current = null
      }
    },
    [config, messages, saveMessage],
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
