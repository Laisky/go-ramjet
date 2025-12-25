import { Route, Routes } from 'react-router-dom'

import { AppLayout } from '@/components/app-layout'
import { GPTChatPage } from '@/pages/gptchat'
// import { GPTChatPaymentPage } from '@/pages/gptchat/payment'
import { HomePage } from '@/pages/home'
import { TaskPage } from '@/pages/task'

export function App() {
  return (
    <AppLayout>
      <Routes>
        <Route path="/" element={<HomePage />} />
        <Route path="/gptchat" element={<GPTChatPage />} />
        {/* <Route path="/gptchat/payment" element={<GPTChatPaymentPage />} /> */}
        <Route path="/tasks/:task" element={<TaskPage />} />
      </Routes>
    </AppLayout>
  )
}
