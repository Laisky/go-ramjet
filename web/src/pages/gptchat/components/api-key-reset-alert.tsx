/**
 * Alert shown when a session's API key was lost to the sync-key autofill bug.
 */
import { KeyRound, TriangleAlert } from 'lucide-react'
import { useState } from 'react'

import { Button } from '@/components/ui/button'
import type { ApiTokenChoice } from '../hooks/session-credentials'
import { useApiTokenRecovery } from '../hooks/use-api-token-recovery'
import type { SessionConfig } from '../types'
import { maskSecretHint } from '../utils/api-token'
import type { ConfigUpdateHandler } from '../utils/credential-policy'

/**
 * API_KEY_HELP_URL points to the guide that explains where a GPTChat API key
 * comes from, so users who no longer have theirs can get it again.
 */
const API_KEY_HELP_URL =
  'https://wiki.laisky.com/projects/gpt/pay/#page_gpt_pay'

/**
 * ApiKeyResetAlertProps configures ApiKeyResetAlert. onResetApiKey opens the
 * settings with the API key editor focused; onDismiss keeps the free tier
 * and stops the alert for this session; choices are keys of other sessions
 * on the same endpoint, and onUseChoice adopts one of them. previousHint is
 * the masked key the reset set aside because it may be the user's own (it
 * equalled the sync key), and onRestorePrevious restores it; busy disables
 * the actions while that runs.
 */
export interface ApiKeyResetAlertProps {
  onResetApiKey: () => void
  onDismiss: () => void
  choices?: ApiTokenChoice[]
  onUseChoice?: (sourceSessionId: number) => void
  previousHint?: string
  onRestorePrevious?: () => void
  busy?: boolean
}

/**
 * ApiKeyResetAlert explains that this session's API key was overwritten by
 * the sync key, offers the keys other sessions on the same endpoint hold for
 * the user to choose from, and walks the user through re-entering it.
 * It returns a high-contrast alert pinned below the page header. The alert
 * is fixed rather than sticky: the page scrolls the window while its
 * ancestors clip overflow, which would detach a sticky element.
 */
export function ApiKeyResetAlert({
  onResetApiKey,
  onDismiss,
  choices = [],
  onUseChoice,
  previousHint,
  onRestorePrevious,
  busy = false,
}: ApiKeyResetAlertProps) {
  const ambiguous = !!previousHint && !!onRestorePrevious
  return (
    <div className="pointer-events-none fixed left-12 right-0 top-12 z-30 px-2 pt-2 sm:px-4">
      <div
        role="alert"
        aria-labelledby="api-key-reset-alert-title"
        className="pointer-events-auto mx-auto flex max-w-3xl items-start gap-3 rounded-lg border-2 border-destructive bg-destructive p-4 text-destructive-foreground shadow-lg"
      >
        <TriangleAlert className="mt-0.5 h-5 w-5 shrink-0" aria-hidden />
        <div className="min-w-0 flex-1 space-y-2 text-sm">
          <p id="api-key-reset-alert-title" className="text-base font-semibold">
            {ambiguous
              ? 'Confirm this session’s API key'
              : 'Your API key needs to be re-entered'}
          </p>
          {ambiguous ? (
            <>
              <p>
                This session&apos;s API key (
                <code className="font-mono">{previousHint}</code>) was the same
                as your sync key. Either a browser password manager overwrote
                the API key with the sync key, or you chose the same value for
                both. Until you decide, this session uses the free tier.
              </p>
              <Button
                size="sm"
                variant="outline"
                className="h-auto gap-1.5 border-destructive-foreground bg-transparent py-1 text-left text-destructive-foreground hover:bg-destructive-foreground/15"
                onClick={onRestorePrevious}
                disabled={busy}
              >
                <KeyRound className="h-4 w-4 shrink-0" aria-hidden />
                <span>
                  It is my API key: keep{' '}
                  <code className="font-mono">{previousHint}</code>
                </span>
              </Button>
            </>
          ) : (
            <p>
              A browser password manager overwrote this session&apos;s API key
              with your sync key. This session has been switched to the free
              tier so it keeps working.
            </p>
          )}
          {choices.length > 0 && onUseChoice && (
            <div className="space-y-1">
              <p>
                Other sessions on the same API endpoint hold a key. Use one only
                if it belongs to the account this session used:
              </p>
              <ul className="flex flex-wrap gap-2">
                {choices.map((choice) => (
                  <li key={choice.sourceSessionId}>
                    <Button
                      size="sm"
                      variant="outline"
                      className="h-auto gap-1.5 border-destructive-foreground bg-transparent py-1 text-left text-destructive-foreground hover:bg-destructive-foreground/15"
                      onClick={() => onUseChoice(choice.sourceSessionId)}
                      disabled={busy}
                    >
                      <KeyRound className="h-4 w-4 shrink-0" aria-hidden />
                      <span>
                        Use <code className="font-mono">{choice.hint}</code>{' '}
                        from {choice.sessionNames.join(', ')}
                      </span>
                    </Button>
                  </li>
                ))}
              </ul>
            </div>
          )}
          <ol className="list-decimal space-y-0.5 pl-5">
            <li>
              Click <strong>Re-enter API key</strong> (or open{' '}
              <strong>Settings → API Key</strong>).
            </li>
            <li>
              Paste your API key and click <strong>Save</strong>.
            </li>
            <li>
              Don&apos;t have it at hand? Get it again via the{' '}
              <a
                href={API_KEY_HELP_URL}
                target="_blank"
                rel="noopener noreferrer"
                className="font-semibold underline underline-offset-2"
              >
                API key guide
              </a>
              .
            </li>
          </ol>
          <div className="flex flex-wrap gap-2 pt-1">
            <Button
              size="sm"
              variant="outline"
              className="gap-1.5 border-destructive-foreground bg-destructive-foreground text-destructive hover:bg-destructive-foreground/90"
              onClick={onResetApiKey}
              disabled={busy}
            >
              <KeyRound className="h-4 w-4" aria-hidden />
              Re-enter API key
            </Button>
            <Button
              size="sm"
              variant="ghost"
              className="text-destructive-foreground hover:bg-destructive-foreground/15"
              onClick={onDismiss}
              disabled={busy}
            >
              Keep free tier
            </Button>
          </div>
        </div>
      </div>
    </div>
  )
}

