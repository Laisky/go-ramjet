/**
 * Alert shown when a link asked to move a session that holds the user's own
 * API key to a different API endpoint.
 */
import { Globe, ShieldAlert } from 'lucide-react'
import { useState } from 'react'

import { Button } from '@/components/ui/button'
import type { SessionConfig } from '../types'
import { maskSecretHint } from '../utils/api-token'

/**
 * describeEndpoint returns the part of apiBase a user recognizes: its origin
 * and path when it parses as a URL, and the trimmed text otherwise.
 */
function describeEndpoint(apiBase: string): string {
  try {
    const url = new URL(apiBase)
    return `${url.origin}${url.pathname.replace(/\/+$/, '')}`
  } catch {
    return apiBase.trim()
  }
}

/**
 * ApiEndpointChangeAlertProps configures ApiEndpointChangeAlert.
 * requestedBase is the endpoint the link asked for, currentBase the one in
 * use, and keyHint the masked stored key that would move with it. onConfirm
 * switches the endpoint, onDismiss keeps the current one, and busy disables
 * both while a choice is being stored.
 */
export interface ApiEndpointChangeAlertProps {
  requestedBase: string
  currentBase: string
  keyHint: string
  onConfirm: () => void
  onDismiss: () => void
  busy?: boolean
}

/**
 * ApiEndpointChangeAlert asks the user whether a link may move this
 * session, and the API key it holds, to another endpoint. Keeping the
 * current endpoint is the prominent choice. It returns an alert pinned below
 * the page header that does not block the rest of the page.
 */
export function ApiEndpointChangeAlert({
  requestedBase,
  currentBase,
  keyHint,
  onConfirm,
  onDismiss,
  busy = false,
}: ApiEndpointChangeAlertProps) {
  const requested = describeEndpoint(requestedBase)
  return (
    <div className="pointer-events-none fixed left-12 right-0 top-12 z-30 px-2 pt-2 sm:px-4">
      <div
        role="alert"
        aria-labelledby="api-endpoint-change-alert-title"
        className="pointer-events-auto mx-auto flex max-w-3xl items-start gap-3 rounded-lg border-2 border-destructive bg-destructive p-4 text-destructive-foreground shadow-lg"
      >
        <ShieldAlert className="mt-0.5 h-5 w-5 shrink-0" aria-hidden />
        <div className="min-w-0 flex-1 space-y-2 text-sm">
          <p
            id="api-endpoint-change-alert-title"
            className="text-base font-semibold"
          >
            Switch this session to another API endpoint?
          </p>
          <p>
            The link you opened asks to send this session&apos;s requests to{' '}
            <code className="break-all font-mono">{requested}</code>. Your saved
            API key (<code className="font-mono">{keyHint}</code>) would be sent
            there too, and whoever runs that address could use it. Switch only
            if you trust it.
          </p>
          <p>
            Current endpoint:{' '}
            <code className="break-all font-mono">
              {describeEndpoint(currentBase)}
            </code>
          </p>
          <div className="flex flex-wrap gap-2 pt-1">
            <Button
              size="sm"
              variant="outline"
              className="border-destructive-foreground bg-destructive-foreground text-destructive hover:bg-destructive-foreground/90"
              onClick={onDismiss}
              disabled={busy}
            >
              Keep current endpoint
            </Button>
            <Button
              size="sm"
              variant="ghost"
              className="h-auto gap-1.5 py-1 text-left text-destructive-foreground hover:bg-destructive-foreground/15"
              onClick={onConfirm}
              disabled={busy}
            >
              <Globe className="h-4 w-4 shrink-0" aria-hidden />
              <span>
                Switch to{' '}
                <code className="break-all font-mono">{requested}</code>
              </span>
            </Button>
          </div>
        </div>
      </div>
    </div>
  )
}

/**
 * ApiEndpointChangeBannerProps configures ApiEndpointChangeBanner:
 * requestedBase is the pending endpoint, config the active session, and
 * onConfirm / onDismiss resolve the prompt.
 */
export interface ApiEndpointChangeBannerProps {
  requestedBase: string
  config: Pick<SessionConfig, 'api_base' | 'api_token'>
  onConfirm: () => Promise<void>
  onDismiss: () => void
}

/**
 * ApiEndpointChangeBanner shows ApiEndpointChangeAlert for a pending
 * link-supplied endpoint and stores the user's choice. A failed switch is
 * reported and leaves the prompt in place. It returns the alert.
 */
export function ApiEndpointChangeBanner({
  requestedBase,
  config,
  onConfirm,
  onDismiss,
}: ApiEndpointChangeBannerProps) {
  const [busy, setBusy] = useState(false)

  /** handleConfirm stores the switch and reports a failure. */
  const handleConfirm = async () => {
    setBusy(true)
    try {
      await onConfirm()
    } catch (err) {
      console.error(
        `[ApiEndpointChangeBanner] switching the API endpoint failed: ${String(err)}`,
      )
      alert(
        'Switching the API endpoint failed. Nothing was changed; try again.',
      )
    } finally {
      setBusy(false)
    }
  }

  return (
    <ApiEndpointChangeAlert
      requestedBase={requestedBase}
      currentBase={config.api_base}
      keyHint={maskSecretHint(config.api_token)}
      busy={busy}
      onConfirm={() => {
        void handleConfirm()
      }}
      onDismiss={onDismiss}
    />
  )
}
