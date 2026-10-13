/**
 * @vitest-environment jsdom
 * @vitest-environment-options {"url":"https://chat.laisky.com/"}
 */
import { createElement } from 'react'
import * as Sentry from '@sentry/react'
import { render, cleanup } from '@testing-library/react'
import { ErrorBoundary } from '@/components/error-boundary'
import {
  browserTelemetry,
  createBrowserTelemetry,
  type BrowserEnvelope,
} from './browser-sentry'
import { privateResponseError } from './privacy'
import { sendChatRequest } from '@/utils/api'
import { afterEach, expect, it, vi } from 'vitest'

afterEach(async () => {
  cleanup()
  await Sentry.close()
  Sentry.setUser(null)
  Sentry.setExtra('access_token', undefined)
  Sentry.setExtra('arguments', undefined)
  Sentry.getCurrentScope().clearBreadcrumbs()
  Sentry.getCurrentScope().clearAttachments()
  vi.restoreAllMocks()
})

it('emits useful standard SDK envelopes with targeted private cause, URL, scope and breadcrumb cleanup', async () => {
  const envelopes: BrowserEnvelope[] = []
  const fetchSpy = vi.spyOn(globalThis, 'fetch')
  const telemetry = createBrowserTelemetry(Sentry, () => ({
    send(envelope) {
      envelopes.push(envelope)
      return Promise.resolve({ statusCode: 200 })
    },
    flush: () => Promise.resolve(true),
  }))
  expect(
    telemetry.initialize(
      {
        enabled: 'true',
        dsn: 'https://0123456789abcdef0123456789abcdef@o1.ingest.sentry.io/1',
        environment: 'test',
        release: 'go-ramjet@abcdef0',
      },
      window.location.hostname,
    ),
  ).toBe(true)

  Sentry.logger.info('synthetic-private-log')
  Sentry.setUser({
    id: 'synthetic-private-user',
    email: 'synthetic-private-email',
  })
  Sentry.setExtra('access_token', 'synthetic-private-token')
  Sentry.setExtra('arguments', [
    new CustomEvent('callback', { detail: 'synthetic-private-callback' }),
  ])
  Sentry.addBreadcrumb({
    category: 'navigation',
    data: {
      from: '/',
      to: '/gptchat?apikey=synthetic-private-query#synthetic-private-fragment',
    },
  })
  console.debug('Non-JSON SSE data:', 'synthetic-private-stream')
  console.error(
    'Failed to load messages:',
    new SyntaxError('synthetic-private-history'),
  )
  Sentry.addBreadcrumb({
    category: 'ui.click',
    message:
      'div#chat-message-synthetic-private-id img[alt="synthetic-private-file"]',
  })
  Sentry.addBreadcrumb({
    category: 'fetch',
    data: {
      method: 'GET',
      status_code: 503,
      url: 'https://user:pass@host.example/path?token=synthetic-private-query#synthetic-private-fragment',
    },
  })
  Sentry.addBreadcrumb({
    category: 'console',
    message: 'Ordinary safe console diagnostic',
  })

  const raw = 'synthetic-private-response ' + 'x'.repeat(300)
  const inner = privateResponseError(raw, 'Chat request failed (HTTP 403)')
  inner.stack =
    'Error: private response\n at sendChatRequest (https://chat.laisky.com/assets/gptchat-LAZY1234.js?token=synthetic-private-query:12:3)'
  const outer = new TypeError(
    "Cannot read properties of undefined (reading 'map')",
    { cause: inner },
  )
  outer.stack =
    'TypeError: ordinary\n at renderChatMessage (https://chat.laisky.com/assets/gptchat-LAZY1234.js:19:8)'
  telemetry.capture(
    outer,
    {
      componentStack:
        '\n    at ChatMessage (https://chat.laisky.com/assets/gptchat-LAZY1234.js:20:4)',
    },
    true,
  )
  telemetry.capture(outer)
  await Sentry.flush(1000)
  const events = () =>
    envelopes.flatMap((envelope) =>
      envelope[1]
        .filter((item) => item[0].type === 'event')
        .map((item) => item[1] as Sentry.ErrorEvent),
    )
  expect(events()).toHaveLength(1)
  expect(events()[0].tags?.site).toBe('chat')
  const first = events()[0]
  expect(
    first.exception?.values?.some((value) => value.value === outer.message),
  ).toBe(true)
  expect(
    first.exception?.values?.some(
      (value) => value.value === 'Chat request failed (HTTP 403)',
    ),
  ).toBe(true)
  expect(
    first.exception?.values?.some((value) =>
      value.type?.startsWith('React ErrorBoundary'),
    ),
  ).toBe(true)
  expect(
    first.exception?.values
      ?.flatMap((value) => value.stacktrace?.frames ?? [])
      .some((frame) => frame.function?.includes('renderChatMessage')),
  ).toBe(true)
  expect(
    first.breadcrumbs?.some(
      (breadcrumb) => breadcrumb.category === 'navigation',
    ),
  ).toBe(true)
  expect(
    first.breadcrumbs?.some(
      (breadcrumb) => breadcrumb.message === 'Ordinary safe console diagnostic',
    ),
  ).toBe(true)

  const debugId = '12345678-1234-4234-8234-123456789abc'
  const frame = {
    filename: 'https://chat.laisky.com/assets/gptchat-LAZY1234.js',
    function: 'decodeResponse',
    lineno: 8,
    colno: 3,
    debug_id: debugId,
  }
  Sentry.captureEvent(
    {
      exception: {
        values: [
          {
            type: 'ProtocolError',
            value: 'Unexpected frontend protocol state',
            stacktrace: { frames: [frame] },
          },
        ],
      },
      debug_meta: {
        images: [
          {
            type: 'sourcemap',
            code_file: frame.filename,
            debug_id: frame.debug_id,
          },
        ],
      },
      request: {
        headers: { Authorization: 'Bearer synthetic-private-header' },
        data: 'synthetic-private-body',
      },
    },
    {
      attachments: [
        { filename: 'private-debug.txt', data: 'synthetic-private-attachment' },
      ],
    },
  )
  await Sentry.flush(1000)
  expect(events()).toHaveLength(2)
  expect(events()[1].debug_meta).toMatchObject({
    images: [{ debug_id: debugId }],
  })
  expect(
    events()[1].exception?.values?.[0].stacktrace?.frames?.[0],
  ).toMatchObject({
    filename: frame.filename,
    function: frame.function,
    lineno: 8,
    colno: 3,
  })
  expect(
    envelopes
      .flatMap((envelope) => envelope[1])
      .some((item) => item[0].type === 'attachment'),
  ).toBe(false)

  const delegate = vi
    .spyOn(browserTelemetry, 'capture')
    .mockImplementation((error, info, handled, operation) =>
      telemetry.capture(error, info, handled, operation),
    )
  /** BrokenChat deliberately exercises the mounted boundary's React component stack. */
  function BrokenChat(): never {
    throw new Error('Ordinary render failure')
  }
  render(createElement(ErrorBoundary, null, createElement(BrokenChat)))
  await Sentry.flush(1000)
  expect(delegate).toHaveBeenCalledTimes(1)
  expect(events()).toHaveLength(3)
  expect(
    events()[2].exception?.values?.some((value) =>
      value.type?.startsWith('React ErrorBoundary'),
    ),
  ).toBe(true)
  cleanup()

  const globals = globalThis as unknown as {
    onunhandledrejection: (event: { reason: unknown }) => unknown
  }
  globals.onunhandledrejection({
    reason: new RangeError('Ordinary unhandled promise failure'),
  })
  await Sentry.flush(1000)
  expect(events()).toHaveLength(4)
  expect(events()[3].exception?.values?.[0].value).toBe(
    'Ordinary unhandled promise failure',
  )
  expect(
    envelopes
      .flatMap((envelope) => envelope[1])
      .every((item) => item[0].type === 'event'),
  ).toBe(true)
  expect(JSON.stringify(envelopes)).not.toContain('synthetic-private')
  expect(fetchSpy).not.toHaveBeenCalled()
})

