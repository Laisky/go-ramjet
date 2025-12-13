/**
 * API client for GPTChat backend endpoints.
 * Handles both regular requests and streaming SSE responses.
 */

export interface ChatMessage {
  role: 'system' | 'user' | 'assistant'
  content: string | ContentPart[]
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
  mcp_servers?: McpServer[]
}

export interface McpServer {
  name: string
  url: string
  api_key?: string
}

export interface ChatCompletionChoice {
  index: number
  message?: { role: string; content: string }
  delta?: { role?: string; content?: string; reasoning_content?: string }
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
  // Fallback - should not happen in modern browsers
  throw new Error('crypto.subtle not available')
}

/**
 * Build common headers for API requests
 */
export async function buildHeaders(
  apiToken: string,
  apiBase?: string
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
  apiBase?: string
): Promise<ChatCompletionResponse> {
  const headers = await buildHeaders(apiToken, apiBase)

  const response = await fetch('/gptchat/api', {
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
  onDone?: () => void
  onError?: (error: Error) => void
}

/**
 * Send a streaming chat completion request
 * Returns an abort controller to cancel the request
 */
export function sendStreamingChatRequest(
  request: ChatRequest,
  apiToken: string,
  callbacks: StreamCallbacks,
  apiBase?: string
): AbortController {
  const abortController = new AbortController()

  const run = async () => {
    try {
      const headers = await buildHeaders(apiToken, apiBase)
      headers['Accept'] = 'text/event-stream'

      const response = await fetch('/gptchat/api', {
        method: 'POST',
        headers,
        body: JSON.stringify({ ...request, stream: true }),
        signal: abortController.signal,
      })

      if (!response.ok) {
        const text = await response.text()
        throw new Error(`[${response.status}]: ${text}`)
      }

      const reader = response.body?.getReader()
      if (!reader) {
        throw new Error('No response body')
      }

      const decoder = new TextDecoder()
      let buffer = ''

      while (true) {
        const { done, value } = await reader.read()

        if (done) {
          callbacks.onDone?.()
          break
        }

        buffer += decoder.decode(value, { stream: true })

        // Process complete SSE messages
        const lines = buffer.split('\n')
        buffer = lines.pop() || '' // Keep incomplete line in buffer

        for (const line of lines) {
          if (line.startsWith('data: ')) {
            const data = line.slice(6).trim()

            if (data === '[DONE]') {
              callbacks.onDone?.()
              return
            }

            try {
              const parsed = JSON.parse(data) as ChatCompletionResponse
              const delta = parsed.choices?.[0]?.delta

              if (delta?.content) {
                callbacks.onContent?.(delta.content)
              }

              if (delta?.reasoning_content) {
                callbacks.onReasoning?.(delta.reasoning_content)
              }
            } catch {
              // Ignore parse errors for non-JSON data
              console.debug('Non-JSON SSE data:', data)
            }
          }
        }
      }
    } catch (error) {
      if (error instanceof Error && error.name === 'AbortError') {
        // Request was cancelled
        return
      }
      callbacks.onError?.(error instanceof Error ? error : new Error(String(error)))
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
  apiBase?: string
): Promise<ImageGenerationResponse> {
  const headers = await buildHeaders(apiToken, apiBase)

  const response = await fetch('/gptchat/images/generations', {
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
 * Upload files for chat context
 */
export async function uploadFiles(
  files: File[],
  apiToken: string
): Promise<{ cache_keys: string[] }> {
  const formData = new FormData()
  for (const file of files) {
    formData.append('files', file)
  }

  const response = await fetch('/gptchat/files/chat', {
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
 * Get current user info
 */
export async function getCurrentUser(
  apiToken: string
): Promise<{ username: string; is_member: boolean }> {
  const response = await fetch('/gptchat/user/me', {
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
  items: object[]
): Promise<{ clientSecret: string }> {
  const response = await fetch('/gptchat/create-payment-intent', {
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
