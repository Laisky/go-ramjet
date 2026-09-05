import { vi } from 'vitest'

/** deferred exposes a Promise's settlement for deterministic cancellation tests. */
export function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((yes, no) => {
    resolve = yes
    reject = no
  })
  return { promise, resolve, reject }
}

/** flush lets asynchronous media continuations run without advancing time. */
export async function flush(): Promise<void> {
  for (let i = 0; i < 12; i++) await Promise.resolve()
}

/** FakeSocket exposes actual browser event boundaries rather than private client methods. */
export class FakeSocket {
  static readonly CONNECTING = 0
  static readonly OPEN = 1
  static readonly CLOSING = 2
  static readonly CLOSED = 3
  static instances: FakeSocket[] = []
  readyState = 0
  bufferedAmount = 0
  url: string
  protocols: string[]
  onopen: (() => void) | null = null
  onmessage: ((event: { data: string }) => void) | null = null
  onclose: ((event: { code: number; reason: string }) => void) | null = null
  onerror: (() => void) | null = null
  sent: Record<string, unknown>[] = []
  send = vi.fn((data: string) => this.sent.push(JSON.parse(data)))
  close = vi.fn(() => {
    this.readyState = FakeSocket.CLOSED
  })
  /** constructor records the URL and authentication protocols for assertions. */
  constructor(url: string, protocols: string[]) {
    this.url = url
    this.protocols = protocols
    FakeSocket.instances.push(this)
  }
  /** open completes only the network handshake, not the Realtime configuration handshake. */
  open(): void {
    this.readyState = FakeSocket.OPEN
    this.onopen?.()
  }
  /** event delivers an ordinary JSON server event. */
  event(data: unknown): void {
    this.onmessage?.({ data: JSON.stringify(data) })
  }
  /** remoteClose simulates either a normal or abnormal provider-initiated closure. */
  remoteClose(code = 1000): void {
    this.readyState = FakeSocket.CLOSED
    this.onclose?.({ code, reason: '' })
  }
}

/** FakeSource models audio scheduling and explicit audible completion. */
export class FakeSource {
  buffer: { duration: number } | null = null
  onended: (() => void) | null = null
  start = vi.fn()
  stop = vi.fn()
  connect = vi.fn()
  disconnect = vi.fn()
  /** finish models the browser's ended event after scheduled playback completes. */
  finish(): void {
    this.onended?.()
  }
}

/** FakeContext implements the Web Audio boundaries used by the production player. */
export class FakeContext {
  static instances: FakeContext[] = []
  static moduleReady: Promise<void> = Promise.resolve()
  static resumeReady: Promise<void> = Promise.resolve()
  static initialState = 'running'
  state = FakeContext.initialState
  currentTime = 0
  sampleRate = 24_000
  destination = {}
  sources: FakeSource[] = []
  microphone = { connect: vi.fn(), disconnect: vi.fn() }
  audioWorklet = { addModule: vi.fn(() => FakeContext.moduleReady) }
  resume = vi.fn(async () => {
    await FakeContext.resumeReady
    if (this.state !== 'closed') this.state = 'running'
  })
  close = vi.fn(async () => {
    this.state = 'closed'
  })
  /** constructor records each context to detect leaked or reopened playback. */
  constructor() {
    FakeContext.instances.push(this)
  }
  /** createBuffer returns a real-duration buffer with observable PCM writes. */
  createBuffer(_channels: number, length: number, rate: number) {
    return { duration: length / rate, copyToChannel: vi.fn() }
  }
  /** createBufferSource records every scheduled output node. */
  createBufferSource() {
    const source = new FakeSource()
    this.sources.push(source)
    return source
  }
  /** createMediaStreamSource exposes microphone graph connections. */
  createMediaStreamSource() {
    return this.microphone
  }
}

/** FakeWorklet models transferable PCM frame delivery without ScriptProcessor support. */
export class FakeWorklet {
  static instances: FakeWorklet[] = []
  port = {
    onmessage: null as ((event: { data: ArrayBuffer }) => void) | null,
    close: vi.fn(),
  }
  connect = vi.fn()
  disconnect = vi.fn()
  /** constructor records the worklet created by production capture setup. */
  constructor() {
    FakeWorklet.instances.push(this)
  }
  /** frame sends microphone PCM using the same message boundary as an AudioWorklet. */
  frame(): void {
    this.port.onmessage?.({ data: new Int16Array([1234, -1234]).buffer })
  }
}

/** installMedia installs fresh browser fakes and returns the observed microphone track. */
export function installMedia() {
  FakeSocket.instances = []
  FakeContext.instances = []
  FakeContext.initialState = 'running'
  FakeContext.moduleReady = Promise.resolve()
  FakeContext.resumeReady = Promise.resolve()
  FakeWorklet.instances = []
  const track = {
    enabled: true,
    onended: null as (() => void) | null,
    stop: vi.fn(),
  }
  const stream = {
    getTracks: () => [track],
    getAudioTracks: () => [track],
  } as unknown as MediaStream
  const getUserMedia = vi.fn().mockResolvedValue(stream)
  vi.stubGlobal('WebSocket', FakeSocket)
  vi.stubGlobal('AudioContext', FakeContext)
  vi.stubGlobal('AudioWorkletNode', FakeWorklet)
  Object.defineProperty(navigator, 'mediaDevices', {
    configurable: true,
    value: { getUserMedia },
  })
  return { track, stream, getUserMedia }
}
