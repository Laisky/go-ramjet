import {
  useCallback,
  type Dispatch,
  type MutableRefObject,
  type SetStateAction,
} from 'react'

import {
  getApiBase,
  sendStreamingChatRequest,
  type ChatMessage as ApiChatMessage,
  type ChatTool,
  type ToolCallDelta,
} from '@/utils/api'
import { extractReferencesFromAnnotations } from '@/utils/chat-parser'

import {
  DefaultSessionConfig,
  type Annotation,
  type ChatMessageData,
  type McpServerConfig,
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

/**
 * extractToolsFromMCPServers extracts tools from enabled MCP servers
 * and converts them to OpenAI-compatible ChatTool format.
 */
function extractToolsFromMCPServers(servers?: McpServerConfig[]): ChatTool[] {
  if (!servers || servers.length === 0) {
    return []
  }

  const tools: ChatTool[] = []
  for (const server of servers) {
    if (!server.enabled) continue
    if (!server.tools || server.tools.length === 0) continue

    const enabledSet = new Set(server.enabled_tool_names || [])

    for (const tool of server.tools) {
      if (!tool.name) continue

      // Filter by enabled_tool_names if specified
      if (enabledSet.size > 0 && !enabledSet.has(tool.name)) {
        continue
      }

      tools.push({
        type: 'function',
        function: {
          name: tool.name,
          description: tool.description,
          parameters: tool.input_schema || {},
        },
      })
    }
  }

  return tools
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
      let turnAnnotations: Annotation[] = [] // Annotations for the current stream run
      let lastRequestId = ''
      let lastModel = ''
      const toolEventLog: string[] = []
      const toolCallAccumulator = new Map<string, ToolCallDelta>()
      const finishReasonRef = { current: null as string | null }

      let lastUpdateTime = 0
      const THROTTLE_MS = 80 // Update UI frequently for streaming effect but avoid overloading

      const updateUIMessages = (force = false) => {
        const now = Date.now()
        if (!force && now - lastUpdateTime < THROTTLE_MS) return
        lastUpdateTime = now

        const combinedReasoning = [...toolEventLog, fullReasoning]
          .filter(Boolean)
          .join('\n')
        const combinedAnnotations = [...fullAnnotations, ...turnAnnotations]
        const references = extractReferencesFromAnnotations(combinedAnnotations)

        setMessages((prev) => {
          // Find the index of the message we want to update.
          // In most cases it's the last message, but strictly matching by chatId is safer.
          const idx = prev.findLastIndex(
            (m: ChatMessageData) =>
              m.chatID === chatId && m.role === 'assistant',
          )

          if (idx === -1) {
            console.warn(
              `[updateUIMessages] message ${chatId} not found in state`,
            )
            return prev
          }

          const existing = prev[idx]
          const updated: ChatMessageData = {
            ...existing,
            content: fullContent,
            reasoningContent: combinedReasoning || undefined,
            annotations:
              combinedAnnotations.length > 0
                ? combinedAnnotations
                : existing.annotations,
            references:
              references.length > 0 ? references : existing.references,
            requestid: lastRequestId || existing.requestid,
            model: lastModel || existing.model,
          }

          // Shallow comparison to avoid redundant state updates
          if (
            updated.content === existing.content &&
            updated.reasoningContent === existing.reasoningContent &&
            updated.requestid === existing.requestid &&
            updated.model === existing.model &&
            (updated.annotations?.length || 0) ===
              (existing.annotations?.length || 0)
          ) {
            return prev
          }

          const next = [...prev]
          next[idx] = updated
          return next
        })
      }

      const appendToolEvent = (line: string) => {
        if (!line) return
        toolEventLog.push(line)
        updateUIMessages(true) // Force update on tool events as they are significant
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
        turnAnnotations = [] // Reset for each stream run

        const safeMaxTokens = normalizePositiveInteger(
          config.max_tokens,
          DefaultSessionConfig.max_tokens,
        )

        // Extract tools from enabled MCP servers when MCP is enabled
        const mcpTools = config.chat_switch.enable_mcp
          ? extractToolsFromMCPServers(config.mcp_servers)
          : []

        // Build enabled servers with full tool definitions for backend routing
        const enabledServers = config.chat_switch.enable_mcp
          ? config.mcp_servers
              ?.filter((s) => s.enabled)
              .map((s) => ({
                id: s.id,
                name: s.name,
                url: s.url,
                api_key: s.api_key,
                enabled: s.enabled,
                tools: s.tools,
                enabled_tool_names: s.enabled_tool_names,
              }))
          : []

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
              tools: mcpTools.length > 0 ? mcpTools : undefined,
              tool_choice: mcpTools.length > 0 ? 'auto' : undefined,
              mcp_servers: enabledServers,
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
                updateUIMessages()
              },
              onReasoning: (chunk) => {
                fullReasoning += chunk
                updateUIMessages()
              },
              onAnnotations: (annotations) => {
                turnAnnotations = annotations
                updateUIMessages()
              },
              onToolCallDelta: accumulateToolCalls,
              onResponseInfo: (info) => {
                if (info.id) lastRequestId = info.id
                if (info.model) lastModel = info.model
                updateUIMessages()
              },
              onFinish: (reason) => {
                finishReasonRef.current = reason ?? null
              },
              onDone: () => {
                updateUIMessages(true)
                resolve()
              },
              onError: (err) => {
                updateUIMessages(true)
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
            fullAnnotations = [...fullAnnotations, ...turnAnnotations]
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

        fullAnnotations = [...fullAnnotations, ...turnAnnotations]
        // Final UI update to ensure state is synchronized with latest closure variables
        updateUIMessages(true)

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
              `${getApiBase()}/oneapi/api/cost/request/${lastRequestId}`,
              {
                headers: {
                  Authorization: `Bearer ${config.api_token}`,
                },
              },
            )
            if (resp.ok) {
              const data = await resp.json()
              if (data.cost_usd !== undefined && data.cost_usd !== null) {
                // Ensure costUsd is stored as a number for type safety
                const costValue = Number(data.cost_usd)
                if (!Number.isNaN(costValue)) {
                  finalMessage.costUsd = costValue
                  setMessages((prev) =>
                    prev.map((m) =>
                      m.chatID === chatId && m.role === 'assistant'
                        ? { ...m, costUsd: costValue }
                        : m,
                    ),
                  )
                }
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
          setMessages((prev) =>
            prev.map((m) =>
              m.chatID === chatId && m.role === 'assistant'
                ? { ...m, error: message }
                : m,
            ),
          )
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
