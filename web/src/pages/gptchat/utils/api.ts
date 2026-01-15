import { getApiBase, getSHA1 } from '@/utils/api'
import type { UserConfig } from '../types'

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
    throw new ApiError(response.status, await response.text())
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

export const api = {
  fetchCurrentUser: (token: string) => {
    return request<UserConfig>('/user/me', {
      headers: {
        Authorization: `Bearer ${token}`,
      },
    })
  },

  async uploadUserData(syncKey: string, data: any) {
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
      const data = await request<any>('/user/config', {
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
        if ((err.status === 400 || err.status === 404) && msg.includes('does not exist')) {
          return {}
        }
      }
      throw err
    }
  },

  async uploadDataset(
    file: File,
    datasetName: string,
    dataKey: string,
    apiToken: string,
    apiBase?: string,
  ): Promise<void> {
    const form = new FormData()
    form.append('file', file)
    form.append('file_key', datasetName)
    form.append('data_key', dataKey)

    const headers: Record<string, string> = {
      Authorization: `Bearer ${apiToken}`,
      'X-Laisky-User-Id': await getSHA1(apiToken),
    }
    if (apiBase) {
      headers['X-Laisky-Api-Base'] = apiBase
    }

    const resp = await fetch(`${resolveApiBase()}/ramjet/gptchat/files`, {
      method: 'POST',
      headers,
      body: form,
    })

    if (!resp.ok) {
      throw new Error(`[${resp.status}]: ${await resp.text()}`)
    }
  },

  async listDatasets(
    dataKey: string,
    apiToken: string,
    apiBase?: string,
  ): Promise<{ datasets: DatasetInfo[]; selected?: string[] }> {
    const headers: Record<string, string> = {
      Authorization: `Bearer ${apiToken}`,
      'X-Laisky-User-Id': await getSHA1(apiToken),
      'Cache-Control': 'no-cache',
      'X-PDFCHAT-PASSWORD': dataKey,
    }
    if (apiBase) {
      headers['X-Laisky-Api-Base'] = apiBase
    }

    const resp = await fetch(`${resolveApiBase()}/ramjet/gptchat/files`, {
      method: 'GET',
      headers,
      cache: 'no-cache',
    })
    if (!resp.ok) {
      throw new Error(`[${resp.status}]: ${await resp.text()}`)
    }
    return resp.json()
  },

  async deleteDataset(
    datasetName: string,
    dataKey: string,
    apiToken: string,
    apiBase?: string,
  ): Promise<void> {
    const headers: Record<string, string> = {
      Authorization: `Bearer ${apiToken}`,
      'X-Laisky-User-Id': await getSHA1(apiToken),
      'Cache-Control': 'no-cache',
      'Content-Type': 'application/json',
      'X-PDFCHAT-PASSWORD': dataKey,
    }
    if (apiBase) {
      headers['X-Laisky-Api-Base'] = apiBase
    }

    const resp = await fetch(`${resolveApiBase()}/ramjet/gptchat/files`, {
      method: 'DELETE',
      headers,
      body: JSON.stringify({ datasets: [datasetName] }),
    })
    if (!resp.ok) {
      throw new Error(`[${resp.status}]: ${await resp.text()}`)
    }
  },

  async listChatbots(
    dataKey: string,
    apiToken: string,
    apiBase?: string,
  ): Promise<ChatbotList> {
    const headers: Record<string, string> = {
      Authorization: `Bearer ${apiToken}`,
      'X-Laisky-User-Id': await getSHA1(apiToken),
      'Cache-Control': 'no-cache',
      'X-PDFCHAT-PASSWORD': dataKey,
    }
    if (apiBase) {
      headers['X-Laisky-Api-Base'] = apiBase
    }

    const resp = await fetch(`${resolveApiBase()}/ramjet/gptchat/ctx/list`, {
      method: 'GET',
      headers,
      cache: 'no-cache',
    })
    if (!resp.ok) {
      throw new Error(`[${resp.status}]: ${await resp.text()}`)
    }
    return resp.json()
  },

  async setActiveChatbot(
    dataKey: string,
    chatbotName: string,
    apiToken: string,
    apiBase?: string,
  ): Promise<void> {
    const headers: Record<string, string> = {
      Authorization: `Bearer ${apiToken}`,
      'X-Laisky-User-Id': await getSHA1(apiToken),
      'Content-Type': 'application/json',
    }
    if (apiBase) {
      headers['X-Laisky-Api-Base'] = apiBase
    }

    const resp = await fetch(`${resolveApiBase()}/ramjet/gptchat/ctx/active`, {
      method: 'POST',
      headers,
      body: JSON.stringify({ data_key: dataKey, chatbot_name: chatbotName }),
    })
    if (!resp.ok) {
      throw new Error(`[${resp.status}]: ${await resp.text()}`)
    }
  },
}
