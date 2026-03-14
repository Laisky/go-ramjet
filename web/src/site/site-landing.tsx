import { Suspense, lazy } from 'react'

import { HomePage } from '@/pages/home'
import { getActiveSiteId } from './site-meta'

// Lazy-load page components to keep the initial bundle small.
// These are only loaded when the landing page determines the active site.
const GPTChatPage = lazy(() =>
  import('@/pages/gptchat').then((m) => ({ default: m.GPTChatPage })),
)
const CVPage = lazy(() =>
  import('@/pages/cv').then((m) => ({ default: m.CVPage })),
)

/**
 * SiteLanding renders the landing page for the active site id and returns a page element.
 */
export function SiteLanding() {
  const siteId = getActiveSiteId()

  if (siteId === 'chat') {
    return (
      <Suspense fallback={null}>
        <GPTChatPage />
      </Suspense>
    )
  }

  if (siteId === 'cv') {
    return (
      <Suspense fallback={null}>
        <CVPage />
      </Suspense>
    )
  }

  return <HomePage />
}
