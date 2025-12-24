import { Link, useLocation } from 'react-router-dom'

import { ThemeToggle } from '@/components/theme-toggle'

/**
 * AppLayout provides the global shell with header.
 */
export function AppLayout({ children }: { children: React.ReactNode }) {
  const location = useLocation()
  const isChatPage = location.pathname.startsWith('/gptchat')
  const containerClass = isChatPage ? 'w-full px-0' : 'mx-auto max-w-5xl px-4'

  return (
    <div className="flex min-h-screen flex-col bg-background text-foreground">
      {!isChatPage && (
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
        className={`${containerClass} flex-1 ${isChatPage ? 'py-0' : 'py-6'}`}
      >
        {children}
      </main>
    </div>
  )
}
