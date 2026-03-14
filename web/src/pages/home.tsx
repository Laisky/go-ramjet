import {
  Database,
  FileText,
  Film,
  GitBranch,
  Globe,
  Heart,
  MessageSquare,
  Search,
  User,
} from 'lucide-react'
import { useEffect } from 'react'
import { Link } from 'react-router-dom'

import { Card, CardDescription, CardTitle } from '@/components/ui/card'
import { cn } from '@/utils/cn'
import { setPageFavicon, setPageTitle } from '@/utils/dom'

interface TaskDefinition {
  key: string
  title: string
  description: string
  icon: React.ReactNode
  featured?: boolean
  iconColor?: string
}

/**
 * Tasks defines the known task pages exposed via the SPA.
 */
const Tasks: TaskDefinition[] = [
  {
    key: 'gptchat',
    title: 'GPT Chat',
    description:
      'Chat with AI models including GPT-4, Claude, Gemini, and more. Supports streaming, image generation, and MCP tools.',
    icon: <MessageSquare className="h-6 w-6" />,
    featured: true,
  },
  {
    key: 'auditlog',
    title: 'Audit Log',
    description: 'Receive and list audit logs via HTTP API.',
    icon: <FileText className="h-5 w-5" />,
    iconColor:
      'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-400',
  },
  {
    key: 'cv',
    title: 'CV',
    description: 'Single-page resume editor with live markdown preview.',
    icon: <User className="h-5 w-5" />,
    iconColor: 'bg-sky-100 text-sky-700 dark:bg-sky-900/30 dark:text-sky-400',
  },
  {
    key: 'jav',
    title: 'JAV',
    description: 'Search endpoint for the JAV task.',
    icon: <Film className="h-5 w-5" />,
    iconColor:
      'bg-pink-100 text-pink-700 dark:bg-pink-900/30 dark:text-pink-400',
  },
  {
    key: 'arweave',
    title: 'Arweave',
    description: 'Gateway/DNS and local cache utilities.',
    icon: <Globe className="h-5 w-5" />,
    iconColor:
      'bg-teal-100 text-teal-700 dark:bg-teal-900/30 dark:text-teal-400',
  },
  {
    key: 'crawler',
    title: 'Crawler',
    description: 'Search endpoint for crawler service.',
    icon: <Search className="h-5 w-5" />,
    iconColor: 'bg-sky-100 text-sky-700 dark:bg-sky-900/30 dark:text-sky-400',
  },
  {
    key: 'gitlab',
    title: 'GitLab',
    description: 'Fetch files from GitLab via API.',
    icon: <GitBranch className="h-5 w-5" />,
    iconColor:
      'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-400',
  },
  {
    key: 'heartbeat',
    title: 'Heartbeat',
    description: 'Quick health endpoint with goroutine stats.',
    icon: <Heart className="h-5 w-5" />,
    iconColor:
      'bg-rose-100 text-rose-700 dark:bg-rose-900/30 dark:text-rose-400',
  },
  {
    key: 'elasticsearch',
    title: 'Elasticsearch',
    description: 'Rollover info and password generator endpoints.',
    icon: <Database className="h-5 w-5" />,
    iconColor:
      'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400',
  },
]

/**
 * HomePage renders the unified landing page with task cards and returns the page element.
 */
export function HomePage() {
  useEffect(() => {
    setPageTitle('Laisky')
    setPageFavicon('https://s3.laisky.com/uploads/2025/12/favicon.ico')
  }, [])

  const featuredTask = Tasks.find((t) => t.featured)
  const otherTasks = Tasks.filter((t) => !t.featured)

  return (
    <div className="space-y-8">
      {/* Hero Section */}
      <section className="space-y-4 text-center">
        <h1 className="text-3xl font-bold tracking-tight text-primary sm:text-4xl">
          go-ramjet
        </h1>
        <p className="mx-auto max-w-2xl text-muted-foreground">
          A CRON-style task server with a unified web interface. Each task
          exposes APIs for specific functionality, and this SPA provides a
          modern navigation layer.
        </p>
      </section>

      {/* Featured Task */}
      {featuredTask && (
        <section>
          <Link to={`/gptchat`} className="block">
            <Card className="group bg-primary p-6 text-primary-foreground transition-shadow hover:shadow-xl hover:shadow-primary/20">
              <div className="mb-4 inline-flex rounded-lg bg-primary-foreground/20 p-3">
                {featuredTask.icon}
              </div>
              <CardTitle className="mb-2 text-2xl text-primary-foreground">
                {featuredTask.title}
              </CardTitle>
              <CardDescription className="text-primary-foreground/80">
                {featuredTask.description}
              </CardDescription>
              <div className="mt-4 inline-flex items-center gap-1 text-sm font-medium">
                Open Chat →
              </div>
            </Card>
          </Link>
        </section>
      )}

      {/* Other Tasks */}
      <section>
        <h2 className="mb-4 text-lg font-semibold">Other Services</h2>
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {otherTasks.map((task) => (
            <Link key={task.key} to={`/tasks/${task.key}`} className="block">
              <Card className="group h-full transition-all hover:border-primary/50 hover:shadow-md">
                <div className="flex items-start gap-3">
                  <div
                    className={cn(
                      'rounded-lg p-2 transition-colors',
                      task.iconColor ??
                        'bg-muted text-muted-foreground group-hover:bg-accent group-hover:text-accent-foreground',
                    )}
                  >
                    {task.icon}
                  </div>
                  <div className="flex-1">
                    <CardTitle className="text-base">{task.title}</CardTitle>
                    <CardDescription className="mt-1 text-sm">
                      {task.description}
                    </CardDescription>
                  </div>
                </div>
              </Card>
            </Link>
          ))}
        </div>
      </section>

      {/* Footer */}
      <footer className="border-t border-border pt-6 text-center text-sm text-muted-foreground">
        <p>
          Built with React, Vite, and TailwindCSS.{' '}
          <a
            href="https://github.com/Laisky/go-ramjet"
            target="_blank"
            rel="noopener noreferrer"
            className="underline hover:text-foreground"
          >
            View on GitHub →
          </a>
        </p>
      </footer>
    </div>
  )
}
