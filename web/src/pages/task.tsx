import { Link, useParams } from 'react-router-dom'

import { Card, CardDescription, CardTitle } from '@/components/ui/card'
import { Tasks } from '@/pages/tasks'

function getTaskInfo(key?: string) {
  return Tasks.find((t) => t.key === key)
}

/**
 * TaskPage renders a per-task route with quick links to existing backend endpoints.
 */
export function TaskPage() {
  const { task } = useParams()
  const info = getTaskInfo(task)

  if (!info) {
    return (
      <div className="space-y-4">
        <h2 className="text-xl font-semibold">Unknown task</h2>
        <p className="text-sm text-black/70 dark:text-white/70">
          The route does not match a known task.
        </p>
        <Link className="text-sm underline" to="/">
          Back to home
        </Link>
      </div>
    )
  }

  const links: Array<{ label: string; href: string }> = (() => {
    switch (info.key) {
      case 'gptchat':
        return [
          { label: 'Open GPTChat UI', href: '/gptchat' },
          { label: 'Payment page', href: '/gptchat/payment' },
          { label: 'GPTChat API', href: '/gptchat/api' },
          { label: 'Current user', href: '/gptchat/user/me' },
        ]
      case 'auditlog':
        return [
          { label: 'List logs', href: '/auditlog/log' },
          { label: 'List normal logs', href: '/auditlog/normal-log' },
        ]
      case 'jav':
        return [{ label: 'Search (q=...)', href: '/jav/search?q=example' }]
      case 'arweave':
        return [
          { label: 'Gateway', href: '/arweave/gateway/' },
          { label: 'DNS list', href: '/arweave/dns/' },
        ]
      case 'crawler':
        return [{ label: 'Search (q=...)', href: '/crawler/search?q=example' }]
      case 'gitlab':
        return [
          { label: 'Get file (file=...)', href: '/gitlab/file?file=README.md' },
        ]
      case 'heartbeat':
        return [{ label: 'Heartbeat', href: '/heartbeat' }]
      case 'elasticsearch':
        return [
          { label: 'Rollover details', href: '/es/rollover' },
          { label: 'Password by date', href: '/es/password' },
        ]
      default:
        return []
    }
  })()

  return (
    <div className="space-y-6">
      <div className="space-y-2">
        <h2 className="text-xl font-semibold">{info.title}</h2>
        <p className="text-sm text-black/70 dark:text-white/70">
          {info.description}
        </p>
      </div>

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        {links.map((l) => (
          <a key={l.href} href={l.href} className="block">
            <Card className="h-full transition-colors hover:bg-black/5 dark:hover:bg-white/5">
              <CardTitle>{l.label}</CardTitle>
              <CardDescription className="mt-1 break-all">
                {l.href}
              </CardDescription>
            </Card>
          </a>
        ))}
      </div>

      <Link className="text-sm underline" to="/">
        Back to home
      </Link>
    </div>
  )
}
