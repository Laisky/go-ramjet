import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type * as Sentry from '@sentry/react'
import { createBrowserTelemetry, sanitizeBrowserEvent } from './browser-sentry'

const config = {
  enabled: 'true',
  dsn: 'https://0123456789abcdef0123456789abcdef@o1.ingest.sentry.io/1',
  environment: 'test',
  release: 'go-ramjet@abcdef0',
}
const assets = new Set(['https://chat.laisky.com/assets/index-ABCDEFGH.js'])
const event: Sentry.ErrorEvent = {
  type: undefined,
  exception: {
    values: [
      {
        type: 'TypeError',
        value: 'private prompt',
        stacktrace: {
          frames: [
            {
              filename:
                'https://chat.laisky.com/assets/index-ABCDEFGH.js?token=secret#private',
              lineno: 12,
              colno: 3,
              function: 'private tool result',
              vars: { token: 'secret' },
              pre_context: ['private completion'],
              context_line: 'secret source',
            },
          ],
        },
      },
    ],
  },
}

/** fixture constructs an SDK double which never creates a network transport. */
function fixture() {
  const sdk = {
    init: vi.fn(() => ({})),
    captureException: vi.fn(),
    globalHandlersIntegration: vi.fn(() => ({ name: 'GlobalHandlers' })),
    dedupeIntegration: vi.fn(() => ({ name: 'Dedupe' })),
  }
  const telemetry = createBrowserTelemetry(
    sdk as unknown as Parameters<typeof createBrowserTelemetry>[0],
  )
  const beforeSend = (input = event, hint: Sentry.EventHint = {}) => {
    const options = sdk.init.mock
      .calls[0]?.[0] as unknown as Sentry.BrowserOptions
    return options.beforeSend?.(input, hint)
  }
  return { sdk, telemetry, beforeSend }
}

