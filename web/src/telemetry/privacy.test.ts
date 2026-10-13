import { expect, it } from 'vitest'
import type { ErrorEvent } from '@sentry/react'
import {
  privateResponseError,
  sanitizeBrowserBreadcrumb,
  sanitizeBrowserEvent,
  sanitizeURL,
} from './privacy'

it('preserves ordinary errors, full lazy stack, mechanism, component context and DebugIDs', () => {
  const frames = Array.from({ length: 150 }, (_, index) => ({
    filename: 'https://chat.laisky.com/assets/gptchat-LAZY1234.js',
    function: 'renderChatMessage',
    lineno: index + 1,
    colno: 8,
    debug_id: '12345678-1234-4234-8234-123456789abc',
  }))
  const event = {
    exception: {
      values: [
        {
          type: 'ApplicationError',
          value: 'Cannot render an empty dataset',
          mechanism: { type: 'react', handled: true },
          stacktrace: { frames },
        },
      ],
    },
    contexts: { react: { componentStack: 'at ChatMessage' } },
    debug_meta: {
      images: [
        {
          type: 'sourcemap',
          code_file: frames[0].filename,
          debug_id: frames[0].debug_id,
        },
      ],
    },
  } as ErrorEvent
  expect(sanitizeBrowserEvent(event)).toEqual(event)
})

it('scrubs marked raw response causes and React copies while preserving the outer message and stack', () => {
  const raw = 'synthetic-private-prompt ' + 'x'.repeat(300)
  const inner = privateResponseError(raw, 'Chat request failed (HTTP 403)')
  const outer = new Error('Chat action failed', { cause: inner })
  inner.cause = new Error(raw)
  const event = {
    exception: {
      values: [
        { type: 'Error', value: outer.message },
        {
          type: 'Error',
          value: raw,
          stacktrace: {
            frames: [
              {
                function: 'sendChatRequest',
                filename: '/assets/gptchat-LAZY1234.js',
              },
            ],
          },
        },
        { type: 'React ErrorBoundary Error', value: raw },
        { type: 'Error', value: 'Error: ' + raw },
        { type: 'Error', value: raw.slice(0, 250) + '...' },
      ],
    },
  } as ErrorEvent
  const safe = sanitizeBrowserEvent(event, { originalException: outer })
  expect(safe.exception?.values?.[0].value).toBe('Chat action failed')
  expect(safe.exception?.values?.slice(1).map((value) => value.value)).toEqual(
    Array(4).fill('Chat request failed (HTTP 403)'),
  )
  expect(safe.exception?.values?.[1].stacktrace?.frames?.[0].function).toBe(
    'sendChatRequest',
  )
  expect(inner.message).toBe(raw)
  expect(JSON.stringify(safe)).not.toContain('synthetic-private-prompt')
})

it('removes explicit user, headers, bodies and BrowserApiErrors arguments while retaining safe metadata', () => {
  const safe = sanitizeBrowserEvent({
    user: { id: 'private-user', email: 'private-user@example.invalid' },
    request: {
      url: 'https://user:pass@chat.laisky.com/gptchat?token=private-query#access_token=private-fragment',
      headers: { Authorization: 'Bearer private-header' },
      cookies: 'private-cookie',
      data: 'private-body',
      method: 'POST',
      query_string: 'token=private-query',
    },
    extra: {
      arguments: [{ detail: 'private-callback-event' }],
      responseBody: 'private-result',
      messages: ['private-history'],
      toolArguments: { value: 'private-mcp' },
      retained: 'safe technical context',
      token_count: 17,
      nested: {
        token: 'private-token',
        access_token: 'private-access',
        refreshToken: 'private-refresh',
        jwt: 'private-jwt',
      },
    },
    exception: { values: [{ type: 'TypeError', value: 'Ordinary failure' }] },
  } as ErrorEvent)
  expect(JSON.stringify(safe)).not.toContain('private-')
  expect(safe.request).toEqual({
    url: 'https://chat.laisky.com/gptchat',
    method: 'POST',
  })
  expect(safe.extra).toMatchObject({
    retained: 'safe technical context',
    token_count: 17,
  })
})

