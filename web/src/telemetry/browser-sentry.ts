import {
  init,
  captureException,
  globalHandlersIntegration,
  dedupeIntegration,
  makeFetchTransport,
} from '@sentry/react'
import type * as Sentry from '@sentry/react'

const DEFAULT_SDK = {
  init,
  captureException,
  globalHandlersIntegration,
  dedupeIntegration,
  makeFetchTransport,
}

export type BrowserTransport = ReturnType<typeof makeFetchTransport>
export type BrowserEnvelope = Parameters<BrowserTransport['send']>[0]

const HOST = 'chat.laisky.com'
const ORIGIN = 'https://chat.laisky.com'

/** isTargetOrigin permits only HTTPS on the exact target host and normalized default port. */
function isTargetOrigin(): boolean {
  return typeof window !== 'undefined' && window.location.origin === ORIGIN
}
const TYPES = new Set([
  'Error',
  'TypeError',
  'RangeError',
  'ReferenceError',
  'SyntaxError',
  'URIError',
  'EvalError',
  'AggregateError',
])

/** BrowserTelemetryConfig contains only explicit, public deployment identifiers. */
export interface BrowserTelemetryConfig {
  enabled?: string
  dsn?: string
  environment?: string
  release?: string
}

/** safeAssetURL retains only same-origin compiled Vite JavaScript asset URLs. */
function safeAssetURL(value: unknown): string | undefined {
  if (typeof value !== 'string' || value.length > 512) return undefined
  try {
    const url = new URL(value, `https://${HOST}/`)
    if (
      url.origin !== `https://${HOST}` ||
      url.username ||
      url.password ||
      !/^\/assets\/(?:index|vendor-react|vendor-markdown|vendor-hljs|vendor-katex|vendor-pouchdb|vendor-radix|vendor-stripe)-[A-Za-z0-9_-]{8}\.js$/.test(
        url.pathname,
      )
    )
      return undefined
    return `${url.origin}${url.pathname}`
  } catch {
    return undefined
  }
}

/** readBuildAssets snapshots bounded, known module assets loaded by the current build. */
function readBuildAssets(): ReadonlySet<string> {
  const urls = new Set<string>()
  const own = safeAssetURL(import.meta.url)
  if (own) urls.add(own)
  if (typeof document !== 'undefined') {
    for (const node of Array.from(
      document.querySelectorAll(
        'script[type="module"][src], link[rel="modulepreload"][href]',
      ),
    ).slice(0, 40)) {
      const url = safeAssetURL(
        node.getAttribute('src') ?? node.getAttribute('href'),
      )
      if (url) urls.add(url)
    }
  }
  return urls
}

/** safeNumber retains bounded positive source coordinates. */
function safeNumber(value: unknown): number | undefined {
  return typeof value === 'number' &&
    Number.isInteger(value) &&
    value > 0 &&
    value <= 10_000_000
    ? value
    : undefined
}

/** sanitizeBrowserEvent projects error events onto a bounded, data-free schema. */
export function sanitizeBrowserEvent(
  event: Sentry.ErrorEvent,
  config: BrowserTelemetryConfig,
  assets: ReadonlySet<string> = new Set(),
): Sentry.ErrorEvent | null {
  if (event.type || !event.exception?.values?.length) return null
  const values = event.exception.values.slice(0, 3).map((exception) => {
    const frames = exception.stacktrace?.frames?.slice(-30).flatMap((frame) => {
      const filename = safeAssetURL(frame.filename)
      return filename && assets.has(filename)
        ? [
            {
              filename,
              lineno: safeNumber(frame.lineno),
              colno: safeNumber(frame.colno),
              in_app: true,
            },
          ]
        : []
    })
    return {
      type: TYPES.has(exception.type ?? '') ? exception.type : 'Error',
      value: 'Browser error (message withheld)',
      ...(frames?.length ? { stacktrace: { frames } } : {}),
      mechanism: {
        type: 'generic',
        handled: exception.mechanism?.handled === true,
      },
    }
  })
  return {
    ...(typeof event.event_id === 'string' &&
    /^[a-f0-9]{32}$/.test(event.event_id)
      ? { event_id: event.event_id }
      : {}),
    type: undefined,
    platform: 'javascript',
    level: 'error',
    exception: { values },
    environment: config.environment,
    release: config.release,
    tags: { site: 'chat' },
  }
}

/** validConfig verifies opt-in, Sentry ingestion DSN, and bounded deployment identifiers. */
function validConfig(
  config: BrowserTelemetryConfig,
  hostname: string,
): boolean {
  if (
    hostname !== HOST ||
    config.enabled !== 'true' ||
    !config.environment ||
    !/^[A-Za-z0-9_-]{1,32}$/.test(config.environment) ||
    !config.release ||
    !/^go-ramjet@[a-f0-9]{7,40}$/.test(config.release) ||
    !config.dsn ||
    config.dsn.length > 256
  )
    return false
  try {
    const url = new URL(config.dsn)
    return (
      url.protocol === 'https:' &&
      /^o[0-9]+\.ingest(?:\.[a-z]+)?\.sentry\.io$/.test(url.hostname) &&
      /^[a-f0-9]{32}$/.test(url.username) &&
      !url.password &&
      !url.port &&
      /^\/[0-9]+$/.test(url.pathname) &&
      !url.search &&
      !url.hash
    )
  } catch {
    return false
  }
}

