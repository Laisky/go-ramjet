import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  RealtimeAudioClient,
  MAX_REALTIME_BUFFERED_BYTES,
} from '../realtime-client'
import { pcm16ToBase64, PcmAudioPlayer } from '../pcm-audio'
import {
  deferred,
  FakeContext,
  FakeSocket,
  FakeWorklet,
  flush,
  installMedia,
} from './media-fixtures'

let media: ReturnType<typeof installMedia>
let clients: RealtimeAudioClient[] = []
/** makeClient constructs a real client with observable outward callbacks. */
function makeClient() {
  const callbacks = {
    onStateChange: vi.fn(),
    onTranscriptChange: vi.fn(),
    onError: vi.fn(),
    onEnded: vi.fn(),
  }
  const client = new RealtimeAudioClient({
    apiBase: 'https://gateway.example.com',
    apiToken: 'test-token',
    instructions: 'Be helpful.',
    callbacks,
  })
  clients.push(client)
  return { client, callbacks }
}
/** connect completes a browser handshake and a distinct server configuration acknowledgement. */
async function connect() {
  const result = makeClient()
  const started = result.client.start()
  await flush()
  const socket = FakeSocket.instances.at(-1)!
  socket.open()
  socket.event({ type: 'session.updated' })
  await started
  return { ...result, socket, context: FakeContext.instances.at(-1)! }
}
/** response begins one ordinary model response and delivers a short native-audio delta. */
function response(socket: FakeSocket, id = 'r1') {
  socket.event({ type: 'response.created', response: { id } })
  socket.event({
    type: 'response.output_audio.delta',
    response_id: id,
    item_id: `item-${id}`,
    content_index: 0,
    delta: pcm16ToBase64(new Int16Array(2400)),
  })
}
/** complete marks generation complete; it does not finish audible playback. */
function complete(socket: FakeSocket, id = 'r1', output: unknown[] = []) {
  socket.event({
    type: 'response.done',
    response: { id, status: 'completed', output },
  })
}
const endCall = {
  type: 'function_call',
  name: 'end_call',
  call_id: 'call-1',
  arguments: '{"reason":"Goodbye"}',
}

