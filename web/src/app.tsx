import { Route, Routes } from 'react-router-dom'

import { AppLayout } from '@/components/app-layout'
import { CVPage } from '@/pages/cv'
import { GPTChatPage } from '@/pages/gptchat'
// import { GPTChatPaymentPage } from '@/pages/gptchat/payment'
import { TaskPage } from '@/pages/task'
import { SiteLanding } from '@/site/site-landing'

/**
 * App renders the application routes inside the layout and returns the root element.
 */
export function App() {
  return (
    <AppLayout>
      <Routes>
        <Route path="/" element={<SiteLanding />} />
        <Route path="/gptchat" element={<GPTChatPage />} />
        {/* <Route path="/gptchat/payment" element={<GPTChatPaymentPage />} /> */}
        <Route path="/cv" element={<CVPage />} />
        <Route path="/tasks/:task" element={<TaskPage />} />
      </Routes>
    </AppLayout>
  )
}
