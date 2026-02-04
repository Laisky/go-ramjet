import { Link, useLocation } from 'react-router-dom'

import { ThemeToggle } from '@/components/theme-toggle'
import { getActiveSiteId } from '@/site/site-meta'
import { cn } from '@/utils/cn'

/**
 * AppLayout wraps children with the global shell layout and returns the layout element.
 */
export function AppLayout({ children }: { children: React.ReactNode }) {
  const location = useLocation()
  const siteId = getActiveSiteId()
  const isChatPage =
    location.pathname.startsWith('/gptchat') || siteId === 'chat'
  const isCvPage = location.pathname.startsWith('/cv') || siteId === 'cv'
  const isSpecialPage = isChatPage || isCvPage
  const containerClass = isSpecialPage
    ? 'w-full px-0'
    : 'mx-auto max-w-5xl px-4'

  return (
    <div
      className={cn(
        'flex flex-col bg-background text-foreground',
        isChatPage ? 'min-h-dvh max-w-full overflow-x-hidden' : 'min-h-screen',
      )}
    >
      {!isSpecialPage && (
        <header className="sticky top-0 z-40 border-b bg-background/80 backdrop-blur-md">
          <div
            className={`${containerClass} flex items-center justify-between py-3`}
          >
            <Link to="/" className="text-sm font-semibold tracking-tight">
              go-ramjet
            </Link>
            <ThemeToggle />
          </div>
        </header>
      )}
      <main
        className={cn(
          containerClass,
          'flex-1 min-h-0',
          isChatPage ? 'flex flex-col py-0' : 'py-6',
        )}
      >
        {children}
      </main>
    </div>
  )
}
