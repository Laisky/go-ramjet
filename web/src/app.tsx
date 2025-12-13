import { Route, Routes } from 'react-router-dom'

import { AppLayout } from '@/components/app-layout'
import { HomePage } from '@/pages/home'
import { TaskPage } from '@/pages/task'

export function App() {
  return (
    <AppLayout>
      <Routes>
        <Route path="/" element={<HomePage />} />
        <Route path="/tasks/:task" element={<TaskPage />} />
      </Routes>
    </AppLayout>
  )
}
