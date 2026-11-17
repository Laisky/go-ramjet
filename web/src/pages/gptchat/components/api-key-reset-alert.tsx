/**
 * Alert shown when a session's API key was lost to the sync-key autofill bug.
 */
import { KeyRound, TriangleAlert } from 'lucide-react'

import { Button } from '@/components/ui/button'
import type { ApiTokenChoice } from '../hooks/session-credentials'
import { useApiTokenRecovery } from '../hooks/use-api-token-recovery'
import type { SessionConfig } from '../types'

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
 * on the same endpoint, and onUseChoice adopts one of them.
 */
export interface ApiKeyResetAlertProps {
  onResetApiKey: () => void
  onDismiss: () => void
  choices?: ApiTokenChoice[]
  onUseChoice?: (sourceSessionId: number) => void
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
}: ApiKeyResetAlertProps) {
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
            Your API key needs to be re-entered
          </p>
          <p>
            A browser password manager overwrote this session&apos;s API key
            with your sync key. This session has been switched to the free tier
            so it keeps working.
          </p>
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
            >
              <KeyRound className="h-4 w-4" aria-hidden />
              Re-enter API key
            </Button>
            <Button
              size="sm"
              variant="ghost"
              className="text-destructive-foreground hover:bg-destructive-foreground/15"
              onClick={onDismiss}
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
 * ApiKeyResetBannerProps configures ApiKeyResetBanner: sessionId and config
 * describe the active session, updateConfig persists an update, and
 * onOpenApiKeySettings opens the settings with the API key editor focused.
 */
export interface ApiKeyResetBannerProps {
  sessionId: number
  config: SessionConfig
  updateConfig: (updates: Partial<SessionConfig>) => Promise<void>
  onOpenApiKeySettings: () => void
}

/**
 * ApiKeyResetBanner shows ApiKeyResetAlert while the active session awaits
 * an API key reset, with the key choices other sessions offer. It returns
 * the alert, or nothing when no reset is pending.
 */
export function ApiKeyResetBanner({
  sessionId,
  config,
  updateConfig,
  onOpenApiKeySettings,
}: ApiKeyResetBannerProps) {
  const { choices, adopt } = useApiTokenRecovery({
    sessionId,
    config,
    updateConfig,
  })
  if (!config.api_token_reset_required) return null

  return (
    <ApiKeyResetAlert
      choices={choices}
      onUseChoice={(sourceSessionId) => {
        void adopt(sourceSessionId)
      }}
      onResetApiKey={onOpenApiKeySettings}
      onDismiss={() => {
        void updateConfig({ api_token_reset_required: false })
      }}
    />
  )
}
