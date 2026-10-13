import { act, cleanup, render, screen } from '@testing-library/react'
import { flushSync } from 'react-dom'
import { createRoot } from 'react-dom/client'
import { BrowserRouter, Route, Routes } from 'react-router-dom'
import { afterEach, expect, it, vi } from 'vitest'
import { ErrorBoundary } from '@/components/error-boundary'
import { browserTelemetry } from './browser-sentry'
import { getReactErrorOptions } from './react-errors'

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

/** BrokenPage deliberately throws a synthetic local error to exercise React behavior. */
function BrokenPage(): never {
  throw new Error('Synthetic render failure')
}

it('captures a routed render failure once and preserves the existing fallback', () => {
  const capture = vi
    .spyOn(browserTelemetry, 'capture')
    .mockImplementation(() => undefined)
  vi.spyOn(console, 'error').mockImplementation(() => undefined)
  render(
    <ErrorBoundary>
      <BrowserRouter>
        <Routes>
          <Route path="*" element={<BrokenPage />} />
        </Routes>
      </BrowserRouter>
    </ErrorBoundary>,
  )
  expect(screen.getByText('Something went wrong.')).toBeInTheDocument()
  expect(capture).toHaveBeenCalledTimes(1)
  expect(capture.mock.calls[0][0]).toBeInstanceOf(Error)
})

it('captures uncaught React root errors through the actual root hook', async () => {
  const capture = vi
    .spyOn(browserTelemetry, 'capture')
    .mockImplementation(() => undefined)
  vi.spyOn(console, 'error').mockImplementation(() => undefined)
  const container = document.createElement('div')
  vi.spyOn(browserTelemetry, 'isEnabled').mockReturnValue(true)
  const root = createRoot(container, getReactErrorOptions())
  flushSync(() => {
    root.render(<BrokenPage />)
  })
  await vi.waitFor(() => expect(capture).toHaveBeenCalledTimes(1))
  expect(capture).toHaveBeenCalledTimes(1)
  await act(async () => {
    root.unmount()
  })
})

it('routes recoverable React errors through safe capture and fixed diagnostics', () => {
  const capture = vi
    .spyOn(browserTelemetry, 'capture')
    .mockImplementation(() => undefined)
  const log = vi.spyOn(console, 'error').mockImplementation(() => undefined)
  const error = new Error('private prompt')
  vi.spyOn(browserTelemetry, 'isEnabled').mockReturnValue(true)
  getReactErrorOptions()?.onRecoverableError?.(error, { componentStack: '' })
  expect(capture).toHaveBeenCalledWith(error)
  expect(log).toHaveBeenCalledWith('[React] Browser render error.')
})

it('keeps boundary fallback usable with telemetry disabled', () => {
  vi.spyOn(console, 'error').mockImplementation(() => undefined)
  render(
    <ErrorBoundary>
      <BrokenPage />
    </ErrorBoundary>,
  )
  expect(screen.getByRole('button', { name: 'Try again' })).toBeInTheDocument()
})

it('keeps default global error dispatch when capture is disabled or unsupported', async () => {
  const capture = vi
    .spyOn(browserTelemetry, 'capture')
    .mockImplementation(() => undefined)
  vi.spyOn(console, 'error').mockImplementation(() => undefined)
  expect(getReactErrorOptions()).toBeUndefined()
  const reported = vi.fn((event: ErrorEvent) => event.preventDefault())
  window.addEventListener('error', reported)
  const root = createRoot(document.createElement('div'), getReactErrorOptions())
  flushSync(() => {
    root.render(<BrokenPage />)
  })
  await vi.waitFor(() => expect(reported).toHaveBeenCalledTimes(1))
  expect(capture).not.toHaveBeenCalled()
  window.removeEventListener('error', reported)
  await act(async () => {
    root.unmount()
  })
})
