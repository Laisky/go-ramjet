import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { renderApp } from '@/test/render'

describe('routing', () => {
  it('renders landing page with task cards', () => {
    renderApp('/')

    expect(screen.getByRole('heading', { name: 'go-ramjet' })).toBeInTheDocument()
    expect(screen.getByText('GPT Chat')).toBeInTheDocument()
    expect(screen.getByText('Audit Log')).toBeInTheDocument()
  })

  it('navigates to task page', async () => {
    const user = userEvent.setup()
    renderApp('/')

    await user.click(screen.getByText('Heartbeat'))

    expect(screen.getByRole('heading', { name: 'Heartbeat', level: 2 })).toBeInTheDocument()
    expect(screen.getByText('/heartbeat')).toBeInTheDocument()
  })
})
