import type { Breadcrumb, EventHint, ErrorEvent } from '@sentry/react'

const privateErrors = new WeakMap<
  object,
  { message: string; expected: boolean }
>()
const sensitiveKey =
  /authorization|cookie|password|passwd|api.?key|api.?token|secret|sso.?token|session.?token|^(?:token|jwt|access.?token|refresh.?token|id.?token)$|^(?:rawResponse|requestBody|responseBody|body|toolArguments|toolResults)$|^prompt$|^completion$|^messages$|^history$|chat.?history|tool.?arguments|tool.?results/i

/** privateResponseError keeps the UI response text while marking its telemetry-safe summary. */
export function privateResponseError(message: string, summary: string): Error {
  return markPrivateError(new Error(message), summary, true)
}

/** markPrivateError labels private-origin errors without changing their UI message or stack. */
export function markPrivateError<T extends Error>(
  error: T,
  message: string,
  expected = false,
): T {
  privateErrors.set(error, { message, expected })
  return error
}

/** isExpectedRequestError identifies ordinary server responses that should remain UI errors. */
export function isExpectedRequestError(error: unknown): boolean {
  return error instanceof Error && privateErrors.get(error)?.expected === true
}

/** sanitizeURL removes credentials, queries and fragments without restricting diagnostic asset paths. */
export function sanitizeURL(value: string): string {
  try {
    const url = new URL(value, 'https://chat.laisky.com/')
    url.username = ''
    url.password = ''
    url.search = ''
    url.hash = ''
    return value.startsWith('/') ? url.pathname : url.href
  } catch {
    return value.split(/[?#]/, 1)[0]
  }
}

/** redactText masks recognizable credentials while preserving ordinary diagnostic messages. */
function redactText(value: string): string {
  return value
    .replace(/\[(\d{3})\]:[\s\S]*/g, '[$1]: [Filtered response body]')
    .replace(
      /TTS request failed \[(\d{3})\]:[\s\S]*/g,
      'TTS request failed [$1]: [Filtered response body]',
    )
    .replace(/https?:\/\/[^\s"'<>]+/g, (url) => sanitizeURL(url))
    .replace(/\b(Bearer|Basic)\s+[A-Za-z0-9._~+/-]+=*/gi, '$1 [Filtered]')
    .replace(/\bsk-(?:proj-)?[A-Za-z0-9_-]{8,}/g, '[Filtered]')
    .replace(
      /\b(api[_-]?key|api[_-]?token|sso[_-]?token|password|secret|token|access[_-]?token|refresh[_-]?token|jwt|authorization|cookie)\s*[=:]\s*[^\s,;]+/gi,
      '$1=[Filtered]',
    )
}

/** privateMessages finds marked original errors and causes, including React's copied error message. */
function privateMessages(original: unknown): Map<string, string> {
  const messages = new Map<string, string>()
  const seen = new WeakSet<object>()
  for (let depth = 0; original instanceof Error && depth < 10; depth++) {
    if (seen.has(original)) break
    seen.add(original)
    const marked = privateErrors.get(original)
    if (marked) {
      messages.set(original.message, marked.message)
      messages.set(original.name + ': ' + original.message, marked.message)
    }
    original = original.cause
  }
  return messages
}

/** redactFields removes private payload fields while retaining SDK diagnostic structure. */
function redactFields(
  value: unknown,
  messages: Map<string, string>,
  key = '',
  depth = 0,
  budget = { remaining: 10_000 },
): unknown {
  if (--budget.remaining < 0) return '[Truncated]'
  if (sensitiveKey.test(key)) return '[Filtered]'
  if (typeof value === 'string') {
    let text = messages.get(value) ?? value
    for (const [raw, summary] of messages) {
      if (raw && text.includes(raw)) text = text.replaceAll(raw, summary)
      // SDK maxValueLength truncates exception values before beforeSend.
      if (raw.length > 250 && text.endsWith('...')) {
        const prefix = raw.slice(0, 24)
        const index = text.indexOf(prefix)
        if (index >= 0 && raw.startsWith(text.slice(index, -3)))
          text = text.slice(0, index) + summary
      }
    }
    if (/^(url|filename|abs_path|code_file|from|to)$/i.test(key))
      return sanitizeURL(text)
    return redactText(text)
  }
  if (!value || typeof value !== 'object') return value
  if (depth >= 10) return '[Truncated]'
  if (Array.isArray(value)) {
    const items: unknown[] = []
    for (const item of value) {
      if (budget.remaining <= 0) {
        items.push('[Truncated]')
        break
      }
      items.push(redactFields(item, messages, '', depth + 1, budget))
    }
    return items
  }
  const result: Record<string, unknown> = {}
  for (const childKey in value) {
    if (!Object.prototype.hasOwnProperty.call(value, childKey)) continue
    if (budget.remaining <= 0) {
      result._truncated = true
      break
    }
    const child = (value as Record<string, unknown>)[childKey]
    if (childKey === 'vars') continue
    result[childKey] = redactFields(
      child,
      messages,
      childKey,
      depth + 1,
      budget,
    )
  }
  return result
}

/** sanitizeBrowserEvent targets private data without projecting away messages, stacks or DebugIDs. */
export function sanitizeBrowserEvent(
  event: ErrorEvent,
  hint: EventHint = {},
): ErrorEvent {
  const result = redactFields(
    event,
    privateMessages(hint.originalException),
  ) as ErrorEvent
  // This app does not set a telemetry user. Preserve technical contexts and tags.
  delete result.user
  if (result.request) {
    delete result.request.headers
    delete result.request.cookies
    delete result.request.data
    delete result.request.query_string
  }
  if (result.extra) {
    for (const key of [
      'arguments',
      'requestBody',
      'responseBody',
      'body',
      'rawResponse',
      'toolArguments',
      'toolResults',
    ])
      delete result.extra[key]
  }
  return result
}

/** consoleMessage retains console diagnostics while excluding audited private payload arguments. */
function consoleMessage(
  breadcrumb: Breadcrumb,
  input: unknown,
): string | undefined {
  const args = Array.isArray(input) ? input : []
  const first = typeof args[0] === 'string' ? args[0] : ''
  if (
    /^(Non-JSON SSE data:|\[fetchTTS\] Server error:|\[fetchTTS\] Requesting TTS audio:)/.test(
      first,
    )
  )
    return redactText(
      first.match(
        /^(Non-JSON SSE data:|\[fetchTTS\] Server error:|\[fetchTTS\] Requesting TTS audio:)/,
      )?.[0] ?? 'Chat diagnostic',
    )
  if (
    /^\[(useConfig|useChatStorage|useChatScroll|useChat|migration)\]/.test(
      first,
    )
  )
    return redactText(
      first.match(/^\[[^\]]+\]\s+[A-Za-z]+/)?.[0] ?? 'Chat state update',
    )
  if (
    /^Failed to (load session config for key|read history for|delete orphan chat data)/.test(
      first,
    )
  )
    return redactText(first.split(/ for | data /, 1)[0])
  if (
    /^(Failed to (load messages|load config|save config|export session)|Migration failed:|Migration copy phase failed)/.test(
      first,
    )
  )
    return redactText(
      first.match(
        /^(Failed to (?:load messages|load config|save config|export session)|Migration failed:|Migration copy phase failed)/,
      )?.[0] ?? 'Chat state diagnostic',
    )
  if (/^Skipping legacy row /.test(first))
    return 'Skipping legacy row with malformed JSON'
  if (/^\[MCP\] /.test(first)) return 'MCP tool synchronization'
  if (args.length) {
    return args
      .map((argument) => {
        if (argument instanceof Error) {
          return (
            privateErrors.get(argument)?.message ?? redactText(argument.message)
          )
        }
        return typeof argument === 'string'
          ? redactText(argument)
          : typeof argument === 'object'
            ? '[Object]'
            : String(argument)
      })
      .join(' ')
  }
  return breadcrumb.message ? redactText(breadcrumb.message) : undefined
}

/** sanitizeBrowserBreadcrumb preserves navigation, request status and UI actions without chat content. */
export function sanitizeBrowserBreadcrumb(
  breadcrumb: Breadcrumb,
  hint?: { input?: unknown },
): Breadcrumb {
  const result = redactFields(breadcrumb, new Map()) as Breadcrumb
  if (result.category === 'console') {
    result.message = consoleMessage(breadcrumb, hint?.input)
    if (result.data) delete result.data.arguments
  }
  if (result.category?.startsWith('ui.') && result.message) {
    result.message = result.message
      .replace(/\[(?:aria-label|alt|title)="[^"]*"\]/g, '')
      .replace(/#(?:chat-message|search-result)-[A-Za-z0-9_-]+/g, '')
  }
  return result
}
