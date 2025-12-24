import {
  Database,
  FileText,
  Film,
  GitBranch,
  Globe,
  Heart,
  MessageSquare,
  Search,
} from 'lucide-react'
import { Link } from 'react-router-dom'

import { Card, CardDescription, CardTitle } from '@/components/ui/card'

interface TaskDefinition {
  key: string
  title: string
  description: string
  icon: React.ReactNode
  featured?: boolean
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
  },
  {
    key: 'jav',
    title: 'JAV',
    description: 'Search endpoint for the JAV task.',
    icon: <Film className="h-5 w-5" />,
  },
  {
    key: 'arweave',
    title: 'Arweave',
    description: 'Gateway/DNS and local cache utilities.',
    icon: <Globe className="h-5 w-5" />,
  },
  {
    key: 'crawler',
    title: 'Crawler',
    description: 'Search endpoint for crawler service.',
    icon: <Search className="h-5 w-5" />,
  },
  {
    key: 'gitlab',
    title: 'GitLab',
    description: 'Fetch files from GitLab via API.',
    icon: <GitBranch className="h-5 w-5" />,
  },
  {
    key: 'heartbeat',
    title: 'Heartbeat',
    description: 'Quick health endpoint with goroutine stats.',
    icon: <Heart className="h-5 w-5" />,
  },
  {
    key: 'elasticsearch',
    title: 'Elasticsearch',
    description: 'Rollover info and password generator endpoints.',
    icon: <Database className="h-5 w-5" />,
  },
]

/**
 * HomePage renders the unified landing page with task cards.
 */
export function HomePage() {
  const featuredTask = Tasks.find((t) => t.featured)
  const otherTasks = Tasks.filter((t) => !t.featured)

  return (
    <div className="space-y-8">
      {/* Hero Section */}
      <section className="space-y-4 text-center">
        <h1 className="text-3xl font-bold tracking-tight sm:text-4xl">
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
            <Card className="group relative overflow-hidden bg-primary p-6 text-primary-foreground transition-all hover:shadow-xl hover:shadow-primary/20">
              <div className="absolute -right-8 -top-8 h-32 w-32 rounded-full bg-primary-foreground/10" />
              <div className="absolute -bottom-4 -left-4 h-24 w-24 rounded-full bg-primary-foreground/10" />
              <div className="relative">
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
                  <div className="rounded-lg bg-muted p-2 text-muted-foreground transition-colors group-hover:bg-accent group-hover:text-accent-foreground">
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
