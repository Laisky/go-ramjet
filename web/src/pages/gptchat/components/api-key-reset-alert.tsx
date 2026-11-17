/**
 * Alert shown when a session's API key was lost to the sync-key autofill bug.
 */
import { KeyRound, TriangleAlert } from 'lucide-react'

import { Button } from '@/components/ui/button'

/**
 * API_KEY_HELP_URL points to the guide that explains where a GPTChat API key
 * comes from, so users who no longer have theirs can get it again.
 */
const API_KEY_HELP_URL =
  'https://wiki.laisky.com/projects/gpt/pay/#page_gpt_pay'

/**
 * ApiKeyResetAlertProps configures ApiKeyResetAlert. onResetApiKey opens the
 * settings with the API key field focused; onDismiss keeps the free tier and
 * stops the alert for this session.
 */
export interface ApiKeyResetAlertProps {
  onResetApiKey: () => void
  onDismiss: () => void
}

/**
 * ApiKeyResetAlert explains that this session's API key was overwritten by the
 * sync key and could not be restored without guessing between credentials,
 * and walks the user through re-entering it. It returns a high-contrast alert pinned below the page header; it is
 * fixed rather than sticky because the page scrolls the window while its
 * ancestors clip overflow, which would detach a sticky element.
 */
export function ApiKeyResetAlert({
  onResetApiKey,
  onDismiss,
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
            with your sync key, and it could not be restored safely: no other
            session on the same API endpoint holds one unambiguous key. This
            session has been switched to the free tier so it keeps working.
          </p>
          <ol className="list-decimal space-y-0.5 pl-5">
            <li>
              Click <strong>Re-enter API key</strong> (or open{' '}
              <strong>Settings → API Key</strong>).
            </li>
            <li>
              Paste your API key; it is saved automatically. Close Settings to
              finish.
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
