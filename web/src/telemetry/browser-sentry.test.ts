import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type * as Sentry from '@sentry/react'
import { createBrowserTelemetry } from './browser-sentry'

const config = {
  enabled: 'true',
  dsn: 'https://0123456789abcdef0123456789abcdef@o1.ingest.sentry.io/1',
  environment: 'test',
  release: 'go-ramjet@abcdef0',
}

/** fixture creates an SDK double without transports or external events. */
function fixture() {
  const sdk = {
    init: vi.fn((...options: [Sentry.BrowserOptions?]) => ({ options })),
    captureException: vi.fn(),
    captureReactException: vi.fn(),
    consoleIntegration: vi.fn(() => ({ name: 'Console' })),
  }
  const telemetry = createBrowserTelemetry(
    sdk as unknown as Parameters<typeof createBrowserTelemetry>[0],
  )
  return { sdk, telemetry, options: () => sdk.init.mock.calls[0][0]! }
}

beforeEach(() => {
  vi.stubGlobal('window', { location: new URL('https://chat.laisky.com/') })
  vi.spyOn(console, 'warn').mockImplementation(() => undefined)
})
afterEach(() => {
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

describe('deployment and SDK behavior', () => {
  it.each(['cv.laisky.com', 'chat2.laisky.com', 'localhost'])(
    'keeps unsupported host %s disabled',
    (hostname) => {
      vi.stubGlobal('window', {
        location: new URL('https://' + hostname + '/'),
      })
      const { sdk, telemetry } = fixture()
      expect(telemetry.initialize(config, hostname)).toBe(false)
      telemetry.capture(new Error('Ordinary failure'))
      expect(sdk.init).not.toHaveBeenCalled()
      expect(sdk.captureException).not.toHaveBeenCalled()
    },
  )
  it.each([
    { ...config, enabled: undefined },
    { ...config, dsn: undefined },
    { ...config, dsn: 'https://user:pass@example.com/1' },
    { ...config, environment: 'invalid value' },
    { ...config, release: 'bad' },
  ])('keeps missing or invalid configuration disabled', (value) => {
    const { sdk, telemetry } = fixture()
    expect(telemetry.initialize(value, 'chat.laisky.com')).toBe(false)
    expect(sdk.init).not.toHaveBeenCalled()
  })
  it.each(['http://chat.laisky.com/', 'https://chat.laisky.com:8443/'])(
    'refuses origin %s',
    (url) => {
      vi.stubGlobal('window', { location: new URL(url) })
      const { sdk, telemetry } = fixture()
      expect(telemetry.initialize(config, 'chat.laisky.com')).toBe(false)
      expect(sdk.init).not.toHaveBeenCalled()
    },
  )
  it.each(['https://chat.laisky.com/', 'https://chat.laisky.com:443/'])(
    'accepts normalized HTTPS origin %s',
    (url) => {
      vi.stubGlobal('window', { location: new URL(url) })
      const { telemetry } = fixture()
      expect(telemetry.initialize(config, 'chat.laisky.com')).toBe(true)
    },
  )
  it('initializes once and preserves standard defaults except health sessions', () => {
    const { sdk, telemetry, options } = fixture()
    expect(telemetry.initialize(config, 'chat.laisky.com')).toBe(true)
    expect(telemetry.initialize(config, 'chat.laisky.com')).toBe(true)
    expect(sdk.init).toHaveBeenCalledTimes(1)
    expect(options().transport).toBeUndefined()
    expect(options().defaultIntegrations).toBeUndefined()
    const defaults = [
      { name: 'Console' },
      { name: 'BrowserSession' },
      { name: 'LinkedErrors' },
      { name: 'BrowserApiErrors' },
    ]
    const integrations = options().integrations as (
      input: typeof defaults,
    ) => typeof defaults
    expect(integrations(defaults).map((value) => value.name)).toEqual([
      'Console',
      'LinkedErrors',
      'BrowserApiErrors',
    ])
    expect(options().dataCollection).toMatchObject({
      httpHeaders: false,
      httpBodies: [],
      cookies: false,
      urlQueryParams: false,
      graphQL: { document: false, variables: false },
      genAI: { inputs: false, outputs: false },
    })
  })
  it('snapshots public identifiers, deduplicates one object and clears attachments', () => {
    const { telemetry, options } = fixture()
    const value = { ...config }
    telemetry.initialize(value, 'chat.laisky.com')
    value.environment = 'changed'
    expect(options().environment).toBe('test')
    const error = new Error('Ordinary failure')
    const hint: Sentry.EventHint = {
      originalException: error,
      attachments: [{ filename: 'private.txt', data: 'private-content' }],
    }
    const event = {
      exception: { values: [{ type: 'Error', value: error.message }] },
    } as Sentry.ErrorEvent
    expect(options().beforeSend?.(event, hint)).toMatchObject(event)
    expect(hint.attachments).toEqual([])
    expect(options().beforeSend?.(event, hint)).toBeNull()
    vi.stubGlobal('window', { location: new URL('https://cv.laisky.com/') })
    expect(telemetry.isEnabled()).toBe(false)
    expect(options().beforeSend?.(event, {})).toBeNull()
  })
  it.each(['throw', 'undefined'])(
    'contains init %s and blocks partially installed callbacks',
    (mode) => {
      const { sdk, telemetry, options } = fixture()
      sdk.init.mockImplementation(() => {
        if (mode === 'throw') throw new Error('synthetic-private-config')
        return undefined as unknown as object
      })
      expect(telemetry.initialize(config, 'chat.laisky.com')).toBe(false)
      expect(
        options().beforeSend?.(
          { exception: { values: [{ value: 'Ordinary failure' }] } },
          {},
        ),
      ).toBeNull()
      telemetry.capture(new Error('Ordinary failure'))
      expect(sdk.captureException).not.toHaveBeenCalled()
      expect(console.warn).toHaveBeenCalledWith(
        '[Telemetry] Browser error capture unavailable.',
      )
    },
  )
  it('contains SDK capture failures and forwards React component info', () => {
    const { sdk, telemetry } = fixture()
    telemetry.initialize(config, 'chat.laisky.com')
    const error = new TypeError('Ordinary failure')
    const info = { componentStack: 'at ChatMessage\nat ChatPage' }
    telemetry.capture(error, info, true)
    expect(sdk.captureReactException).toHaveBeenCalledWith(error, info, {
      mechanism: { type: 'auto.function.react.error_handler', handled: true },
    })
    sdk.captureException.mockImplementation(() => {
      throw new Error('synthetic-private-sdk-value')
    })
    expect(() => telemetry.capture(error)).not.toThrow()
    expect(console.warn).toHaveBeenCalledWith(
      '[Telemetry] Browser error capture unavailable.',
    )
  })
})

it('drops only malformed breadcrumbs without interrupting logging or ordinary breadcrumbs', () => {
  const { telemetry, options } = fixture()
  telemetry.initialize(config, 'chat.laisky.com')
  const malformed = Object.defineProperty({}, 'message', {
    enumerable: true,
    get() {
      throw new Error('synthetic-private-getter')
    },
  })
  expect(() => options().beforeBreadcrumb?.(malformed, {})).not.toThrow()
  expect(options().beforeBreadcrumb?.(malformed, {})).toBeNull()
  expect(
    options().beforeBreadcrumb?.(
      { category: 'console', message: 'Ordinary safe diagnostic' },
      {},
    ),
  ).toMatchObject({ message: 'Ordinary safe diagnostic' })
})
