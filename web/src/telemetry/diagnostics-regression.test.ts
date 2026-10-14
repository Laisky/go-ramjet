import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import type * as Sentry from '@sentry/react'
import { createBrowserTelemetry } from './browser-sentry'

const config = {
  enabled: 'true',
  dsn: 'https://0123456789abcdef0123456789abcdef@o1.ingest.sentry.io/1',
  environment: 'test',
  release: 'go-ramjet@abcdef0',
}

/** initializedOptions exposes SDK configuration without opening a transport. */
function initializedOptions() {
  const sdk = {
    init: vi.fn((...options: [Sentry.BrowserOptions?]) => ({ options })),
    captureException: vi.fn(),
    captureReactException: vi.fn(),
    consoleIntegration: vi.fn(() => ({ name: 'Console' })),
    globalHandlersIntegration: vi.fn(() => ({ name: 'GlobalHandlers' })),
    dedupeIntegration: vi.fn(() => ({ name: 'Dedupe' })),
  }
  const telemetry = createBrowserTelemetry(
    sdk as unknown as Parameters<typeof createBrowserTelemetry>[0],
  )
  expect(telemetry.initialize(config, 'chat.laisky.com')).toBe(true)
  return sdk.init.mock.calls[0][0] as unknown as Sentry.BrowserOptions
}

beforeEach(() => {
  vi.stubGlobal('window', { location: new URL('https://chat.laisky.com/') })
})
afterEach(() => {
  vi.unstubAllGlobals()
})

it('preserves an ordinary message, lazy stack function, mechanism and debug IDs', () => {
  const options = initializedOptions()
  const frame = {
    filename: 'https://chat.laisky.com/assets/gptchat-LAZY1234.js',
    function: 'renderChatMessage',
    lineno: 19,
    colno: 8,
    debug_id: '12345678-1234-4234-8234-123456789abc',
  }
  const event = {
    exception: {
      values: [
        {
          type: 'TypeError',
          value: "Cannot read properties of undefined (reading 'map')",
          mechanism: { type: 'auto.browser.browserapierrors', handled: false },
          stacktrace: { frames: [frame] },
        },
      ],
    },
    contexts: { react: { componentStack: 'at ChatMessage\nat ChatPage' } },
    debug_meta: {
      images: [
        {
          type: 'sourcemap',
          code_file: frame.filename,
          debug_id: frame.debug_id,
        },
      ],
    },
  } as Sentry.ErrorEvent
  const result = options.beforeSend?.(event, {
    originalException: new TypeError(event.exception?.values?.[0].value),
  })
  expect(result).toMatchObject(event)
})

it('uses standard transport and defaults while retaining bounded breadcrumbs', () => {
  const options = initializedOptions()
  expect(options.transport).toBeUndefined()
  expect(options.defaultIntegrations).toBeUndefined()
  expect(options.maxBreadcrumbs).not.toBe(0)
  expect(
    options.beforeBreadcrumb?.(
      { category: 'navigation', data: { from: '/', to: '/gptchat' } },
      {},
    ),
  ).toMatchObject({
    category: 'navigation',
    data: { from: '/', to: '/gptchat' },
  })
})
