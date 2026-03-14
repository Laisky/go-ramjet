import { Link, Route, Routes } from 'react-router-dom'

import { AppLayout } from '@/components/app-layout'
import { CVPage } from '@/pages/cv'
import { GPTChatPage } from '@/pages/gptchat'
import { TaskPage } from '@/pages/task'
import { SiteLanding } from '@/site/site-landing'

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
      <Routes>
        <Route path="/" element={<SiteLanding />} />
        <Route path="/gptchat" element={<GPTChatPage />} />
        <Route path="/cv" element={<CVPage />} />
        <Route path="/tasks/:task" element={<TaskPage />} />
        <Route path="*" element={<NotFoundPage />} />
      </Routes>
    </AppLayout>
  )
}