/**
 * RESTORE_WITH_ROTATION_PROMPT asks the user to confirm that restoring a key
 * that is also the sync key rotates the sync key first.
 */
const RESTORE_WITH_ROTATION_PROMPT =
  'Keep this value as the API key of this session? It is also your sync key, so the sync key is rotated first: your cloud backup moves to a new random key and the copy under the old key is deleted. If you already rotated the sync key on another device, cancel and paste that key into Settings → Data Sync instead.'

/**
 * ApiKeyResetBannerProps configures ApiKeyResetBanner: sessionId and config
 * describe the active session, updateConfig stores an update,
 * onOpenApiKeySettings opens the settings with the API key editor focused,
 * and rotateSyncKey moves the cloud backup to a new sync key, which
 * restoring a key that is also the sync key needs.
 */
export interface ApiKeyResetBannerProps {
  sessionId: number
  config: SessionConfig
  updateConfig: ConfigUpdateHandler
  onOpenApiKeySettings: () => void
  rotateSyncKey?: (oldKey: string) => Promise<unknown>
}

/**
 * ApiKeyResetBanner shows ApiKeyResetAlert while the active session awaits
 * an API key reset, with the key choices other sessions offer and, when the
 * reset set a possibly genuine key aside, the option to restore it. A
 * failed action is reported and leaves the reset pending. It returns the
 * alert, or nothing when no reset is pending.
 */
export function ApiKeyResetBanner({
  sessionId,
  config,
  updateConfig,
  onOpenApiKeySettings,
  rotateSyncKey,
}: ApiKeyResetBannerProps) {
  const [busy, setBusy] = useState(false)
  const { choices, adopt, needsRotation, restorePrevious } =
    useApiTokenRecovery({ sessionId, config, updateConfig, rotateSyncKey })
  if (!config.api_token_reset_required) return null

  /**
   * run performs action with the alert disabled, and reports a failure as
   * failureMessage.
   */
  const run = async (
    action: () => Promise<unknown>,
    failureMessage: string,
  ) => {
    setBusy(true)
    try {
      await action()
    } catch (err) {
      console.error(`[ApiKeyResetBanner] ${failureMessage}: ${String(err)}`)
      alert(`${failureMessage}. Nothing was changed; try again.`)
    } finally {
      setBusy(false)
    }
  }

  const previousToken = config.api_token_before_reset?.trim() || ''
  const canRestore = !!previousToken && (!needsRotation || !!rotateSyncKey)

  return (
    <ApiKeyResetAlert
      busy={busy}
      choices={choices}
      onUseChoice={(sourceSessionId) => {
        void run(() => adopt(sourceSessionId), 'Using that API key failed')
      }}
      previousHint={canRestore ? maskSecretHint(previousToken) : undefined}
      onRestorePrevious={
        canRestore
          ? () => {
              if (
                needsRotation &&
                !window.confirm(RESTORE_WITH_ROTATION_PROMPT)
              ) {
                return
              }
              void run(restorePrevious, 'Restoring the API key failed')
            }
          : undefined
      }
      onResetApiKey={onOpenApiKeySettings}
      onDismiss={() => {
        void run(
          () => updateConfig({ api_token_reset_required: false }),
          'Saving your choice failed',
        )
      }}
    />
  )
}
