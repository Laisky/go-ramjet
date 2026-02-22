import type { Annotation } from '@/pages/gptchat/types'

/**
 * API client for GPTChat backend endpoints.
 * Handles both regular requests and streaming SSE responses.
 */

/**
 * getApiBase resolves the API base path from the current location.
 */
export function getApiBase(): string {
  if (typeof window === 'undefined') {
    return ''
  }
  const pathname = window.location?.pathname || ''
  const segments = pathname.split('/').filter(Boolean)
  if (segments[0] === 'gptchat') {
    return '/gptchat'
  }
  return ''
}

export interface ToolCallFunction {
  name: string
  arguments: string
}

export interface ToolCall {
  id: string
  type: 'function'
  function: ToolCallFunction
}

export interface ToolCallDelta {
  id?: string
  type?: string
  function?: Partial<ToolCallFunction>
}

export interface ChatMessage {
  role: 'system' | 'user' | 'assistant' | 'tool'
  content: string | ContentPart[]
  name?: string
  tool_call_id?: string
  tool_calls?: ToolCall[]
}

export interface ContentPart {
  type: 'text' | 'image_url'
  text?: string
  image_url?: { url: string }
}

export interface ChatRequest {
  model: string
  messages: ChatMessage[]
  max_tokens?: number
  temperature?: number
  presence_penalty?: number
  frequency_penalty?: number
  stream?: boolean
  enable_mcp?: boolean
  tools?: ChatTool[]
  tool_choice?: string | object
  mcp_servers?: McpServer[]
  laisky_extra?: {
    chat_switch?: {
      disable_https_crawler?: boolean
      enable_google_search?: boolean
      all_in_one?: boolean
      enable_memory?: boolean
    }
  }
}

// OpenAI-compatible tool definition
export interface ChatTool {
  type: 'function'
  function: {
    name: string
    description?: string
    parameters?: Record<string, unknown>
  }
  strict?: boolean
}

// Deep research
export interface DeepResearchTask {
  task_id: string
  status: string
  result?: string
  output?: string
  content?: string
  summary?: string
}

export interface McpServer {
  id?: string
  name: string
  url: string
  api_key?: string
  enabled?: boolean
  tools?: McpTool[]
  enabled_tool_names?: string[]
}

// MCP tool definition (may use input_schema instead of parameters)
export interface McpTool {
  name: string
  description?: string
  parameters?: Record<string, unknown>
  input_schema?: Record<string, unknown>
}

export interface ChatCompletionChoice {
  index: number
  message?: { role: string; content: string }
  delta?: {
    role?: string
    content?: string | ContentPart[]
    reasoning_content?: string
    reasoning?: string
    annotations?: Annotation[]
    tool_calls?: ToolCallDelta[]
  }
  finish_reason?: string | null
}

export interface ChatCompletionResponse {
  id: string
  object: string
  created: number
  model: string
  choices: ChatCompletionChoice[]
  usage?: {
    prompt_tokens: number
    completion_tokens: number
    total_tokens: number
  }
}

export interface StreamChunk {
  content: string
  reasoningContent?: string
  done: boolean
  error?: string
}

export interface ImageGenerationRequest {
  model: string
  prompt: string
  n?: number
  size?: string
  response_format?: 'url' | 'b64_json'
}

export interface ImageGenerationResponse {
  created: number
  data: Array<{
    url?: string
    b64_json?: string
    revised_prompt?: string
  }>
}

export interface ImageEditResponse {
  task_id: string
  image_urls: string[]
}

/**
 * Get SHA-1 hash of a string
 */
export async function getSHA1(str: string): Promise<string> {
  if (typeof crypto !== 'undefined' && crypto.subtle) {
    const encoder = new TextEncoder()
    const data = encoder.encode(str)
    const hash = await crypto.subtle.digest('SHA-1', data)
    return Array.from(new Uint8Array(hash))
      .map((b) => b.toString(16).padStart(2, '0'))
      .join('')
  }
  // Fallback for non-secure contexts (e.g., local dev over HTTP)
  console.debug('[getSHA1] crypto.subtle not available; using fallback hash')
  let hash = 2166136261
  for (let i = 0; i < str.length; i += 1) {
    hash ^= str.charCodeAt(i)
    hash += (hash << 1) + (hash << 4) + (hash << 7) + (hash << 8) + (hash << 24)
  }
  return (hash >>> 0).toString(16).padStart(8, '0')
}

