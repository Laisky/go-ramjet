import {
  useCallback,
  type Dispatch,
  type MutableRefObject,
  type SetStateAction,
} from 'react'

import {
  sendStreamingChatRequest,
  type ChatMessage as ApiChatMessage,
  type ToolCallDelta,
} from '@/utils/api'
import { extractReferencesFromAnnotations } from '@/utils/chat-parser'

import {
  DefaultSessionConfig,
  type Annotation,
  type ChatMessageData,
  type SessionConfig,
} from '../types'
import { callMCPTool } from '../utils/mcp'

export interface ChatStreamingOptions {
  config: SessionConfig
  setMessages: Dispatch<SetStateAction<ChatMessageData[]>>
  setIsLoading: (value: boolean) => void
  setError: (value: string | null) => void
  saveMessage: (message: ChatMessageData) => Promise<void>
  abortControllerRef: MutableRefObject<AbortController | null>
  currentChatIdRef: MutableRefObject<string | null>
}

export interface StreamAssistantRequest {
  chatId: string
  payload: ApiChatMessage[]
}

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

const resolveServerForTool = (config: SessionConfig, toolName: string) => {
  if (!toolName) return null
  const servers = config.mcp_servers || []
  return (
    servers.find((server) => {
      if (!server.enabled) return false
      if (server.enabled_tool_names && server.enabled_tool_names.length > 0) {
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
  config: SessionConfig,
  appendToolEvent: (line: string) => void,
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

    const server = resolveServerForTool(config, toolName)
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

/**
 * useChatStreaming streams assistant replies (including MCP tool calls) into state.
 */
export function useChatStreaming({
  config,
  setMessages,
  setIsLoading,
  setError,
  saveMessage,
  abortControllerRef,
  currentChatIdRef,
}: ChatStreamingOptions) {
  const streamAssistantReply = useCallback(
    async ({ chatId, payload }: StreamAssistantRequest) => {
      setIsLoading(true)
      currentChatIdRef.current = chatId

      let fullContent = ''
      let fullReasoning = ''
      let fullAnnotations: Annotation[] = []
      let lastRequestId = ''
      let lastModel = ''
      const toolEventLog: string[] = []
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

      const runStream = async (messagesToSend: ApiChatMessage[]) => {
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
              messages: messagesToSend,
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
              onResponseInfo: (info) => {
                if (info.id) lastRequestId = info.id
                if (info.model) lastModel = info.model
                setMessages((prev) =>
                  prev.map((m) =>
                    m.chatID === chatId && m.role === 'assistant'
                      ? {
                          ...m,
                          requestid: info.id || m.requestid,
                          model: info.model || m.model,
                        }
                      : m,
                  ),
                )
              },
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
        let workingPayload = payload
        while (true) {
          const finishReason = await runStream(workingPayload)
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
            workingPayload = await buildToolContinuationMessages(
              config,
              appendToolEvent,
              workingPayload,
              toolCalls,
            )
            continue
          }
          break
        }

        const finalMessage: ChatMessageData = {
          chatID: chatId,
          role: 'assistant',
          content: fullContent,
          model: lastModel || config.selected_model,
          reasoningContent:
            [...toolEventLog, fullReasoning].filter(Boolean).join('\n') ||
            undefined,
          annotations: fullAnnotations,
          references: extractReferencesFromAnnotations(fullAnnotations),
          timestamp: Date.now(),
          requestid: lastRequestId,
        }

        if (lastRequestId) {
          try {
            const resp = await fetch(
              `/gptchat/oneapi/api/cost/request/${lastRequestId}`,
              {
                headers: {
                  Authorization: `Bearer ${config.api_token}`,
                },
              },
            )
            if (resp.ok) {
              const data = await resp.json()
              if (data.cost_usd) {
                finalMessage.costUsd = data.cost_usd
                setMessages((prev) =>
                  prev.map((m) =>
                    m.chatID === chatId && m.role === 'assistant'
                      ? { ...m, costUsd: data.cost_usd }
                      : m,
                  ),
                )
              }
            }
          } catch (err) {
            console.error('Failed to fetch cost:', err)
          }
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
    [
      abortControllerRef,
      config,
      currentChatIdRef,
      saveMessage,
      setError,
      setIsLoading,
      setMessages,
    ],
  )

  return { streamAssistantReply }
}