it('keeps expected HTTP responses in UI and emits one actionable transport/decode failure through the actual request path', async () => {
  const envelopes: BrowserEnvelope[] = []
  const telemetry = createBrowserTelemetry(Sentry, () => ({
    send(envelope) {
      envelopes.push(envelope)
      return Promise.resolve({ statusCode: 200 })
    },
    flush: () => Promise.resolve(true),
  }))
  telemetry.initialize(
    {
      enabled: 'true',
      dsn: 'https://0123456789abcdef0123456789abcdef@o1.ingest.sentry.io/1',
      environment: 'test',
      release: 'go-ramjet@abcdef0',
    },
    'chat.laisky.com',
  )
  vi.spyOn(browserTelemetry, 'capture').mockImplementation(
    (error, info, handled, operation) =>
      telemetry.capture(error, info, handled, operation),
  )
  const fetchMock = vi.spyOn(globalThis, 'fetch')
  fetchMock.mockResolvedValue(
    new Response('synthetic-private-body', { status: 401 }),
  )
  await expect(
    sendChatRequest({ messages: [], model: 'fixture' }, 'fixture-token'),
  ).rejects.toThrow('synthetic-private-body')
  await Sentry.flush(1000)
  const events = () =>
    envelopes.flatMap((envelope) =>
      envelope[1]
        .filter((item) => item[0].type === 'event')
        .map((item) => item[1] as Sentry.ErrorEvent),
    )
  expect(events()).toHaveLength(0)
  fetchMock.mockRejectedValue(new TypeError('Failed to fetch'))
  await expect(
    sendChatRequest({ messages: [], model: 'fixture' }, 'fixture-token'),
  ).rejects.toThrow('Failed to fetch')
  await Sentry.flush(1000)
  expect(events()).toHaveLength(1)
  expect(events()[0].exception?.values?.[0].value).toBe('Failed to fetch')
  fetchMock.mockResolvedValue(
    new Response('synthetic-private-body', {
      status: 200,
      headers: { 'Content-Type': 'application/json' },
    }),
  )
  await expect(
    sendChatRequest({ messages: [], model: 'fixture' }, 'fixture-token'),
  ).rejects.toBeInstanceOf(SyntaxError)
  await Sentry.flush(1000)
  expect(events()).toHaveLength(2)
  expect(events()[1].exception?.values?.[0].value).toBe(
    'Chat completion response decoding failed',
  )
  expect(JSON.stringify(envelopes)).not.toContain('synthetic-private')
})
