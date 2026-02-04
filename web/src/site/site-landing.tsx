import { CVPage } from '@/pages/cv'
import { GPTChatPage } from '@/pages/gptchat'
import { HomePage } from '@/pages/home'
import { getActiveSiteId } from './site-meta'

/**
 * SiteLanding renders the landing page for the active site id and returns a page element.
 */
export function SiteLanding() {
  const siteId = getActiveSiteId()

  if (siteId === 'chat') {
    return <GPTChatPage />
  }

  if (siteId === 'cv') {
    return <CVPage />
  }

  return <HomePage />
}
