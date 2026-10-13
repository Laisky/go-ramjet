import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { createPrivateTransport, type BrowserEnvelope } from './browser-sentry'

const config = { environment: 'test', release: 'go-ramjet@abcdef0' }
const event = {
  type: undefined,
  event_id: 'a'.repeat(32),
  exception: { values: [{ type: 'Error', value: 'private prompt' }] },
  user: { email: 'private@example.com' },
  extra: { key: 'secret' },
}

/** envelope builds synthetic SDK envelopes without an external transport. */
function envelope(type: string, payload: unknown = event): BrowserEnvelope {
  return [
    {
      event_id: 'a'.repeat(32),
      sent_at: 'private',
      trace: { release: 'secret' },
    },
    [[{ type }, payload]],
  ] as unknown as BrowserEnvelope
}

beforeEach(() => {
  vi.stubGlobal('window', { location: { hostname: 'chat.laisky.com' } })
  vi.spyOn(console, 'warn').mockImplementation(() => undefined)
})
afterEach(() => {
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

it('reconstructs only the safe error item and drops trace headers and attachments', async () => {
  const base = {
    send: vi.fn(() => Promise.resolve({})),
    flush: vi.fn(() => Promise.resolve(true)),
  }
  const transport = createPrivateTransport(base, config, () => true)
  const input = envelope('event')
  input[1].push([
    { type: 'attachment', length: 6, filename: 'private.txt' },
    'secret',
  ] as never)
  await transport.send(input)
  expect(base.send).toHaveBeenCalledTimes(1)
  const sent = base.send.mock.calls[0][0] as unknown as BrowserEnvelope
  expect(sent[0]).toEqual({
    event_id: 'a'.repeat(32),
    sent_at: expect.stringMatching(/^\d{4}-\d{2}-\d{2}T/),
  })
  expect(sent[1]).toHaveLength(1)
  expect(sent[1][0][0]).toEqual({ type: 'event' })
  expect(JSON.stringify(sent)).not.toMatch(/private|secret|trace|attachment/)
})

it.each([
  'transaction',
  'span',
  'session',
  'sessions',
  'log',
  'metric',
  'replay_event',
  'replay_recording',
  'profile',
  'profile_chunk',
  'attachment',
  'client_report',
  'feedback',
])('blocks %s envelope items', async (type) => {
  const base = {
    send: vi.fn(() => Promise.resolve({})),
    flush: vi.fn(() => Promise.resolve(true)),
  }
  await createPrivateTransport(base, config, () => true).send(envelope(type))
  expect(base.send).not.toHaveBeenCalled()
})

it('drops malformed payloads, invalid event IDs and unready/unsupported-host sends', async () => {
  const base = {
    send: vi.fn(() => Promise.resolve({})),
    flush: vi.fn(() => Promise.resolve(true)),
  }
  const transport = createPrivateTransport(base, config, () => true)
  for (const input of [
    envelope('event', 'private'),
    envelope('event', null),
    envelope('event', { ...event, event_id: 'secret' }),
    envelope('event', { ...event, type: 'transaction' }),
  ])
    await transport.send(input)
  await createPrivateTransport(base, config, () => false).send(
    envelope('event'),
  )
  vi.stubGlobal('window', { location: { hostname: 'cv.laisky.com' } })
  await transport.send(envelope('event'))
  expect(base.send).not.toHaveBeenCalled()
})

it('contains synchronous/asynchronous SDK transport and flush failure', async () => {
  const base = {
    send: vi.fn(() => {
      throw new Error('secret')
    }),
    flush: vi.fn(() => {
      throw new Error('secret')
    }),
  }
  const transport = createPrivateTransport(base, config, () => true)
  await expect(transport.send(envelope('event'))).resolves.toEqual({})
  await expect(transport.flush()).resolves.toBe(false)
  base.send.mockImplementation(
    () => Promise.reject(new Error('private')) as never,
  )
  base.flush.mockImplementation(
    () => Promise.reject(new Error('private')) as never,
  )
  await expect(transport.send(envelope('event'))).resolves.toEqual({})
  await expect(transport.flush()).resolves.toBe(false)
  expect(console.warn).toHaveBeenCalledWith(
    '[Telemetry] Browser error transport unavailable.',
  )
})
