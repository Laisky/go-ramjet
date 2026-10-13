import { afterEach, expect, it, vi } from 'vitest'
import { browserTelemetry } from './browser-sentry'
import { privateResponseError, sanitizeBrowserEvent } from './privacy'
import {
  captureUnexpectedRequestFailure,
  decodePrivateResponse,
  parsePrivateJSON,
  requestWithDiagnostics,
} from './request-diagnostics'
vi.mock('./browser-sentry', () => ({ browserTelemetry: { capture: vi.fn() } }))
afterEach(() => vi.clearAllMocks())

it.each([400, 401, 403, 422, 429, 500])(
  'keeps ordinary HTTP %s errors in the UI without creating events',
  async (status) => {
    const original = privateResponseError(
      'private response body',
      'API request failed (HTTP ' + status + ')',
    )
    await expect(
      requestWithDiagnostics('Chat request', async () => {
        throw original
      }),
    ).rejects.toBe(original)
    expect(browserTelemetry.capture).not.toHaveBeenCalled()
  },
)

it('keeps cancellation out of events', () => {
  captureUnexpectedRequestFailure(
    new DOMException('Cancelled', 'AbortError'),
    'Chat request',
  )
  expect(browserTelemetry.capture).not.toHaveBeenCalled()
})

it('captures one legitimate unexpected transport failure without erasing its diagnostic message', async () => {
  const error = new TypeError('Failed to fetch')
  await expect(
    requestWithDiagnostics('Chat request', async () => {
      throw error
    }),
  ).rejects.toBe(error)
  expect(browserTelemetry.capture).toHaveBeenCalledExactlyOnceWith(
    error,
    undefined,
    false,
    'Chat request',
  )
  expect(error.message).toBe('Failed to fetch')
})

it('preserves an ordinary programmer error inside a request', async () => {
  const error = new TypeError(
    "Cannot read properties of undefined (reading 'map')",
  )
  await expect(
    requestWithDiagnostics('Chat request', async () => {
      throw error
    }),
  ).rejects.toBe(error)
  expect(
    sanitizeBrowserEvent(
      { exception: { values: [{ value: error.message }] } },
      { originalException: error },
    ).exception?.values?.[0].value,
  ).toBe(error.message)
})

it('marks only decoding errors at a private response origin and preserves UI behavior', async () => {
  const error = new SyntaxError('Unexpected token in private-chat-response')
  await expect(
    requestWithDiagnostics('Chat request', () =>
      decodePrivateResponse('Chat request', async () => {
        throw error
      }),
    ),
  ).rejects.toBe(error)
  expect(browserTelemetry.capture).toHaveBeenCalledExactlyOnceWith(
    error,
    undefined,
    false,
    'Chat request',
  )
  const safe = sanitizeBrowserEvent(
    { exception: { values: [{ value: error.message }] } },
    { originalException: error },
  )
  expect(safe.exception?.values?.[0].value).toBe(
    'Chat request response decoding failed',
  )
  expect(error.message).toContain('private-chat-response')
})

it('marks malformed private stream JSON while retaining the SyntaxError stack', () => {
  try {
    parsePrivateJSON('private-stream-output', 'Chat stream')
  } catch (error) {
    expect(error).toBeInstanceOf(SyntaxError)
    const safe = sanitizeBrowserEvent(
      {
        exception: {
          values: [{ type: 'SyntaxError', value: (error as Error).message }],
        },
      },
      { originalException: error },
    )
    expect(safe.exception?.values?.[0].value).toBe(
      'Chat stream response decoding failed',
    )
  }
})
