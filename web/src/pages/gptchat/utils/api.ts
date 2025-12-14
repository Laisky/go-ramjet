import type { UserConfig } from '../types'

const API_BASE = '/gptchat'

export class ApiError extends Error {
  status: number

  constructor(status: number, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

async function request<T>(
  endpoint: string,
  options: RequestInit = {}
): Promise<T> {
  const response = await fetch(`${API_BASE}${endpoint}`, options)

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
    const data = await request<any>('/user/config', {
      method: 'GET',
      headers: {
        'X-LAISKY-SYNC-KEY': syncKey,
        'Cache-Control': 'no-cache',
      },
    })
    return data
  },
}
