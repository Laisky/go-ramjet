import { Link } from 'react-router-dom'

import { Card, CardDescription, CardTitle } from '@/components/ui/card'
import { Tasks } from '@/pages/tasks'

/**
 * HomePage renders the unified landing page.
 */
export function HomePage() {
  return (
    <div className="space-y-6">
      <section className="space-y-2">
        <h1 className="text-2xl font-semibold">go-ramjet</h1>
        <p className="text-sm text-black/70 dark:text-white/70">
          A CRON-style task server. Each task exposes APIs, and the SPA provides a unified navigation
          layer.
        </p>
      </section>

      <section className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        {Tasks.map((t) => (
          <Link key={t.key} to={`/tasks/${t.key}`} className="block">
            <Card className="h-full transition-colors hover:bg-black/5 dark:hover:bg-white/5">
              <CardTitle>{t.title}</CardTitle>
              <CardDescription className="mt-1">{t.description}</CardDescription>
            </Card>
          </Link>
        ))}
      </section>
    </div>
  )
}
