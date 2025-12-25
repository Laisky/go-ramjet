import { render } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { ThemeProvider } from 'next-themes'

import { App } from '@/app'

/**
 * renderApp renders the full app with router and theme provider.
 */
export function renderApp(initialPath = '/') {
  return render(
    <ThemeProvider attribute="class" defaultTheme="system" enableSystem>
      <MemoryRouter initialEntries={[initialPath]}>
        <App />
      </MemoryRouter>
    </ThemeProvider>,
  )
}
