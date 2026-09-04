import { beforeEach, describe, expect, it, vi } from 'vitest'

const playerMocks = vi.hoisted(() => ({
  start: vi.fn<() => Promise<void>>(),
  close: vi.fn<() => Promise<void>>(),
  beginResponse: vi.fn(),
  append: vi.fn<() => Promise<void>>(),
  interrupt: vi.fn<() => number>(),
}))

vi.mock('../pcm-audio', () => ({
  PcmAudioPlayer: class {
    start = playerMocks.start
    close = playerMocks.close
    beginResponse = playerMocks.beginResponse
    append = playerMocks.append
    interrupt = playerMocks.interrupt
  },
  pcm16ToBase64: vi.fn(() => ''),
  resampleFloat32ToPCM16: vi.fn(() => new Int16Array()),
}))

import {
  RealtimeAudioClient,
  type RealtimeAudioState,
} from '../realtime-client'

interface Deferred<T> {
  promise: Promise<T>
  resolve: (value: T) => void
  reject: (reason?: unknown) => void
}

/** createDeferred creates a manually settled Promise for lifecycle race tests. */
function createDeferred<T>(): Deferred<T> {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })
  return { promise, resolve, reject }
}

/** createMediaStreamStub creates a microphone stream with an observable track. */
function createMediaStreamStub() {
  const track = {
    enabled: true,
    stop: vi.fn(),
  }
  const stream = {
    getTracks: vi.fn(() => [track]),
    getAudioTracks: vi.fn(() => [track]),
  } as unknown as MediaStream
  return { stream, track }
}

/** createClient creates a Realtime client with observable callbacks. */
function createClient() {
  const states: RealtimeAudioState[] = []
  const onError = vi.fn()
  const client = new RealtimeAudioClient({
    apiBase: 'https://proxy.example.com',
    apiToken: 'test-token',
    instructions: 'Be helpful.',
    callbacks: {
      onStateChange: (state) => states.push(state),
      onTranscriptChange: vi.fn(),
      onError,
    },
  })
  return { client, states, onError }
}

/** flushMicrotasks lets pending async continuations run without advancing timers. */
async function flushMicrotasks(): Promise<void> {
  await Promise.resolve()
  await Promise.resolve()
}

class FakeWebSocket {
  static readonly CONNECTING = 0
  static readonly OPEN = 1
  static readonly CLOSING = 2
  static readonly CLOSED = 3
  static instances: FakeWebSocket[] = []

  readonly url: string
  readonly protocols?: string | string[]
  readyState = FakeWebSocket.CONNECTING
  onopen: ((event: Event) => void) | null = null
  onmessage: ((event: MessageEvent) => void) | null = null
  onerror: ((event: Event) => void) | null = null
  onclose: ((event: CloseEvent) => void) | null = null
  send = vi.fn()
  close = vi.fn((code?: number, reason?: string) => {
    this.readyState = FakeWebSocket.CLOSED
    this.onclose?.({ code: code ?? 1000, reason: reason ?? '' } as CloseEvent)
  })

  /** constructor records a fake socket for explicit test-driven events. */
  constructor(url: string, protocols?: string | string[]) {
    this.url = url
    this.protocols = protocols
    FakeWebSocket.instances.push(this)
  }

  /** open completes the fake WebSocket handshake. */
  open(): void {
    this.readyState = FakeWebSocket.OPEN
    this.onopen?.(new Event('open'))
  }

  /** remoteClose simulates an upstream close frame. */
  remoteClose(code: number, reason = ''): void {
    this.readyState = FakeWebSocket.CLOSED
    this.onclose?.({ code, reason } as CloseEvent)
  }
}

describe('RealtimeAudioClient lifecycle regressions', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
    FakeWebSocket.instances = []
    playerMocks.start.mockReset().mockResolvedValue(undefined)
    playerMocks.close.mockReset().mockResolvedValue(undefined)
    playerMocks.beginResponse.mockReset()
    playerMocks.append.mockReset().mockResolvedValue(undefined)
    playerMocks.interrupt.mockReset().mockReturnValue(0)
  })

  it('disposes a late microphone grant without opening a socket after stop', async () => {
    const permission = createDeferred<MediaStream>()
    const { stream, track } = createMediaStreamStub()
    Object.defineProperty(navigator, 'mediaDevices', {
      configurable: true,
      value: {
        getUserMedia: vi.fn(() => permission.promise),
      },
    })
    vi.stubGlobal('WebSocket', FakeWebSocket)

    const { client } = createClient()
    const startPromise = client.start()
    await flushMicrotasks()

    await client.stop()
    permission.resolve(stream)
    await startPromise.catch(() => undefined)

    expect(track.stop).toHaveBeenCalledTimes(1)
    expect(FakeWebSocket.instances).toHaveLength(0)
  })

  it('releases media after a normal remote close', async () => {
    const { stream, track } = createMediaStreamStub()
    Object.defineProperty(navigator, 'mediaDevices', {
      configurable: true,
      value: {
        getUserMedia: vi.fn().mockResolvedValue(stream),
      },
    })
    vi.stubGlobal('WebSocket', FakeWebSocket)

    const { client, states, onError } = createClient()
    Reflect.set(client, 'openCapture', vi.fn().mockResolvedValue(undefined))
    const startPromise = client.start()
    await flushMicrotasks()
    const socket = FakeWebSocket.instances[0]
    expect(socket).toBeDefined()

    socket.open()
    await startPromise
    socket.remoteClose(1000, 'normal closure')
    await flushMicrotasks()

    expect(track.stop).toHaveBeenCalledTimes(1)
    expect(states.at(-1)).toBe('idle')
    expect(onError).not.toHaveBeenCalled()
  })
})
