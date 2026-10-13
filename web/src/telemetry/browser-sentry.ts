import { init, captureException, captureReactException } from '@sentry/react'
import type * as Sentry from '@sentry/react'
import { sanitizeBrowserBreadcrumb, sanitizeBrowserEvent } from './privacy'

const DEFAULT_SDK = {
  init,
  captureException,
  captureReactException,
}
export type BrowserEnvelope = Parameters<
  ReturnType<typeof Sentry.makeFetchTransport>['send']
>[0]

/** BrowserTelemetryConfig contains only explicit public deployment identifiers. */
export interface BrowserTelemetryConfig {
  enabled?: string
  dsn?: string
  environment?: string
  release?: string
}

/** isTargetOrigin limits instrumentation to the requested site. */
function isTargetOrigin(): boolean {
  return (
    typeof window !== 'undefined' &&
    window.location.origin === 'https://chat.laisky.com'
  )
}

/** validConfig validates opt-in and public configuration while keeping disabled builds usable. */
function validConfig(
  config: BrowserTelemetryConfig,
  hostname: string,
): boolean {
  if (
    hostname !== 'chat.laisky.com' ||
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

/** createBrowserTelemetry uses standard SDK capture with targeted privacy and safe UI fallbacks. */
export function createBrowserTelemetry(
  sdk: Pick<
    typeof Sentry,
    'init' | 'captureException' | 'captureReactException'
  > = DEFAULT_SDK,
  transport?: Sentry.BrowserOptions['transport'],
) {
  let attempted = false
  let enabled = false
  const seen = new WeakSet<object>()
  return {
    /** initialize configures the SDK once for an explicitly enabled target deployment. */
    initialize(config: BrowserTelemetryConfig, hostname: string): boolean {
      if (attempted) return enabled && isTargetOrigin()
      if (!isTargetOrigin() || !validConfig(config, hostname)) return false
      attempted = true
      const deployment = Object.freeze({ ...config })
      try {
        const client = sdk.init({
          ...(transport ? { transport } : {}),
          dsn: deployment.dsn,
          environment: deployment.environment,
          release: deployment.release,
          initialScope: { tags: { site: 'chat' } },
          integrations(defaults) {
            // Keep the standard error/breadcrumb integrations; release-health sessions are outside scope.
            return defaults.filter(
              (integration) => integration.name !== 'BrowserSession',
            )
          },
          enableLogs: false,
          tracePropagationTargets: [],
          dataCollection: {
            userInfo: false,
            cookies: false,
            httpHeaders: false,
            httpBodies: [],
            urlQueryParams: false,
            graphQL: { document: false, variables: false },
            genAI: { inputs: false, outputs: false },
            stackFrameVariables: false,
          },
          beforeBreadcrumb(breadcrumb, hint) {
            try {
              return sanitizeBrowserBreadcrumb(breadcrumb, hint)
            } catch {
              return null
            }
          },
          beforeSend(event, hint) {
            hint.attachments = []
            if (!enabled || !isTargetOrigin()) return null
            try {
              const original = hint.originalException
              if (
                (typeof original === 'object' && original !== null) ||
                typeof original === 'function'
              ) {
                if (seen.has(original)) return null
                seen.add(original)
              }
              return sanitizeBrowserEvent(event, hint)
            } catch {
              return null
            }
          },
        })
        enabled = !!client
        if (!enabled)
          console.warn('[Telemetry] Browser error capture unavailable.')
      } catch {
        enabled = false
        console.warn('[Telemetry] Browser error capture unavailable.')
      }
      return enabled
    },
    /** isEnabled reports active capture without changing unrelated site behavior. */
    isEnabled(): boolean {
      return enabled && isTargetOrigin()
    },
    /** capture records one error with React component information when supplied. */
    capture(
      error: unknown,
      info?: { componentStack?: string | null },
      handled = false,
      operation?: string,
    ): void {
      if (!enabled || !isTargetOrigin()) return
      try {
        if (info)
          sdk.captureReactException(error, info, {
            ...(operation ? { captureContext: { tags: { operation } } } : {}),
            mechanism: { type: 'auto.function.react.error_handler', handled },
          })
        else
          sdk.captureException(
            error,
            operation ? { tags: { operation } } : undefined,
          )
      } catch {
        console.warn('[Telemetry] Browser error capture unavailable.')
      }
    },
  }
}

export const browserTelemetry = createBrowserTelemetry()
