import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/telemetry/request-diagnostics', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/telemetry/request-diagnostics')>()),
  captureUnexpectedRequestFailure: vi.fn(),
}))

import { captureUnexpectedRequestFailure } from '@/telemetry/request-diagnostics'
import { sendStreamingChatRequest } from '../api'

/** Frames of one ordinary streamed answer, as the Go backend writes them. */
const answerFrames = [
  'data: {"id":"req-1","model":"m","choices":[{"delta":{"role":"assistant","content":"Hello"}}]}\n\n',
  'data: {"id":"req-1","model":"m","choices":[{"delta":{"content":" world"}}]}\n\n',
  'data: {"id":"req-1","model":"m","choices":[{"delta":{},"finish_reason":"stop"}]}\n\n',
  'data: [DONE]\n\n',
]

/** The periodic keep-alive the backend emits: an SSE comment frame. */
const heartbeatFrame = ': heartbeat\n\n'

/**
 * streamEvents feeds chunks through the real streaming client and returns every
 * callback it fired, in order, once the stream is done.
 */
async function streamEvents(chunks: string[]): Promise<string[]> {
  const encoder = new TextEncoder()
  const queue = chunks.map((chunk) => encoder.encode(chunk))
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue({
      ok: true,
      headers: new Headers(),
      body: {
        getReader: () => ({
          read: async () =>
            queue.length > 0
              ? { done: false, value: queue.shift() }
              : { done: true, value: undefined },
        }),
      },
    }),
  )

  const events: string[] = []
  await new Promise<void>((resolve, reject) => {
    sendStreamingChatRequest(
      { model: 'm', messages: [{ role: 'user', content: 'hi' }] },
      'test-token',
      {
        onContent: (content) => events.push('content:' + content),
        onReasoning: (content) => events.push('reasoning:' + content),
        onFinish: (reason) => events.push('finish:' + reason),
        onResponseInfo: (info) => events.push('info:' + info.id),
        onDone: () => {
          events.push('done')
          resolve()
        },
        onError: reject,
      },
    )
  })
  return events
}

/** splitAt cuts text into two chunks at offset, simulating TCP chunking. */
function splitAt(text: string, offset: number): string[] {
  return [text.slice(0, offset), text.slice(offset)]
}

describe('chat stream heartbeats', () => {
  beforeEach(() => {
    vi.mocked(captureUnexpectedRequestFailure).mockClear()
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('ignores interleaved comment heartbeats without extra events or telemetry', async () => {
    const baseline = await streamEvents(answerFrames)
    expect(baseline).toContain('content:Hello')
    expect(baseline.at(-1)).toBe('done')
    expect(captureUnexpectedRequestFailure).not.toHaveBeenCalled()

    const withHeartbeats = [
      heartbeatFrame,
      answerFrames[0],
      ...splitAt(heartbeatFrame, 4),
      ...splitAt(answerFrames[1] + heartbeatFrame, 20),
      heartbeatFrame + answerFrames[2],
      heartbeatFrame,
      answerFrames[3],
    ]
    expect(await streamEvents(withHeartbeats)).toEqual(baseline)
    expect(captureUnexpectedRequestFailure).not.toHaveBeenCalled()
  })

  it('reports legacy data heartbeats as protocol failures', async () => {
    // This is why periodic heartbeats must not use the data: [HEARTBEAT] form.
    const legacy = [
      answerFrames[0],
      'data: [HEARTBEAT]\n\n',
      ...answerFrames.slice(1),
    ]
    await streamEvents(legacy)
    expect(captureUnexpectedRequestFailure).toHaveBeenCalledTimes(1)
  })
})