beforeEach(() => {
  vi.stubGlobal('window', { location: { hostname: 'chat.laisky.com' } })
  vi.spyOn(console, 'warn').mockImplementation(() => undefined)
})
afterEach(() => {
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

describe('browser privacy projection', () => {
  it('retains useful error type, safe asset coordinates and deployment identity only', () => {
    const result = sanitizeBrowserEvent(event, config, assets)
    expect(result).toEqual({
      type: undefined,
      platform: 'javascript',
      level: 'error',
      environment: 'test',
      release: 'go-ramjet@abcdef0',
      tags: { site: 'chat' },
      exception: {
        values: [
          {
            type: 'TypeError',
            value: 'Browser error (message withheld)',
            mechanism: { type: 'generic', handled: false },
            stacktrace: {
              frames: [
                {
                  filename: 'https://chat.laisky.com/assets/index-ABCDEFGH.js',
                  lineno: 12,
                  colno: 3,
                  in_app: true,
                },
              ],
            },
          },
        ],
      },
    })
  })
  it('drops credentials, bodies, chat history, MCP inputs/results and arbitrary nested metadata', () => {
    const dirty = {
      ...event,
      message: 'private prompt',
      user: { email: 'private@example.com' },
      request: {
        url: 'https://chat.laisky.com/gptchat/private?api_key=secret',
        headers: { Authorization: 'Bearer secret', Cookie: 'secret' },
        data: 'private body',
      },
      breadcrumbs: [{ message: 'private history', data: { token: 'secret' } }],
      extra: {
        localStorage: 'secret',
        toolArguments: 'private',
        toolResult: 'private',
      },
      contexts: { arbitrary: { completion: 'private' } },
      tags: { email: 'private@example.com' },
      debug_meta: {
        images: [
          { type: 'sourcemap', code_file: 'secret', debug_id: 'secret' },
        ],
      },
    }
    expect(
      JSON.stringify(sanitizeBrowserEvent(dirty, config, assets)),
    ).not.toMatch(
      /private|secret|Authorization|Cookie|localStorage|toolArguments|toolResult/,
    )
  })
  it.each([
    'https://cv.laisky.com/assets/index-ABCDEFGH.js',
    'https://chat.laisky.com/gptchat/user-secret/entry.js',
    'https://chat.laisky.com/assets/sk-sensitiveprompt-ABCDEFGH.js',
    'https://chat.laisky.com/assets/private@example.com-ABCDEFGH.js',
    'https://secret@chat.laisky.com/assets/index-ABCDEFGH.js',
    'https://chat.laisky.com/assets/index-secret12.js',
    'data:text/javascript,secret',
    'https://chat.laisky.com/assets/index-ABCDEFGH.js' + 'x'.repeat(600),
  ])('discards unsafe stack URL %s', (filename) => {
    const dirty = {
      type: undefined,
      exception: {
        values: [
          {
            type: 'private custom error',
            value: 'secret',
            stacktrace: { frames: [{ filename }] },
          },
        ],
      },
    } as Sentry.ErrorEvent
    expect(
      sanitizeBrowserEvent(dirty, config, assets)?.exception?.values?.[0],
    ).toEqual({
      type: 'Error',
      value: 'Browser error (message withheld)',
      mechanism: { type: 'generic', handled: false },
    })
  })
  it('bounds chained exceptions and frames and rejects messages/transactions', () => {
    const long = {
      ...event,
      exception: {
        values: Array(100).fill({
          ...event.exception!.values![0],
          stacktrace: {
            frames: Array(1000).fill(
              event.exception!.values![0].stacktrace!.frames![0],
            ),
          },
        }),
      },
    }
    const safe = sanitizeBrowserEvent(long, config, assets)
    expect(safe?.exception?.values).toHaveLength(3)
    expect(safe?.exception?.values?.[0].stacktrace?.frames).toHaveLength(30)
    expect(
      sanitizeBrowserEvent({ type: undefined, message: 'secret' }, config),
    ).toBeNull()
    expect(
      sanitizeBrowserEvent(
        { ...event, type: 'transaction' } as unknown as Sentry.ErrorEvent,
        config,
      ),
    ).toBeNull()
  })
})

describe('fail-closed bootstrap and capture', () => {
  it.each([
    'cv.laisky.com',
    'chat2.laisky.com',
    'localhost',
    'chat.laisky.com.attacker.test',
    'sso.laisky.com',
  ])('never initializes on %s', (host) => {
    const { sdk, telemetry } = fixture()
    expect(telemetry.initialize(config, host)).toBe(false)
    telemetry.capture(new Error('secret'))
    expect(sdk.init).not.toHaveBeenCalled()
    expect(sdk.captureException).not.toHaveBeenCalled()
  })
  it.each([
    {},
    { ...config, enabled: 'false' },
    { ...config, dsn: undefined },
    { ...config, environment: undefined },
    { ...config, release: undefined },
    { ...config, dsn: 'https://secret@attacker.test/1' },
    { ...config, dsn: config.dsn + '?token=secret' },
    { ...config, environment: 'private@example.com' },
    { ...config, release: 'secret release' },
  ])('requires explicit valid deployment configuration', (input) => {
    const { sdk, telemetry } = fixture()
    expect(telemetry.initialize(input, 'chat.laisky.com')).toBe(false)
    expect(sdk.init).not.toHaveBeenCalled()
  })
  it('initializes once, excludes broad integrations, and drops logs/metrics/breadcrumbs', () => {
    const { sdk, telemetry } = fixture()
    expect(telemetry.initialize(config, 'chat.laisky.com')).toBe(true)
    expect(telemetry.initialize(config, 'chat.laisky.com')).toBe(true)
    expect(sdk.init).toHaveBeenCalledTimes(1)
    const options = sdk.init.mock
      .calls[0]?.[0] as unknown as Sentry.BrowserOptions
    expect(options.defaultIntegrations).toBe(false)
    expect(options.integrations).toEqual([
      { name: 'GlobalHandlers' },
      { name: 'Dedupe' },
    ])
    expect(options.dataCollection).toEqual({
      userInfo: false,
      cookies: false,
      httpHeaders: false,
      httpBodies: [],
      urlQueryParams: false,
      graphQL: { document: false, variables: false },
      genAI: { inputs: false, outputs: false },
      databaseQueryData: false,
      queues: false,
      stackFrameVariables: false,
      frameContextLines: 0,
    })
    expect(options.tracePropagationTargets).toEqual([])
    expect(options.tracesSampleRate).toBeUndefined()
    expect(options.beforeBreadcrumb?.({})).toBeNull()
    expect(options.beforeSendLog?.({} as Sentry.Log)).toBeNull()
    expect(options.beforeSendMetric?.({} as Sentry.Metric)).toBeNull()
  })
  it('deduplicates the same boundary/global Error and clears attachments', () => {
    const { telemetry, beforeSend } = fixture()
    telemetry.initialize(config, 'chat.laisky.com')
    const original = new Error('secret')
    const hint = {
      originalException: original,
      attachments: [{ filename: 'private.txt', data: 'secret' }],
    }
    expect(beforeSend(event, hint)).not.toBeNull()
    expect(hint.attachments).toEqual([])
    expect(beforeSend(event, { originalException: original })).toBeNull()
  })
  it('snapshots deployment identity and rechecks exact host at send time', () => {
    const { telemetry, beforeSend } = fixture()
    const mutable = { ...config }
    telemetry.initialize(mutable, 'chat.laisky.com')
    mutable.environment = 'private@example.com'
    mutable.release = 'secret'
    expect(beforeSend()).toMatchObject({
      environment: 'test',
      release: 'go-ramjet@abcdef0',
    })
    vi.stubGlobal('window', { location: { hostname: 'cv.laisky.com' } })
    expect(beforeSend()).toBeNull()
  })
  it.each(['throw', 'undefined'])(
    'drops events after partial init failure: %s',
    (failure) => {
      const { sdk, telemetry, beforeSend } = fixture()
      sdk.init.mockImplementation(() => {
        if (failure === 'throw') throw new Error('secret')
        return undefined as unknown as object
      })
      expect(telemetry.initialize(config, 'chat.laisky.com')).toBe(false)
      expect(beforeSend()).toBeNull()
      telemetry.capture(new Error('secret'))
      expect(sdk.captureException).not.toHaveBeenCalled()
      expect(telemetry.initialize(config, 'chat.laisky.com')).toBe(false)
      expect(sdk.init).toHaveBeenCalledTimes(1)
      expect(console.warn).toHaveBeenCalledWith(
        '[Telemetry] Browser error capture unavailable.',
      )
    },
  )
  it('contains capture and sanitation failures without logging their content', () => {
    const { sdk, telemetry, beforeSend } = fixture()
    telemetry.initialize(config, 'chat.laisky.com')
    sdk.captureException.mockImplementation(() => {
      throw new Error('secret')
    })
    expect(() => telemetry.capture(new Error('secret'))).not.toThrow()
    const hostile = Object.defineProperty({}, 'exception', {
      get() {
        throw new Error('secret')
      },
    })
    expect(beforeSend(hostile as Sentry.ErrorEvent)).toBeNull()
    expect(console.warn).toHaveBeenCalledWith(
      '[Telemetry] Browser error capture unavailable.',
    )
  })
})
