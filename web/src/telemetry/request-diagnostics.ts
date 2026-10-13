import { browserTelemetry } from './browser-sentry'
import { isExpectedRequestError, markPrivateError } from './privacy'

/** captureUnexpectedRequestFailure records actionable failures while preserving ordinary diagnostic messages. */
export function captureUnexpectedRequestFailure(
  error: unknown,
  operation: string,
): void {
  if (
    isExpectedRequestError(error) ||
    (typeof error === 'object' &&
      error !== null &&
      'name' in error &&
      error.name === 'AbortError')
  )
    return
  const original =
    error instanceof Error
      ? error
      : new Error('Unexpected frontend request failure')
  browserTelemetry.capture(original, undefined, false, operation)
}

/** decodePrivateResponse labels decoding errors that can contain snippets of private response bodies. */
export async function decodePrivateResponse<T>(
  operation: string,
  decode: () => Promise<T>,
): Promise<T> {
  try {
    return await decode()
  } catch (error) {
    if (error instanceof SyntaxError)
      markPrivateError(error, operation + ' response decoding failed')
    throw error
  }
}

/** parsePrivateJSON preserves decoding behavior while marking private stream snippets for telemetry. */
export function parsePrivateJSON<T>(text: string, operation: string): T {
  try {
    return JSON.parse(text) as T
  } catch (error) {
    if (error instanceof SyntaxError)
      markPrivateError(error, operation + ' response decoding failed')
    throw error
  }
}

/** requestWithDiagnostics preserves request/UI behavior and records unexpected frontend failures once. */
export async function requestWithDiagnostics<T>(
  operation: string,
  request: () => Promise<T>,
): Promise<T> {
  try {
    return await request()
  } catch (error) {
    captureUnexpectedRequestFailure(error, operation)
    throw error
  }
}
