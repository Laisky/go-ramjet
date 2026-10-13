import '@/telemetry/instrument'
import React from 'react'
import ReactDOM from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import { ThemeProvider } from 'next-themes'

import { getReactErrorOptions } from '@/telemetry/react-errors'
import { App } from '@/app'
import { ErrorBoundary } from '@/components/error-boundary'
import { bootstrapSite, resolveSiteId } from '@/site/site-meta'
import { bootstrapOwnershipSession } from '@/pages/gptchat/utils/ownership-session'
import '@/styles/globals.css'
import '@/styles/cv.css'
import '@/styles/site-themes.css'

bootstrapOwnershipSession(resolveSiteId())
bootstrapSite()

ReactDOM.createRoot(
  document.getElementById('root')!,
  getReactErrorOptions(),
).render(
  <React.StrictMode>
    <ErrorBoundary>
      <ThemeProvider attribute="class" defaultTheme="system" enableSystem>
        <BrowserRouter>
          <App />
        </BrowserRouter>
      </ThemeProvider>
    </ErrorBoundary>
  </React.StrictMode>,
)
