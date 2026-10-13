/**
 * @vitest-environment jsdom
 * @vitest-environment-options {"url":"https://chat.laisky.com/"}
 */
import { createElement } from 'react'
import * as Sentry from '@sentry/react'
import { render, cleanup } from '@testing-library/react'
import { ErrorBoundary } from '@/components/error-boundary'
import { browserTelemetry } from './browser-sentry'
import { expect, it, vi } from 'vitest'
import { createBrowserTelemetry, type BrowserEnvelope } from './browser-sentry'

it('sends one sanitized offline envelope for each unhandled error/rejection, without duplicate boundary events or extra items', async () => {
  const envelopes: BrowserEnvelope[] = []
  const fetchSpy = vi.spyOn(globalThis, 'fetch')
  const sdk = {
    ...Sentry,
    makeFetchTransport() {
      return {
        send(envelope: BrowserEnvelope) {
          envelopes.push(envelope)
          return Promise.resolve({ statusCode: 200 })
        },
        flush: () => Promise.resolve(true),
      }
    },
  }
  const telemetry = createBrowserTelemetry(sdk)
  const config = {
    enabled: 'true',
    dsn: 'https://0123456789abcdef0123456789abcdef@o1.ingest.sentry.io/1',
    environment: 'test',
    release: 'go-ramjet@abcdef0',
  }
  expect(window.location.hostname).toBe('chat.laisky.com')
  const script = document.createElement('script')
  script.type = 'module'
  script.src = 'https://chat.laisky.com/assets/index-ABCDEFGH.js'
  document.head.append(script)
  expect(telemetry.initialize(config, window.location.hostname)).toBe(true)
  Sentry.setUser({ id: 'private user', email: 'private@example.com' })
  Sentry.setExtra('private prompt', { apiKey: 'secret' })
  Sentry.addBreadcrumb({ message: 'private chat history' })
  const error = new TypeError('private prompt and completion')
  error.stack =
    'TypeError: private\n    at secret (https://chat.laisky.com/assets/index-ABCDEFGH.js?token=secret:12:3)'
  const globals = globalThis as unknown as {
    onerror: (
      message: string,
      url: string,
      line: number,
      column: number,
      error: Error,
    ) => unknown
    onunhandledrejection: (event: { reason: unknown }) => unknown
  }
  globals.onerror(
    'private message',
    'https://chat.laisky.com/assets/index-ABCDEFGH.js?token=secret',
    12,
    3,
    error,
  )
  telemetry.capture(error)
  await Sentry.flush(1000)
  expect(envelopes).toHaveLength(1)
  expect(envelopes[0][1][0][1]).toMatchObject({
    exception: {
      values: [
        {
          stacktrace: {
            frames: [
              {
                filename: 'https://chat.laisky.com/assets/index-ABCDEFGH.js',
                lineno: 12,
                colno: 3,
              },
            ],
          },
        },
      ],
    },
  })
  const rejection = new RangeError('private MCP arguments/results')
  globals.onunhandledrejection({ reason: rejection })
  await Sentry.flush(1000)
  expect(envelopes).toHaveLength(2)
  Sentry.captureEvent(
    {
      exception: {
        values: [{ type: 'Error', value: 'private internal error' }],
      },
      extra: { private: 'secret' },
      request: {
        headers: { Authorization: 'Bearer secret' },
        data: 'private body',
      },
    },
    {
      data: { __sentry__: true },
      attachments: [{ filename: 'private.txt', data: 'secret' }],
    },
  )
  await Sentry.flush(1000)
  expect(envelopes).toHaveLength(3)
  const capture = vi
    .spyOn(browserTelemetry, 'capture')
    .mockImplementation((error) => telemetry.capture(error))
  const log = vi.spyOn(console, 'error').mockImplementation(() => undefined)
  /** BrokenPrimitive deliberately throws a local synthetic primitive for boundary coverage. */
  function BrokenPrimitive(): never {
    throw 'private primitive prompt'
  }
  render(createElement(ErrorBoundary, null, createElement(BrokenPrimitive)))
  await Sentry.flush(1000)
  expect(capture).toHaveBeenCalledTimes(1)
  expect(envelopes).toHaveLength(4)
  cleanup()
  capture.mockRestore()
  log.mockRestore()
  globals.onunhandledrejection({ reason: 'private primitive tool output' })
  await Sentry.flush(1000)
  expect(envelopes).toHaveLength(5)
  for (const envelope of envelopes) {
    expect(envelope[1]).toHaveLength(1)
    expect(envelope[1][0][0].type).toBe('event')
    expect(JSON.stringify(envelope)).not.toMatch(
      /private|secret|apiKey|completion|MCP|token=/,
    )
    expect(envelope[1][0][1]).toMatchObject({
      platform: 'javascript',
      level: 'error',
      environment: 'test',
      release: 'go-ramjet@abcdef0',
      tags: { site: 'chat' },
    })
  }
  expect(fetchSpy).not.toHaveBeenCalled()
  script.remove()
  await Sentry.close()
  fetchSpy.mockRestore()
})
