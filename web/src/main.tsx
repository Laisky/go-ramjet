import { completeCodeSignIn } from '@/lib/sso-code'
import { resolveSiteId } from '@/site/site-meta'

/** start completes the one-time callback before telemetry, app modules or React can load. */
export async function start(): Promise<void> {
  const site = resolveSiteId()
  const app = site === 'cv' || site === 'chat' ? site : null
  await completeCodeSignIn(app)
  await import('./render-app')
}
export const ready = start()
