import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  sendChatRequest,
  sendStreamingChatRequest,
  generateImage,
  uploadFiles,
  type ChatRequest,
} from '../api'

describe('chat transport after research retirement', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('preserves ordinary chat with MCP, search, memory, and retrieval attachments', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue({ ok: true, json: async () => ({ choices: [] }) })
    vi.stubGlobal('fetch', fetchMock)
    const request: ChatRequest = {
      model: 'gpt-4o-mini',
      messages: [
        {
          role: 'user',
          content: [
            { type: 'text', text: 'Use the uploaded document' },
            {
              type: 'image_url',
              image_url: { url: 'https://example.com/image.png' },
            },
          ],
        },
      ],
      enable_mcp: true,
      tools: [
        { type: 'function', function: { name: 'lookup', parameters: {} } },
      ],
      mcp_servers: [
        { name: 'test-server', url: 'https://example.com/mcp', enabled: true },
      ],
      laisky_extra: {
        chat_switch: {
          enable_google_search: true,
          enable_memory: true,
          disable_https_crawler: false,
        },
      },
    }
    await sendChatRequest(request, 'test-token', 'https://example.com/v1')
    expect(fetchMock).toHaveBeenCalledOnce()
    const [url, options] = fetchMock.mock.calls[0]
    expect(url).toMatch(/\/api$/)
    expect(JSON.parse(options.body)).toEqual({ ...request, stream: false })
    expect(options.headers['X-Laisky-Api-Base']).toBe('https://example.com/v1')
  })

  it('keeps provider research models on ordinary streaming transport', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      headers: new Headers(),
      body: {
        getReader: () => ({ read: vi.fn().mockResolvedValue({ done: true }) }),
      },
    })
    vi.stubGlobal('fetch', fetchMock)
    await new Promise<void>((resolve, reject) => {
      sendStreamingChatRequest(
        {
          model: 'o3-deep-research',
          messages: [{ role: 'user', content: 'Research this' }],
        },
        'test-token',
        { onDone: resolve, onError: reject },
      )
    })
    expect(fetchMock).toHaveBeenCalledOnce()
    const [url, options] = fetchMock.mock.calls[0]
    expect(url).toMatch(/\/api$/)
    expect(JSON.parse(options.body)).toMatchObject({
      model: 'o3-deep-research',
      stream: true,
    })
  })

  it('retains image generation and file upload endpoints', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ data: [], cache_keys: [] }),
    })
    vi.stubGlobal('fetch', fetchMock)
    await generateImage(
      { model: 'black-forest-labs/flux-dev', prompt: 'A tree' },
      'test-token',
    )
    await uploadFiles(
      [new File(['retained data'], 'context.txt')],
      'test-token',
    )
    expect(fetchMock.mock.calls[0][0]).toMatch(/\/images\/generations$/)
    expect(fetchMock.mock.calls[1][0]).toMatch(/\/files\/chat$/)
    expect(
      fetchMock.mock.calls.every(
        ([url]) => !String(url).includes('deepresearch'),
      ),
    ).toBe(true)
  })
})
