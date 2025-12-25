import type { McpServerConfig, McpTool } from '../types'

type MutableServer = McpServerConfig & Record<string, unknown>

// Helper to generate UUID v4
function generateUUIDv4() {
  return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, function (c) {
    const r = (Math.random() * 16) | 0,
      v = c == 'x' ? r : (r & 0x3) | 0x8
    return v.toString(16)
  })
}

// Helper: Trim space
function trimSpace(str?: string) {
  return (str || '').trim()
}

// Helper: Generate random string
function randomString(length: number) {
  return Math.random()
    .toString(36)
    .substring(2, 2 + length)
}

/**
 * fetchJSONOrSSE tries to parse response as JSON.
 * If response Content-Type is text/event-stream, it parses SSE events.
 */
async function fetchJSONOrSSE(resp: Response): Promise<unknown> {
  const ct = (resp.headers.get('content-type') || '').toLowerCase()
  if (ct.includes('application/json')) {
    return await resp.json()
  }

  if (ct.includes('text/event-stream')) {
    // Simple SSE parser for single message scenarios often used in legacy code
    const text = await resp.text()
    const lines = text.split('\n')
    for (const line of lines) {
      if (line.startsWith('data: ')) {
        const dataStr = line.substring(6).trim()
        if (dataStr && dataStr !== '[DONE]') {
          try {
            return JSON.parse(dataStr)
          } catch (e) {
            // ignore
          }
        }
      }
    }
  }

  // Fallback try json
  try {
    return await resp.json()
  } catch {
    // If not json, return text? Or null? Legacy returned null or threw.
    return null
  }
}

/**
 * normalizeMCPToolListResponse ensures the tool list is an array.
 */
function normalizeMCPToolListResponse(data: unknown): unknown[] | null {
  if (!data) return null

  if (typeof data === 'object' && data !== null) {
    const maybeResult = data as {
      result?: { tools?: unknown[] }
      tools?: unknown[]
    }
    if (Array.isArray(maybeResult.result?.tools)) {
      return maybeResult.result.tools
    }
    if (Array.isArray(maybeResult.tools)) {
      return maybeResult.tools
    }
  }

  if (Array.isArray(data)) {
    return data
  }

  return null
}

function normalizeToolsToOpenAIFormat(tools: unknown[]): McpTool[] {
  return tools.map((tool) => {
    const item = tool as Record<string, unknown>
    return {
      name: typeof item.name === 'string' ? item.name : 'unknown-tool',
      description:
        typeof item.description === 'string' ? item.description : undefined,
      input_schema: (item.inputSchema || item.parameters) as
        | Record<string, unknown>
        | undefined,
    }
  })
}

async function ensureMCPSession(
  server: MutableServer,
  endpointURL: string,
  headers: Record<string, string>,
) {
  if (server.mcp_initialized) return

  const protocolVersion = trimSpace(
    String(server.mcp_protocol_version || '2025-06-18'),
  )
  let sessionID = trimSpace(
    typeof server.mcp_session_id === 'string'
      ? server.mcp_session_id
      : String(server.mcp_session_id || ''),
  )
  if (!sessionID) {
    sessionID = `mcp-session-${generateUUIDv4()}`
    // We modify the server object in place (in memory), caller should handle persistence if needed
    server.mcp_session_id = sessionID
  }

  const initPayload = {
    jsonrpc: '2.0',
    id: 0,
    method: 'initialize',
    params: {
      protocolVersion,
      capabilities: {
        sampling: {},
        elicitation: {},
        roots: { listChanged: true },
      },
      clientInfo: {
        name: 'go-ramjet-gptchat',
        version: '0.0.0',
      },
    },
  }

  // Headers for init
  const h = { ...headers }
  // Remove mcp specific headers for init if legacy did so?
  // Legacy: "Use MCP-required headers after initialize (server may ignore them during initialize)."
  // But then it adds them in initHeaders?
  // It says "const initHeaders... 'mcp-protocol-version'..."
  // Wait, line 7914 in legacy: initHeaders includes them.
  // But the loop line 7926 uses initHeaders.
  // So we should include them.
  h['mcp-protocol-version'] = protocolVersion
  h['mcp-session-id'] = sessionID

  const resp = await fetch(endpointURL, {
    method: 'POST',
    headers: h,
    body: JSON.stringify(initPayload),
  })

  if (!resp.ok) {
    throw new Error(`HTTP ${resp.status} during MCP init`)
  }

  const initData: any = await fetchJSONOrSSE(resp)
  if (initData?.error) {
    throw new Error(initData.error.message || 'MCP initialize error')
  }

  // Notifications/initialized
  const notifyPayload = {
    jsonrpc: '2.0',
    method: 'notifications/initialized',
  }

  await fetch(endpointURL, {
    method: 'POST',
    headers: h,
    body: JSON.stringify(notifyPayload),
  })

  server.mcp_initialized = true
}

