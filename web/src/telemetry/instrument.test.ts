import { afterEach, expect, it, vi } from 'vitest'

const telemetry = vi.hoisted(() => ({ initialize: vi.fn() }))
vi.mock('./browser-sentry', () => ({ browserTelemetry: telemetry }))

afterEach(() => {
  vi.unstubAllGlobals()
  vi.clearAllMocks()
  vi.resetModules()
})

it.each([
  'http://chat.laisky.com/',
  'https://chat.laisky.com:8443/',
  'https://cv.laisky.com/',
  'https://chat2.laisky.com/',
])('keeps the bootstrap outside target origin %s disabled', async (url) => {
  vi.stubGlobal('window', { location: new URL(url) })
  await import('./instrument')
  expect(telemetry.initialize).not.toHaveBeenCalled()
})

it.each(['https://chat.laisky.com/', 'https://chat.laisky.com:443/'])(
  'initializes only the normalized target origin %s',
  async (url) => {
    vi.stubGlobal('window', { location: new URL(url) })
    await import('./instrument')
    expect(telemetry.initialize).toHaveBeenCalledTimes(1)
    expect(telemetry.initialize.mock.calls[0][1]).toBe('chat.laisky.com')
  },
)