beforeEach(() => {
  media = installMedia()
  clients = []
})
afterEach(async () => {
  await Promise.all(clients.map((client) => client.stop()))
  vi.useRealTimers()
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

describe('Realtime call lifecycle', () => {
  it('settles cancelled startup promptly and stops a late permission grant without opening a socket', async () => {
    const permission = deferred<MediaStream>()
    media.getUserMedia.mockReturnValue(permission.promise)
    const { client, callbacks } = makeClient()
    const result = client.start().catch((error: Error) => error.name)
    await client.stop()
    expect(await result).toBe('AbortError')
    permission.resolve(media.stream)
    await flush()
    expect(media.track.stop).toHaveBeenCalledTimes(1)
    expect(FakeSocket.instances).toHaveLength(0)
    expect(callbacks.onEnded).toHaveBeenCalledTimes(1)
  })

  it('waits for session.updated before attaching microphone capture', async () => {
    const { client, callbacks } = makeClient()
    const result = client.start()
    await flush()
    const socket = FakeSocket.instances[0]
    socket.open()
    await flush()
    expect(FakeWorklet.instances).toHaveLength(0)
    expect(callbacks.onStateChange).not.toHaveBeenCalledWith('listening')
    expect(socket.protocols).not.toContain('openai-beta.realtime-v1')
    socket.event({ type: 'session.updated' })
    await result
    expect(FakeWorklet.instances).toHaveLength(1)
  })

  it('does not create a capture graph when cancelled during worklet module loading', async () => {
    const module = deferred<void>()
    FakeContext.moduleReady = module.promise
    const { client } = makeClient()
    const result = client.start().catch((error: Error) => error.name)
    await flush()
    const socket = FakeSocket.instances[0]
    socket.open()
    socket.event({ type: 'session.updated' })
    await flush()
    expect(
      FakeContext.instances[0].audioWorklet.addModule,
    ).toHaveBeenCalledTimes(1)
    await client.stop()
    expect(await result).toBe('AbortError')
    module.resolve()
    await flush()
    expect(FakeWorklet.instances).toHaveLength(0)
    expect(media.track.stop).toHaveBeenCalledTimes(1)
  })

  it('does not reopen audio after cancellation while resume is pending', async () => {
    const resume = deferred<void>()
    FakeContext.initialState = 'suspended'
    FakeContext.resumeReady = resume.promise
    const { client } = makeClient()
    const result = client.start().catch((error: Error) => error.name)
    await flush()
    await client.stop()
    expect(await result).toBe('AbortError')
    resume.resolve()
    await flush()
    expect(FakeContext.instances).toHaveLength(1)
    expect(FakeContext.instances[0].state).toBe('closed')
    expect(FakeSocket.instances).toHaveLength(0)
  })

  it('cancels an outstanding WebSocket handshake and rejects duplicate starts', async () => {
    const { client } = makeClient()
    const result = client.start().catch((error: Error) => error.name)
    await expect(client.start()).rejects.toThrow('new client')
    await flush()
    await client.stop()
    expect(await result).toBe('AbortError')
    expect(media.getUserMedia).toHaveBeenCalledTimes(1)
    expect(FakeSocket.instances[0].close).toHaveBeenCalledTimes(1)
  })

  it.each([1000, 1006])(
    'cleans every resource after remote closure %i',
    async (code) => {
      const { socket, client, context, callbacks } = await connect()
      socket.remoteClose(code)
      await flush()
      await client.stop()
      expect(media.track.stop).toHaveBeenCalledTimes(1)
      expect(FakeWorklet.instances[0].port.close).toHaveBeenCalledTimes(1)
      expect(context.close).toHaveBeenCalledTimes(1)
      expect(callbacks.onEnded).toHaveBeenCalledTimes(1)
      expect(callbacks.onStateChange).toHaveBeenLastCalledWith('idle')
      expect(callbacks.onError).toHaveBeenCalledTimes(code === 1000 ? 0 : 1)
    },
  )

  it('times out a server that never acknowledges configuration', async () => {
    vi.useFakeTimers()
    const { client } = makeClient()
    const result = client.start().catch((error: Error) => error.message)
    await flush()
    FakeSocket.instances[0].open()
    await vi.advanceTimersByTimeAsync(20_001)
    expect(await result).toContain('timed out')
    expect(media.track.stop).toHaveBeenCalledTimes(1)
  })
})

describe('Continuous native audio behavior', () => {
  it('keeps one connection across multiple turns and silence; response.done waits for playback', async () => {
    const { socket, context, callbacks } = await connect()
    for (const id of ['r1', 'r2', 'r3']) {
      response(socket, id)
      complete(socket, id)
      await flush()
      expect(callbacks.onStateChange).toHaveBeenLastCalledWith('speaking')
      context.sources.at(-1)!.finish()
      await flush()
      expect(callbacks.onStateChange).toHaveBeenLastCalledWith('listening')
    }
    expect(FakeSocket.instances).toHaveLength(1)
    expect(socket.close).not.toHaveBeenCalled()
    expect(media.track.stop).not.toHaveBeenCalled()
    expect(callbacks.onEnded).not.toHaveBeenCalled()
  })

  it('gates microphone frames during mute and resumes on the same transport', async () => {
    const { socket, client } = await connect()
    const worklet = FakeWorklet.instances[0]
    worklet.frame()
    client.setMuted(true)
    worklet.frame()
    expect(media.track.enabled).toBe(false)
    expect(
      socket.sent.filter((event) => event.type === 'input_audio_buffer.append'),
    ).toHaveLength(1)
    client.setMuted(false)
    worklet.frame()
    expect(media.track.enabled).toBe(true)
    expect(
      socket.sent.filter((event) => event.type === 'input_audio_buffer.append'),
    ).toHaveLength(2)
    expect(socket.close).not.toHaveBeenCalled()
  })

  it('bounds backpressure with an explicit connection error, not an unbounded delayed-audio queue', async () => {
    const { socket, callbacks } = await connect()
    socket.bufferedAmount = MAX_REALTIME_BUFFERED_BYTES
    FakeWorklet.instances[0].frame()
    socket.bufferedAmount += 1
    FakeWorklet.instances[0].frame()
    await flush()
    expect(
      socket.sent.filter((event) => event.type === 'input_audio_buffer.append'),
    ).toHaveLength(1)
    expect(callbacks.onError).toHaveBeenCalledWith(
      expect.stringContaining('too slow'),
    )
    expect(media.track.stop).toHaveBeenCalledTimes(1)
  })

  it('truncates at zero before playback begins and rejects queued or late audio after barge-in', async () => {
    const { socket, context } = await connect()
    response(socket)
    socket.event({ type: 'input_audio_buffer.speech_started' })
    socket.event({
      type: 'response.output_audio.delta',
      response_id: 'r1',
      item_id: 'item-r1',
      delta: 'AAA=',
    })
    await flush()
    expect(context.sources).toHaveLength(0)
    expect(socket.sent).toContainEqual({
      type: 'conversation.item.truncate',
      item_id: 'item-r1',
      content_index: 0,
      audio_end_ms: 0,
    })
    response(socket, 'r2')
    await flush()
    expect(context.sources).toHaveLength(1)
  })

  it('ignores an old response completion after a new response starts', async () => {
    const { socket, callbacks } = await connect()
    response(socket, 'old')
    socket.event({ type: 'input_audio_buffer.speech_started' })
    socket.event({ type: 'response.created', response: { id: 'new' } })
    complete(socket, 'old', [endCall])
    await flush()
    expect(callbacks.onStateChange).toHaveBeenLastCalledWith('thinking')
    expect(callbacks.onEnded).not.toHaveBeenCalled()
  })

  it('waits for audible goodbye, acknowledges one completed end_call and ends exactly once', async () => {
    const { socket, context, callbacks } = await connect()
    response(socket)
    socket.event({ ...endCall, type: 'response.function_call_arguments.done' })
    expect(socket.close).not.toHaveBeenCalled()
    complete(socket, 'r1', [endCall])
    complete(socket, 'r1', [endCall])
    await flush()
    expect(socket.close).not.toHaveBeenCalled()
    expect(callbacks.onStateChange).toHaveBeenLastCalledWith('ending')
    expect(
      socket.sent.filter((event) => event.type === 'conversation.item.create'),
    ).toHaveLength(1)
    context.sources[0].finish()
    await flush()
    expect(callbacks.onEnded).toHaveBeenCalledExactlyOnceWith(
      'assistant',
      'Goodbye',
    )
    expect(media.track.stop).toHaveBeenCalledTimes(1)
  })

  it('lets user hang-up override pending goodbye playback without a second end callback', async () => {
    const { socket, client, callbacks } = await connect()
    response(socket)
    complete(socket, 'r1', [endCall])
    await flush()
    await client.stop('user')
    await flush()
    expect(callbacks.onEnded).toHaveBeenCalledExactlyOnceWith('user', undefined)
    expect(socket.close).toHaveBeenCalledTimes(1)
  })

  it.each(['{}', '{"reason":42}', 'not-json'])(
    'rejects malformed end_call arguments %s',
    async (argumentsJSON) => {
      const { socket, callbacks } = await connect()
      complete(socket, 'r1', [{ ...endCall, arguments: argumentsJSON }])
      await flush()
      expect(callbacks.onEnded).not.toHaveBeenCalled()
      expect(socket.close).not.toHaveBeenCalled()
    },
  )
})

describe('PCM playback ownership', () => {
  it('excludes output gaps from heard audio and refuses to reopen after close', async () => {
    const player = new PcmAudioPlayer()
    await player.start()
    const context = FakeContext.instances[0]
    const second = pcm16ToBase64(new Int16Array(24_000))
    await player.append(second)
    context.currentTime = 2
    await player.append(second)
    context.currentTime = 2.5
    expect(player.interrupt()).toBe(1500)
    await player.close()
    await expect(player.append(second)).rejects.toThrow('unavailable')
    await expect(player.start()).rejects.toMatchObject({ name: 'AbortError' })
    expect(FakeContext.instances).toHaveLength(1)
  })
})
