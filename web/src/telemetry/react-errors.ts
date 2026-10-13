import { browserTelemetry } from './browser-sentry'

/** reportRootError captures uncaught/recoverable React errors and preserves console visibility. */
function reportRootError(error: unknown): void {
  browserTelemetry.capture(error)
  console.error('[React] Browser render error.')
}

export const reactErrorOptions = {
  onUncaughtError: reportRootError,
  onRecoverableError: reportRootError,
}
