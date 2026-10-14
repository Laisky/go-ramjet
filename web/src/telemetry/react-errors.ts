import type { RootOptions } from 'react-dom/client'
import { browserTelemetry } from './browser-sentry'

/** reportRootError captures uncaught/recoverable React errors and preserves console visibility. */
function reportRootError(
  error: unknown,
  info: { componentStack?: string | null },
): void {
  browserTelemetry.capture(error, info)
  console.error('[React] Browser render error.')
}

/** getReactErrorOptions returns active error hooks, retaining React defaults when capture is disabled. */
export function getReactErrorOptions(): RootOptions | undefined {
  return browserTelemetry.isEnabled()
    ? { onUncaughtError: reportRootError, onRecoverableError: reportRootError }
    : undefined
}