/**
 * Build common headers for API requests
 */
export async function buildHeaders(
  apiToken: string,
  apiBase?: string,
): Promise<Record<string, string>> {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    Accept: 'application/json',
  }

  if (apiToken) {
    headers['Authorization'] = `Bearer ${apiToken}`
    headers['X-Laisky-User-Id'] = await getSHA1(apiToken)
  }

  if (apiBase) {
    headers['X-Laisky-Api-Base'] = apiBase
  }

  return headers
}

/**
 * Send a chat completion request (non-streaming)
 */
export async function sendChatRequest(
  request: ChatRequest,
  apiToken: string,
  apiBase?: string,
): Promise<ChatCompletionResponse> {
  const headers = await buildHeaders(apiToken, apiBase)

  const response = await fetch(`${getApiBase()}/api`, {
    method: 'POST',
    headers,
    body: JSON.stringify({ ...request, stream: false }),
  })

  if (!response.ok) {
    const text = await response.text()
    throw new Error(`[${response.status}]: ${text}`)
  }

  return response.json()
}

/**
 * Callback for handling stream events
 */
export interface StreamCallbacks {
  onContent?: (content: string) => void
  onReasoning?: (content: string) => void
  onAnnotations?: (annotations: Annotation[]) => void
  onToolCallDelta?: (toolCalls: ToolCallDelta[]) => void
  onFinish?: (finishReason?: string | null) => void
  onDone?: () => void
  onError?: (error: Error) => void
  onResponseInfo?: (info: { id: string; model: string }) => void
}

/**
 * Send a streaming chat completion request
 * Returns an abort controller to cancel the request
 */
export function sendStreamingChatRequest(
  request: ChatRequest,
  apiToken: string,
  callbacks: StreamCallbacks,
  apiBase?: string,
): AbortController {
  const abortController = new AbortController()
  let isThinking = false
  let collectedAnnotations: Annotation[] = []

  const run = async () => {
    try {
      const headers = await buildHeaders(apiToken, apiBase)
      headers['Accept'] = 'text/event-stream'

      const response = await fetch(`${getApiBase()}/api`, {
        method: 'POST',
        headers,
        body: JSON.stringify({ ...request, stream: true }),
        signal: abortController.signal,
      })

      if (!response.ok) {
        const text = await response.text()
        throw new Error(`[${response.status}]: ${text}`)
      }

      const requestId =
        response.headers.get('x-oneapi-request-id') ||
        response.headers.get('x-request-id')

      const reader = response.body?.getReader()
      if (!reader) {
        throw new Error('No response body')
      }

      const decoder = new TextDecoder()
      let buffer = ''

      while (true) {
        const { done, value } = await reader.read()

        if (done) {
          await callbacks.onDone?.()
          break
        }

        buffer += decoder.decode(value, { stream: true })

        // Process complete SSE messages
        const lines = buffer.split('\n')
        buffer = lines.pop() || '' // Keep incomplete line in buffer

        for (const line of lines) {
          if (!line.startsWith('data: ')) continue
          const data = line.slice(6).trim()

          if (data === '[DONE]') {
            await callbacks.onDone?.()
            return
          }

          try {
            const parsed = JSON.parse(data) as ChatCompletionResponse
            const id = requestId || parsed.id
            const model = parsed.model
            if (id || model) {
              callbacks.onResponseInfo?.({
                id: id || '',
                model: model || '',
              })
            }

            const delta = parsed.choices?.[0]?.delta
            if (!delta) {
              continue
            }

            if (delta.annotations && delta.annotations.length > 0) {
              collectedAnnotations = collectedAnnotations.concat(
                delta.annotations,
              )
              callbacks.onAnnotations?.(collectedAnnotations)
            }

            if (delta.tool_calls && delta.tool_calls.length > 0) {
              callbacks.onToolCallDelta?.(delta.tool_calls)
            }

            if (delta.reasoning_content) {
              callbacks.onReasoning?.(delta.reasoning_content)
            }
            if (delta.reasoning) {
              callbacks.onReasoning?.(delta.reasoning)
            }

            const chunk = delta.content
            if (!chunk) {
              continue
            }

            const contentChunks: string[] = []
            const reasoningChunks: string[] = []
            const pushContent = (val?: string) => {
              if (val) {
                contentChunks.push(val)
              }
            }
            const pushReasoning = (val?: string) => {
              if (val) {
                reasoningChunks.push(val)
              }
            }

            if (typeof chunk === 'string') {
              if (chunk === '<think>') {
                isThinking = true
                continue
              }
              if (chunk === '</think>') {
                isThinking = false
                continue
              }
              if (isThinking) {
                pushReasoning(chunk)
              } else {
                pushContent(chunk)
              }
            } else if (Array.isArray(chunk)) {
              for (const part of chunk) {
                if (!part) continue
                if (part.type === 'text') {
                  if (isThinking) {
                    pushReasoning(part.text)
                  } else {
                    pushContent(part.text)
                  }
                } else if (part.type === 'image_url' && part.image_url?.url) {
                  pushContent(`\n![Image](${part.image_url.url})\n`)
                }
              }
            }

            if (contentChunks.length > 0) {
              callbacks.onContent?.(contentChunks.join(''))
            }
            if (reasoningChunks.length > 0) {
              callbacks.onReasoning?.(reasoningChunks.join(''))
            }

            if (parsed.choices?.[0]?.finish_reason) {
              callbacks.onFinish?.(parsed.choices[0].finish_reason)
            }
          } catch {
            console.debug('Non-JSON SSE data:', data)
          }
        }
      }
    } catch (error) {
      if (error instanceof Error && error.name === 'AbortError') {
        // Request was cancelled
        return
      }
      callbacks.onError?.(
        error instanceof Error ? error : new Error(String(error)),
      )
    }
  }

  run()
  return abortController
}