it.each([
  'https://user:pass@host.example/path?token=private#access_token=private',
  '/gptchat?apikey=private&text=private#private',
])('strips userinfo/query/fragment from %s', (url) => {
  expect(sanitizeURL(url)).not.toMatch(/private|user:pass|[?#]/)
  const safe = sanitizeBrowserEvent({
    exception: { values: [{ value: 'Request failed at ' + url }] },
  } as ErrorEvent)
  expect(JSON.stringify(safe)).not.toMatch(/private|user:pass/)
})

it('preserves safe console and method/status/navigation breadcrumbs while stripping sensitive fields', () => {
  expect(
    sanitizeBrowserBreadcrumb(
      {
        category: 'console',
        message: 'ordinary',
        data: { arguments: ['private'] },
      },
      { input: ['Rendering failed', new TypeError('Missing component prop')] },
    ),
  ).toMatchObject({
    message: 'Rendering failed Missing component prop',
    data: {},
  })
  expect(
    sanitizeBrowserBreadcrumb({
      category: 'fetch',
      data: {
        method: 'POST',
        status_code: 503,
        url: 'https://user:pass@host.example/path?api_token=private#private',
      },
    }),
  ).toMatchObject({
    data: {
      method: 'POST',
      status_code: 503,
      url: 'https://host.example/path',
    },
  })
  expect(
    sanitizeBrowserBreadcrumb({
      category: 'navigation',
      data: { from: '/?api_token=private', to: '/gptchat#private' },
    }),
  ).toMatchObject({ data: { from: '/', to: '/gptchat' } })
})

it.each([
  'Non-JSON SSE data:',
  '[fetchTTS] Server error:',
  'Failed to load messages:',
  'Failed to load config:',
  'Failed to save config:',
  'Migration failed:',
])('scrubs audited private console family %s', (message) => {
  const result = sanitizeBrowserBreadcrumb(
    {
      category: 'console',
      message: message + ' private-prompt',
      data: { arguments: ['private-prompt'] },
    },
    { input: [message, new SyntaxError('private-prompt')] },
  )
  expect(result.message?.replace(/:$/, '')).toBe(message.replace(/:$/, ''))
  expect(JSON.stringify(result)).not.toContain('private-prompt')
})

it('strips private DOM attributes and message/session identifiers without dropping the UI action', () => {
  const safe = sanitizeBrowserBreadcrumb({
    category: 'ui.click',
    message:
      'div#chat-message-private-id button[aria-label="private-session"][title="private-mcp"] img[alt="private-filename"]',
  })
  expect(safe.category).toBe('ui.click')
  expect(safe.message).toContain('button')
  expect(JSON.stringify(safe)).not.toContain('private-')
})

it('bounds sanitizer breadth and cycles without limiting normal stacks', () => {
  const many = Object.fromEntries(
    Array.from({ length: 50_000 }, (_, index) => ['field' + index, 'safe']),
  )
  const cyclic: Record<string, unknown> = {}
  cyclic.self = cyclic
  const safe = sanitizeBrowserEvent({ extra: { many, cyclic } } as ErrorEvent)
  expect(JSON.stringify(safe).length).toBeLessThan(250_000)
  expect(JSON.stringify(safe)).toContain('_truncated')
})

it('scrubs nested payload fields and explicit credential text while preserving token counts', () => {
  const event = sanitizeBrowserEvent({
    exception: {
      values: [
        {
          value:
            'token=synthetic-private-a access_token=synthetic-private-b refreshToken=synthetic-private-c jwt=synthetic-private-d authorization=synthetic-private-e cookie=synthetic-private-f Basic c3ludGhldGljLXByaXZhdGU= token_count=17',
        },
      ],
    },
    extra: {
      nested: {
        rawResponse: 'synthetic-private-response',
        requestBody: 'synthetic-private-request',
        responseBody: 'synthetic-private-response',
        body: 'synthetic-private-body',
        toolArguments: ['synthetic-private-arguments'],
        toolResults: ['synthetic-private-results'],
        token_count: 17,
      },
    },
  } as ErrorEvent)
  expect(JSON.stringify(event)).not.toContain('synthetic-private')
  expect(JSON.stringify(event)).not.toContain('c3ludGhldGljLXByaXZhdGU')
  expect(JSON.stringify(event)).toContain('token_count=17')
  expect(event.extra?.nested).toMatchObject({ token_count: 17 })
  const breadcrumb = sanitizeBrowserBreadcrumb({
    category: 'fetch',
    data: { method: 'GET', nested: { body: 'synthetic-private-body' } },
  })
  expect(JSON.stringify(breadcrumb)).not.toContain('synthetic-private')
})

it('summarizes private log families even when the first formatted argument already contains private text', () => {
  for (const prefix of [
    'Non-JSON SSE data:',
    '[fetchTTS] Server error:',
    'Failed to load messages',
    'Failed to save config',
  ]) {
    const breadcrumb = sanitizeBrowserBreadcrumb(
      { category: 'console' },
      { input: [prefix + ' synthetic-private-body'] },
    )
    expect(JSON.stringify(breadcrumb)).not.toContain('synthetic-private')
  }
})
