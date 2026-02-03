export type TaskDefinition = {
  key: string
  title: string
  description: string
}

/**
 * Tasks defines the known task pages exposed via the SPA.
 */
export const Tasks: TaskDefinition[] = [
  {
    key: 'gptchat',
    title: 'GPT Chat',
    description:
      'Chat UI and related endpoints (existing UI remains at /gptchat).',
  },
  {
    key: 'auditlog',
    title: 'Audit Log',
    description: 'Receive and list audit logs via HTTP API.',
  },
  {
    key: 'cv',
    title: 'CV',
    description: 'Single-page resume editor with markdown preview.',
  },
  {
    key: 'jav',
    title: 'JAV',
    description: 'Search endpoint for the JAV task.',
  },
  {
    key: 'arweave',
    title: 'Arweave',
    description: 'Gateway/DNS and local cache utilities.',
  },
  {
    key: 'crawler',
    title: 'Crawler',
    description: 'Search endpoint for crawler service.',
  },
  {
    key: 'gitlab',
    title: 'GitLab',
    description: 'Fetch files from GitLab via API.',
  },
  {
    key: 'heartbeat',
    title: 'Heartbeat',
    description: 'Quick health endpoint with goroutine stats.',
  },
  {
    key: 'elasticsearch',
    title: 'Elasticsearch',
    description: 'Rollover info and password generator endpoints.',
  },
]