export async function syncMCPServerTools(
  server: McpServerConfig,
): Promise<{ updatedServer: McpServerConfig; count: number }> {
  const endpointURL = server.url.endsWith('/') ? server.url : `${server.url}/`

  // Auth
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    Accept: 'application/json, text/event-stream',
  }
  if (server.api_key) {
    // Try raw first as per legacy
    headers['Authorization'] = server.api_key
  }

  // 1. Ensure Session
  // We treat the passed server config as mutable for session state,
  // but we return a clean new config object for React state.
  // Note: server.mcp_session_id etc are not in McpServerConfig type yet,
  // but we can cast or add them to type if we want to persist them.
  // For now we'll store them in a temporary object extending config.
  const sessionServer = { ...server } as MutableServer

  await ensureMCPSession(sessionServer, endpointURL, headers)

  // 2. Fetch Tools (JSON-RPC)
  const payload = {
    jsonrpc: '2.0',
    id: randomString(8),
    method: 'tools/list',
    params: {},
  }

  const protocolVersion = String(
    sessionServer.mcp_protocol_version || '2025-06-18',
  )
  const sessionID = String(sessionServer.mcp_session_id || '')

  const rpcHeaders = {
    ...headers,
    'mcp-protocol-version': protocolVersion,
    'mcp-session-id': sessionID,
  }

  const resp = await fetch(endpointURL, {
    method: 'POST',
    headers: rpcHeaders,
    body: JSON.stringify(payload),
  })

  if (!resp.ok) {
    throw new Error(`HTTP ${resp.status} fetching tools`)
  }

  const data: any = await fetchJSONOrSSE(resp)
  if (!data) throw new Error('Invalid JSON-RPC response')
  if (data.error) throw new Error(data.error.message || 'MCP error')

  const normalizedRawTools = normalizeMCPToolListResponse(data)
  if (!normalizedRawTools) throw new Error('Invalid tool list format')

  const tools = normalizeToolsToOpenAIFormat(normalizedRawTools)

  // 3. Update Server Config
  const updatedServer = { ...server, tools }

  // Update enabled tools logic
  const toolNames = tools.map((t) => t.name)
  const prevEnabled = new Set(server.enabled_tool_names || [])

  const newEnabled: string[] = []
  if (prevEnabled.size === 0) {
    // Enable all by default if none were selected previously (fresh sync)
    newEnabled.push(...toolNames)
  } else {
    // Keep existing selections, add new ones??
    // Legacy: "Default to enabling all synced tools, while preserving any prior user selections."
    // Wait, legacy code:
    // if (prevEnabled.length === 0 || prevSet.has(name)) { merged.push(name) }
    // This implicitly removes tools that were explicitly unchecked? No, if prevEnabled is empty, it adds all.
    // If prevEnabled is NOT empty, it only keeps name if it was in prevEnabled.
    // BUT wait, "Default to enabling all synced tools" comment vs code.
    // Code: `if (prevEnabled.length === 0 || prevSet.has(name))`
    // If I have [A] enabled, and I sync [A, B]. prevSet has A.
    // Name A: prevSet has A -> push A.
    // Name B: prevSet !has B -> Don't push?
    // So legacy code seemingly DOES NOT auto-enable new tools if you already have a selection.
    // UNLESS the comment means something else.
    // Let's stick to: If empty, enable all. If not empty, only keep intersections.
    for (const name of toolNames) {
      if (prevEnabled.size === 0 || prevEnabled.has(name)) {
        newEnabled.push(name)
      }
    }
  }

  updatedServer.enabled_tool_names = newEnabled

  return { updatedServer, count: tools.length }
}

function stringifyToolResult(data: unknown): string {
  if (data === null || typeof data === 'undefined') return ''
  if (typeof data === 'string') return data
  if (typeof data === 'number' || typeof data === 'boolean') return String(data)
  try {
    return JSON.stringify(data)
  } catch (err) {
    return String(data)
  }
}

function normalizeArguments(args: unknown): unknown {
  if (typeof args === 'string') {
    try {
      return JSON.parse(args)
    } catch {
      return { _raw: args }
    }
  }
  if (typeof args === 'object' && args !== null) {
    return args
  }
  return {}
}

function guessToolCallEndpoints(baseUrl?: string): string[] {
  const normalized = (baseUrl || '').replace(/\/+$/, '')
  if (!normalized) return []
  return Array.from(
    new Set([`${normalized}/tools/call`, `${normalized}/call`, normalized]),
  )
}

function getAuthHeaderCandidates(apiKey?: string): string[] {
  if (!apiKey) return []
  const trimmed = apiKey.trim()
  if (!trimmed) return []
  if (/^bearer\s+/i.test(trimmed)) {
    return [trimmed]
  }
  return [trimmed, `Bearer ${trimmed}`]
}

export async function callMCPTool(
  server: McpServerConfig,
  toolName: string,
  args: unknown,
): Promise<string> {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    Accept: 'application/json, text/event-stream',
  }

  const authCandidates = getAuthHeaderCandidates(server.api_key)
  const payload = {
    name: toolName,
    arguments: normalizeArguments(args),
  }

  const endpoints = guessToolCallEndpoints(server.url)
  let lastErr: Error | null = null

  for (const endpoint of endpoints) {
    const auths = authCandidates.length > 0 ? authCandidates : ['']
    for (const auth of auths) {
      try {
        const h = { ...headers }
        if (auth) {
          h.Authorization = auth
        } else {
          delete h.Authorization
        }
        const resp = await fetch(endpoint, {
          method: 'POST',
          headers: h,
          body: JSON.stringify(payload),
        })
        if (!resp.ok) {
          lastErr = new Error(`HTTP ${resp.status}`)
          continue
        }
        const ct = (resp.headers.get('content-type') || '').toLowerCase()
        if (ct.includes('application/json')) {
          const data = await resp.json().catch(() => null)
          if (data && typeof data === 'object') {
            if (data.result) return stringifyToolResult(data.result)
            if (data.content) return stringifyToolResult(data.content)
          }
          return stringifyToolResult(data)
        }
        const text = await resp.text()
        return text
      } catch (err) {
        lastErr = err instanceof Error ? err : new Error(String(err))
        continue
      }
    }
  }

  throw lastErr || new Error('Failed to contact MCP server')
}
