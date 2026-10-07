import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { CVPage } from './index'

/** contentResponse constructs a self-contained CV fixture without remote resources. */
function contentResponse() {
  return Response.json({
    content: '# Migration\n\nEngineer',
    is_default: false,
  })
}

describe('CV dependency lifecycle compatibility', () => {
  beforeEach(() => {
    localStorage.clear()
    window.history.replaceState({}, '', '/cv')
    vi.stubGlobal(
      'fetch',
      vi
        .fn<typeof fetch>()
        .mockImplementation(async (input) =>
          String(input) === '/cv/content'
            ? contentResponse()
            : Response.json({}),
        ),
    )
  })

  afterEach(async () => {
    cleanup()
    await act(async () => {})
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
    window.history.replaceState({}, '', '/')
  })

  it.each(['stored', 'redirect'])(
    'uses the %s token on the first content request',
    async (source) => {
      localStorage.setItem('cv_sso_token', 'stored-token')
      if (source === 'redirect')
        window.history.replaceState(
          {},
          '',
          '/cv?sso_token=redirect-token&keep=1',
        )
      render(<CVPage />)
      await screen.findByRole('heading', { name: 'Migration' })
      const calls = vi
        .mocked(fetch)
        .mock.calls.filter(([input]) => String(input) === '/cv/content')
      expect(calls).toHaveLength(1)
      expect(calls[0][1]?.headers).toEqual({
        Authorization: `Bearer ${source}-token`,
      })
      expect(window.location.search).not.toContain('sso_token')
      expect(localStorage.getItem('cv_sso_token')).toBe(`${source}-token`)
    },
  )

  it('aborts content loading and does not report failures after unmount', async () => {
    let reject!: (error: Error) => void
    const request = new Promise<Response>((_, rejectPromise) => {
      reject = rejectPromise
    })
    vi.mocked(fetch).mockImplementation((input) =>
      String(input) === '/cv/content'
        ? request
        : Promise.resolve(Response.json({})),
    )
    const warning = vi.spyOn(console, 'error').mockImplementation(() => {})
    const { unmount } = render(<CVPage />)
    const call = vi
      .mocked(fetch)
      .mock.calls.find(([input]) => String(input) === '/cv/content')
    unmount()
    expect(call?.[1]?.signal?.aborted).toBe(true)
    await act(async () => {
      reject(new Error('late transport failure'))
    })
    expect(warning).not.toHaveBeenCalled()
  })

  it('keeps copy feedback mounted across unrelated renders', async () => {
    vi.stubGlobal(
      'navigator',
      Object.create(navigator, {
        clipboard: {
          value: { writeText: vi.fn().mockResolvedValue(undefined) },
        },
      }),
    )
    const now = vi.spyOn(Date, 'now').mockReturnValue(1000)
    const { container, rerender } = render(<CVPage />)
    await screen.findByRole('heading', { name: 'Migration' })
    fireEvent.click(screen.getAllByTitle('Click to copy email')[0])
    await waitFor(() =>
      expect(container.querySelector('.cv-copy-feedback')).not.toBeNull(),
    )
    const feedback = container.querySelector('.cv-copy-feedback')
    now.mockReturnValue(2000)
    rerender(<CVPage />)
    expect(container.querySelector('.cv-copy-feedback')).toBe(feedback)
  })
})