/**
 * Send an image generation request
 */
export async function generateImage(
  request: ImageGenerationRequest,
  apiToken: string,
  apiBase?: string,
): Promise<ImageGenerationResponse> {
  const headers = await buildHeaders(apiToken, apiBase)

  const response = await fetch(`${getApiBase()}/images/generations`, {
    method: 'POST',
    headers,
    body: JSON.stringify(request),
  })

  if (!response.ok) {
    const text = await response.text()
    throw new Error(`[${response.status}]: ${text}`)
  }

  return response.json()
}

/**
 * Edit an image with mask using Flux fill endpoint.
 */
export async function editImageWithMask(
  model: string,
  payload: {
    prompt: string
    image: string
    mask: string
  },
  apiToken: string,
  apiBase?: string,
): Promise<ImageEditResponse> {
  const headers = await buildHeaders(apiToken, apiBase)
  const body = {
    prompt: payload.prompt,
    image: payload.image,
    mask: payload.mask,
    model,
  }

  const response = await fetch(`${getApiBase()}/images/edits`, {
    method: 'POST',
    headers,
    body: JSON.stringify(body),
  })

  if (!response.ok) {
    const text = await response.text()
    throw new Error(`[${response.status}]: ${text}`)
  }

  return response.json()
}

/**
 * Create a deep-research task.
 */
export async function createDeepResearchTask(
  prompt: string,
  apiToken: string,
  apiBase?: string,
): Promise<{ task_id: string }> {
  const headers = await buildHeaders(apiToken, apiBase)

  const response = await fetch(`${getApiBase()}/deepresearch`, {
    method: 'POST',
    headers,
    body: JSON.stringify({ prompt }),
  })

  if (!response.ok) {
    const text = await response.text()
    throw new Error(`[${response.status}]: ${text}`)
  }

  return response.json()
}

/**
 * Fetch deep-research task status.
 */
export async function fetchDeepResearchStatus(
  taskId: string,
  apiToken: string,
  apiBase?: string,
): Promise<DeepResearchTask> {
  const headers = await buildHeaders(apiToken, apiBase)

  const response = await fetch(`${getApiBase()}/deepresearch/${taskId}`, {
    method: 'GET',
    headers,
  })

  if (!response.ok) {
    const text = await response.text()
    throw new Error(`[${response.status}]: ${text}`)
  }

  return response.json()
}

/**
 * Upload files for chat context
 */
