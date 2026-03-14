import { Suspense, lazy } from 'react'
import { Link, Route, Routes } from 'react-router-dom'

import { AppLayout } from '@/components/app-layout'
import { SiteLanding } from '@/site/site-landing'

const GPTChatPage = lazy(() =>
  import('@/pages/gptchat').then((m) => ({ default: m.GPTChatPage })),
)
const CVPage = lazy(() =>
  import('@/pages/cv').then((m) => ({ default: m.CVPage })),
)
const TaskPage = lazy(() =>
  import('@/pages/task').then((m) => ({ default: m.TaskPage })),
)

function PageLoader() {
  return (
    <div className="flex min-h-[50vh] items-center justify-center">
      <div className="h-6 w-6 animate-spin rounded-full border-2 border-primary/20 border-t-primary" />
    </div>
  )
}

function NotFoundPage() {
  return (
    <div className="flex min-h-[50vh] flex-col items-center justify-center text-center">
      <h1 className="text-4xl font-bold">404</h1>
      <p className="mt-2 text-muted-foreground">Page not found.</p>
      <Link
        to="/"
        className="mt-4 text-sm text-primary underline hover:text-primary/80"
      >
        Back to home
      </Link>
    </div>
  )
}

/**
 * App renders the application routes inside the layout and returns the root element.
 */
export function App() {
  return (
    <AppLayout>
      <Suspense fallback={<PageLoader />}>
        <Routes>
          <Route path="/" element={<SiteLanding />} />
          <Route path="/gptchat" element={<GPTChatPage />} />
          <Route path="/cv" element={<CVPage />} />
          <Route path="/tasks/:task" element={<TaskPage />} />
          <Route path="*" element={<NotFoundPage />} />
        </Routes>
      </Suspense>
    </AppLayout>
  )
}
