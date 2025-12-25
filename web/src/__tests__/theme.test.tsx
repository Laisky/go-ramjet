import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { renderApp } from '@/test/render'

describe('theme', () => {
  it('toggles theme through menu options', async () => {
    const user = userEvent.setup()
    renderApp('/')

    const toggle = screen.getByRole('button', { name: /toggle theme/i })
    await user.click(toggle)

    await user.click(screen.getByText('Dark'))
    expect(document.documentElement.classList.contains('dark')).toBe(true)

    await user.click(toggle)
    await user.click(screen.getByText('Light'))
    expect(document.documentElement.classList.contains('dark')).toBe(false)

    await user.click(toggle)
    await user.click(screen.getByText('System'))
    expect(window.localStorage.getItem('theme')).toBe('system')
  })
})