export async function uploadFiles(
  files: File[],
  apiToken: string,
): Promise<{ cache_keys: string[] }> {
  const formData = new FormData()
  for (const file of files) {
    formData.append('files', file)
  }

  const response = await fetch(`${getApiBase()}/files/chat`, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${apiToken}`,
      'X-Laisky-User-Id': await getSHA1(apiToken),
    },
    body: formData,
  })

  if (!response.ok) {
    const text = await response.text()
    throw new Error(`[${response.status}]: ${text}`)
  }

  return response.json()
}

/**
 * Upload a single file for chat context
 */
export async function uploadFile(
  file: File,
  apiToken: string,
): Promise<{ url: string }> {
  const fileExt = file.name.slice(file.name.lastIndexOf('.'))
  const formData = new FormData()
  formData.append('file', file)
  formData.append('file_ext', fileExt)

  const response = await fetch(`${getApiBase()}/files/chat`, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${apiToken}`,
      'X-Laisky-User-Id': await getSHA1(apiToken),
    },
    body: formData,
  })

  if (!response.ok) {
    const text = await response.text()
    throw new Error(`[${response.status}]: ${text}`)
  }

  return response.json()
}

export async function transcribeAudio(
  file: File,
  apiToken: string,
): Promise<string> {
  const formData = new FormData()
  formData.append('file', file)
  formData.append('model', 'whisper-1')

  const response = await fetch(
    `${getApiBase()}/oneapi/v1/audio/transcriptions`,
    {
      method: 'POST',
      headers: {
        Authorization: `Bearer ${apiToken}`,
        'X-Laisky-User-Id': await getSHA1(apiToken),
      },
      body: formData,
    },
  )

  if (!response.ok) {
    const text = await response.text()
    throw new Error(`[${response.status}]: ${text}`)
  }

  const data = await response.json().catch(() => ({}))
  if (typeof data?.text === 'string') {
    return data.text
  }
  if (Array.isArray(data?.segments)) {
    return data.segments
      .map((seg: { text?: string }) => seg.text)
      .filter(Boolean)
      .join(' ')
      .trim()
  }
  return ''
}

/**
 * Get current user info
 */
export async function getCurrentUser(
  apiToken: string,
): Promise<{ username: string; is_member: boolean }> {
  const response = await fetch(`${getApiBase()}/user/me`, {
    headers: {
      Authorization: `Bearer ${apiToken}`,
    },
  })

  if (!response.ok) {
    const text = await response.text()
    throw new Error(`[${response.status}]: ${text}`)
  }

  return response.json()
}

/**
 * Create a payment intent
 */
export async function createPaymentIntent(
  items: object[],
): Promise<{ clientSecret: string }> {
  const response = await fetch(`${getApiBase()}/create-payment-intent`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Accept: 'application/json',
    },
    body: JSON.stringify({ items }),
  })

  if (!response.ok) {
    const text = await response.text()
    throw new Error(`[${response.status}]: ${text}`)
  }

  return response.json()
}

/**
 * Fetch TTS audio from the server (Azure TTS).
 * Returns an object URL for the audio blob that can be used with an <audio> element.
 *
 * @param text - The text to convert to speech
 * @param apiToken - The API token for authentication
 * @returns Promise resolving to an object URL for the audio blob
 */
export async function fetchTTS(
  text: string,
  apiToken: string,
): Promise<string> {
  const url = `${getApiBase()}/audio/tts?apikey=${encodeURIComponent(apiToken)}&text=${encodeURIComponent(text)}`

  console.debug('[fetchTTS] Requesting TTS audio:', {
    textLength: text.length,
    url: url.replace(apiToken, '***'),
  })

  const response = await fetch(url, {
    method: 'GET',
  })

  if (!response.ok) {
    const errorText = await response.text()
    console.debug('[fetchTTS] Server error:', {
      status: response.status,
      error: errorText,
    })
    throw new Error(`TTS request failed [${response.status}]: ${errorText}`)
  }

  const blob = await response.blob()
  console.debug('[fetchTTS] Received audio blob:', {
    size: blob.size,
    type: blob.type,
  })

  // Create a WAV blob and object URL
  const wavBlob = new Blob([blob], { type: 'audio/wav' })
  const objectUrl = URL.createObjectURL(wavBlob)

  return objectUrl
}
