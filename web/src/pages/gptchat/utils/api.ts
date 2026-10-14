import { markPrivateError, privateResponseError } from '@/telemetry/privacy'
import { getApiBase, getSHA1 } from '@/utils/api'
import type { UserConfig } from '../types'
import { ownershipSessionHeader } from './ownership-session'

const resolveApiBase = () => getApiBase()

export class ApiError extends Error {
  status: number

  constructor(status: number, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

export interface DatasetInfo {
  name: string
  taskStatus?: string
  progress?: number
}

export interface ChatbotList {
  chatbots: string[]
  current?: string
}

async function request<T>(
  endpoint: string,
  options: RequestInit = {},
): Promise<T> {
  const response = await fetch(`${resolveApiBase()}${endpoint}`, options)

  if (!response.ok) {
    throw markPrivateError(
      new ApiError(response.status, await response.text()),
      'Chat account request failed (HTTP ' + response.status + ')',
      true,
    )
  }

  // Handle empty responses
  const text = await response.text()
  if (!text) {
    return {} as T
  }

  try {
    return JSON.parse(text)
  } catch {
    return text as unknown as T
  }
}

/**
 * buildRamjetHeaders validates a supplied BYOK key and returns its authorization,
 * legacy identity, separate original SSO session, and unchanged provider endpoint.
 */
async function buildRamjetHeaders(
  apiToken: string,
  apiBase?: string,
  sessionToken?: string,
): Promise<Record<string, string>> {
  if (
    typeof apiToken !== 'string' ||
    !apiToken ||
    apiToken === 'DEFAULT_PROXY_TOKEN' ||
    apiToken.startsWith('FREETIER-') ||
    /[\s\p{Cc}]/u.test(apiToken)
  ) {
    throw new Error('A valid BYOK API key is required for Ramjet operations.')
  }

  const headers: Record<string, string> = {
    ...ownershipSessionHeader(sessionToken),
    Authorization: `Bearer ${apiToken}`,
    'X-Laisky-User-Id': await getSHA1(apiToken),
  }
  if (apiBase) {
    headers['X-Laisky-Api-Base'] = apiBase
  }
  return headers
}

/**
 * withCacheBuster returns an endpoint URL that bypasses stale browser/proxy caches.
 */
function withCacheBuster(endpoint: string): string {
  const sep = endpoint.includes('?') ? '&' : '?'
  return `${endpoint}${sep}_=${Date.now()}`
}

export const api = {
  fetchCurrentUser: (token: string) => {
    return request<UserConfig>(withCacheBuster('/user/me'), {
      cache: 'no-store',
      headers: {
        Authorization: `Bearer ${token}`,
        'Cache-Control': 'no-cache',
        Pragma: 'no-cache',
      },
    })
  },

  async uploadUserData(syncKey: string, data: unknown) {
    // Compress data
    const jsonStr = JSON.stringify(data)

    await request('/user/config', {
      method: 'POST',
      headers: {
        'X-LAISKY-SYNC-KEY': syncKey,
        'Content-Type': 'application/json',
      },
      body: jsonStr, // Backend expects raw body? Or compressed? Backend compresses it.
      // Wait, backend: "body, err := ctx.GetRawData() ... gcompress.GzCompress"
      // So backend expects the raw JSON string (or whatever bytes), then IT compresses and encrypts.
      // Legacy code sent JSON object via fetch, so it was stringified.
    })
  },

  async downloadUserData(syncKey: string) {
    try {
      const data = await request<unknown>('/user/config', {
        method: 'GET',
        headers: {
          'X-LAISKY-SYNC-KEY': syncKey,
          'Cache-Control': 'no-cache',
        },
      })
      return data
    } catch (err) {
      if (err instanceof ApiError) {
        const msg = (err.message || '').toLowerCase()
        if (
          (err.status === 400 || err.status === 404) &&
          msg.includes('does not exist')
        ) {
          return {}
        }
      }
      throw err
    }
  },

  /** uploadDataset uploads a private file using a separate SSO session and the selected model key. */
  async uploadDataset(
    file: File,
    datasetName: string,
    dataKey: string,
    apiToken: string,
    apiBase?: string,
    sessionToken?: string,
    signal?: AbortSignal,
  ): Promise<void> {
    const form = new FormData()
    form.append('file', file)
    form.append('file_key', datasetName)
    form.append('data_key', dataKey)

    const headers = await buildRamjetHeaders(apiToken, apiBase, sessionToken)

    const resp = await fetch(`${resolveApiBase()}/ramjet/gptchat/files`, {
      method: 'POST',
      headers,
      signal,
      redirect: 'error',
      mode: 'same-origin',
      body: form,
    })

    if (!resp.ok) {
      throw privateResponseError(
        `[${resp.status}]: ${await resp.text()}`,
        'Ramjet request failed (HTTP ' + resp.status + ')',
      )
    }
  },

  /** listDatasets reads account-owned metadata using SSO; legacy model arguments are ignored. */
  async listDatasets(
    dataKey: string,
    _apiToken: string,
    _apiBase?: string,
    sessionToken?: string,
    signal?: AbortSignal,
  ): Promise<{ datasets: DatasetInfo[]; selected?: string[] }> {
    const headers: Record<string, string> = {
      ...ownershipSessionHeader(sessionToken),
      'Cache-Control': 'no-cache',
      'X-PDFCHAT-PASSWORD': dataKey,
    }

    const resp = await fetch(`${resolveApiBase()}/ramjet/gptchat/files`, {
      method: 'GET',
      headers,
      signal,
      redirect: 'error',
      mode: 'same-origin',
      cache: 'no-cache',
    })
    if (!resp.ok) {
      throw privateResponseError(
        `[${resp.status}]: ${await resp.text()}`,
        'Ramjet request failed (HTTP ' + resp.status + ')',
      )
    }
    return resp.json()
  },

  /** deleteDataset removes an account-owned dataset using SSO; legacy model arguments are ignored. */
  async deleteDataset(
    datasetName: string,
    dataKey: string,
    _apiToken: string,
    _apiBase?: string,
    sessionToken?: string,
    signal?: AbortSignal,
  ): Promise<void> {
    const headers: Record<string, string> = {
      ...ownershipSessionHeader(sessionToken),
      'Cache-Control': 'no-cache',
      'Content-Type': 'application/json',
      'X-PDFCHAT-PASSWORD': dataKey,
    }

    const resp = await fetch(`${resolveApiBase()}/ramjet/gptchat/files`, {
      method: 'DELETE',
      headers,
      signal,
      redirect: 'error',
      mode: 'same-origin',
      body: JSON.stringify({ datasets: [datasetName] }),
    })
    if (!resp.ok) {
      throw privateResponseError(
        `[${resp.status}]: ${await resp.text()}`,
        'Ramjet request failed (HTTP ' + resp.status + ')',
      )
    }
  },

  /** listChatbots reads account-owned chatbot metadata using SSO; legacy model arguments are ignored. */
  async listChatbots(
    dataKey: string,
    _apiToken: string,
    _apiBase?: string,
    sessionToken?: string,
    signal?: AbortSignal,
  ): Promise<ChatbotList> {
    const headers: Record<string, string> = {
      ...ownershipSessionHeader(sessionToken),
      'Cache-Control': 'no-cache',
      'X-PDFCHAT-PASSWORD': dataKey,
    }

    const resp = await fetch(`${resolveApiBase()}/ramjet/gptchat/ctx/list`, {
      method: 'GET',
      headers,
      signal,
      redirect: 'error',
      mode: 'same-origin',
      cache: 'no-cache',
    })
    if (!resp.ok) {
      throw privateResponseError(
        `[${resp.status}]: ${await resp.text()}`,
        'Ramjet request failed (HTTP ' + resp.status + ')',
      )
    }
    return resp.json()
  },

  /** setActiveChatbot activates a private chatbot using a separate SSO session and selected model key. */
  async setActiveChatbot(
    dataKey: string,
    chatbotName: string,
    apiToken: string,
    apiBase?: string,
    sessionToken?: string,
    signal?: AbortSignal,
  ): Promise<void> {
    const headers: Record<string, string> = {
      ...(await buildRamjetHeaders(apiToken, apiBase, sessionToken)),
      'Content-Type': 'application/json',
    }

    const resp = await fetch(`${resolveApiBase()}/ramjet/gptchat/ctx/active`, {
      method: 'POST',
      headers,
      signal,
      redirect: 'error',
      mode: 'same-origin',
      body: JSON.stringify({ data_key: dataKey, chatbot_name: chatbotName }),
    })
    if (!resp.ok) {
      throw privateResponseError(
        `[${resp.status}]: ${await resp.text()}`,
        'Ramjet request failed (HTTP ' + resp.status + ')',
      )
    }
  },
}