/** createPrivateTransport enforces errors-only projection at the final SDK envelope boundary. */
export function createPrivateTransport(
  base: BrowserTransport,
  config: BrowserTelemetryConfig,
  isReady: () => boolean,
  assets: ReadonlySet<string> = new Set(),
): BrowserTransport {
  return {
    /** send forwards only a reconstructed error item and safe event identifier. */
    send(envelope) {
      try {
        if (!isReady() || !isTargetOrigin()) return Promise.resolve({})
        const item = envelope[1]?.[0]
        if (
          item?.[0]?.type !== 'event' ||
          typeof item[1] !== 'object' ||
          item[1] === null
        )
          return Promise.resolve({})
        const event = sanitizeBrowserEvent(
          item[1] as Sentry.ErrorEvent,
          config,
          assets,
        )
        if (!event?.event_id) return Promise.resolve({})
        const safe: BrowserEnvelope = [
          { event_id: event.event_id, sent_at: new Date().toISOString() },
          [[{ type: 'event' }, event]],
        ]
        return Promise.resolve(base.send(safe)).catch(() => {
          console.warn('[Telemetry] Browser error transport unavailable.')
          return {}
        })
      } catch {
        // Drop malformed or unprojectable payloads, including internal SDK errors.
        return Promise.resolve({})
      }
    },
    /** flush waits only for the guarded base transport and contains SDK failure. */
    flush(timeout) {
      try {
        return Promise.resolve(base.flush(timeout)).catch(() => false)
      } catch {
        return Promise.resolve(false)
      }
    },
  }
}

/** createBrowserTelemetry returns idempotent, fail-closed error capture for an SDK instance. */
export function createBrowserTelemetry(
  sdk: Pick<
    typeof Sentry,
    | 'init'
    | 'captureException'
    | 'globalHandlersIntegration'
    | 'dedupeIntegration'
    | 'makeFetchTransport'
  > = DEFAULT_SDK,
) {
  let attempted = false
  let enabled = false
  const seen = new WeakSet<object>()
  return {
    /** initialize enables capture once, only for a valid deployment on the exact target host. */
    initialize(config: BrowserTelemetryConfig, hostname: string): boolean {
      if (attempted) return enabled
      if (!isTargetOrigin() || !validConfig(config, hostname)) return false
      attempted = true
      const deployment = Object.freeze({ ...config })
      try {
        const assets = readBuildAssets()
        const client = sdk.init({
          transport: (options) =>
            createPrivateTransport(
              sdk.makeFetchTransport({
                ...options,
                headers: undefined,
                fetchOptions: {
                  credentials: 'omit',
                  referrerPolicy: 'no-referrer',
                },
              }),
              deployment,
              () => enabled,
              assets,
            ),
          dsn: deployment.dsn,
          environment: deployment.environment,
          release: deployment.release,
          defaultIntegrations: false,
          integrations: [
            sdk.globalHandlersIntegration(),
            sdk.dedupeIntegration(),
          ],
          beforeSendLog: () => null,
          beforeSendMetric: () => null,
          ignoreSpans: [/.*/],
          tracePropagationTargets: [],
          maxBreadcrumbs: 0,
          sendClientReports: false,
          dataCollection: {
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
          },
          beforeBreadcrumb: () => null,
          beforeSend(event, hint) {
            try {
              hint.attachments = []
              if (!enabled || !isTargetOrigin()) return null
              const original = hint.originalException
              if (
                (typeof original === 'object' && original !== null) ||
                typeof original === 'function'
              ) {
                if (seen.has(original)) return null
                seen.add(original)
              }
              return sanitizeBrowserEvent(event, deployment, assets)
            } catch {
              return null
            }
          },
        })
        enabled = !!client
        if (!enabled)
          console.warn('[Telemetry] Browser error capture unavailable.')
      } catch {
        console.warn('[Telemetry] Browser error capture unavailable.')
        enabled = false
      }
      return enabled
    },
    /** isEnabled reports whether initialized capture is active on the exact target host. */
    isEnabled(): boolean {
      return enabled && isTargetOrigin()
    },
    /** capture reports boundary or React root errors while preserving UI behavior on SDK failure. */
    capture(error: unknown): void {
      if (!enabled || !isTargetOrigin()) return
      try {
        sdk.captureException(error)
      } catch {
        console.warn('[Telemetry] Browser error capture unavailable.')
      }
    },
  }
}

export const browserTelemetry = createBrowserTelemetry()
