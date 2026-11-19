/**
 * Alert shown when a link asked to change the API endpoint or the API key of
 * a session that holds the user's own key.
 */
import { ShieldAlert } from 'lucide-react'
import { useState } from 'react'

import { Button } from '@/components/ui/button'
import type { SessionConfig } from '../types'
import { maskSecretHint } from '../utils/api-token'
import {
  heldUserApiToken,
  type LinkCredentialChange,
} from '../utils/credential-policy'

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
 * LinkCredentialChangeAlertProps configures LinkCredentialChangeAlert.
 * change is what the link asked for, currentBase the endpoint in use, and
 * keyHint the masked key the session holds. onConfirm applies the change,
 * onDismiss keeps the current settings, and busy disables both while a
 * choice is being stored.
 */
export interface LinkCredentialChangeAlertProps {
  change: LinkCredentialChange
  currentBase: string
  keyHint: string
  onConfirm: () => void
  onDismiss: () => void
  busy?: boolean
}

/**
 * LinkCredentialChangeAlert asks the user whether a link may move this
 * session to another endpoint, replace its API key, or both. Keeping the
 * current settings is the prominent choice. It returns an alert pinned below
 * the page header that does not block the rest of the page.
 */
export function LinkCredentialChangeAlert({
  change,
  currentBase,
  keyHint,
  onConfirm,
  onDismiss,
  busy = false,
}: LinkCredentialChangeAlertProps) {
  const requested =
    change.apiBase !== undefined ? describeEndpoint(change.apiBase) : ''
  const linkKeyHint =
    change.apiToken !== undefined ? maskSecretHint(change.apiToken) : ''
  return (
    <div className="pointer-events-none fixed left-12 right-0 top-12 z-30 px-2 pt-2 sm:px-4">
      <div
        role="alert"
        aria-labelledby="link-credential-change-alert-title"
        className="pointer-events-auto mx-auto flex max-w-3xl items-start gap-3 rounded-lg border-2 border-destructive bg-destructive p-4 text-destructive-foreground shadow-lg"
      >
        <ShieldAlert className="mt-0.5 h-5 w-5 shrink-0" aria-hidden />
        <div className="min-w-0 flex-1 space-y-2 text-sm">
          <p
            id="link-credential-change-alert-title"
            className="text-base font-semibold"
          >
            Apply the API settings from this link?
          </p>
          {requested && (
            <p>
              The link you opened asks to send this session&apos;s requests to{' '}
              <code className="break-all font-mono">{requested}</code> instead
              of{' '}
              <code className="break-all font-mono">
                {describeEndpoint(currentBase)}
              </code>
              .
            </p>
          )}
          {linkKeyHint && (
            <p>
              The link asks to replace your API key (
              <code className="font-mono">{keyHint}</code>) with{' '}
              <code className="font-mono">{linkKeyHint}</code>. Your requests
              would then use, and be visible to, that key&apos;s account.
            </p>
          )}
          {requested && !linkKeyHint && (
            <p>
              Your API key (<code className="font-mono">{keyHint}</code>) would
              be sent there too, and whoever runs that address could use it.
            </p>
          )}
          <p>Apply these settings only if you trust whoever sent the link.</p>
          <div className="flex flex-wrap gap-2 pt-1">
            <Button
              size="sm"
              variant="outline"
              className="border-destructive-foreground bg-destructive-foreground text-destructive hover:bg-destructive-foreground/90"
              onClick={onDismiss}
              disabled={busy}
            >
              Keep my current settings
            </Button>
            <Button
              size="sm"
              variant="ghost"
              className="text-destructive-foreground hover:bg-destructive-foreground/15"
              onClick={onConfirm}
              disabled={busy}
            >
              Apply link settings
            </Button>
          </div>
        </div>
      </div>
    </div>
  )
}

/**
 * LinkCredentialChangeBannerProps configures LinkCredentialChangeBanner:
 * change is the pending link change, config the active session, and
 * onConfirm / onDismiss resolve the prompt.
 */
export interface LinkCredentialChangeBannerProps {
  change: LinkCredentialChange
  config: Pick<
    SessionConfig,
    | 'api_base'
    | 'api_token'
    | 'api_token_reset_required'
    | 'api_token_before_reset'
    | 'sync_key'
  >
  onConfirm: () => Promise<void>
  onDismiss: () => void
}

/**
 * LinkCredentialChangeBanner shows LinkCredentialChangeAlert for a pending
 * link change and stores the user's choice. A refused or failed change is
 * reported and leaves the prompt in place. It returns the alert.
 */
export function LinkCredentialChangeBanner({
  change,
  config,
  onConfirm,
  onDismiss,
}: LinkCredentialChangeBannerProps) {
  const [busy, setBusy] = useState(false)

  /** handleConfirm stores the change and reports a failure. */
  const handleConfirm = async () => {
    setBusy(true)
    try {
      await onConfirm()
    } catch (err) {
      console.error(
        `[LinkCredentialChangeBanner] applying the link settings failed: ${String(err)}`,
      )
      alert('Applying the link settings failed. Nothing was changed.')
    } finally {
      setBusy(false)
    }
  }

  return (
    <LinkCredentialChangeAlert
      change={change}
      currentBase={config.api_base}
      keyHint={maskSecretHint(heldUserApiToken(config, config.sync_key))}
      busy={busy}
      onConfirm={() => {
        void handleConfirm()
      }}
      onDismiss={onDismiss}
    />
  )
}
