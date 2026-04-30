import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { CVPage } from './index'

/**
 * createJSONResponse builds a JSON fetch response for CV page tests.
 */
function createJSONResponse(payload: unknown) {
  return new Response(JSON.stringify(payload), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
  })
}

describe('CVPage history modal', () => {
  beforeEach(() => {
    window.localStorage.clear()
    document.head.innerHTML = ''
  })

  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('loads persistent history and switches the editor to the selected saved version', async () => {
    const user = userEvent.setup()
    window.localStorage.setItem('cv_sso_token', 'test-token')

    const fetchMock = vi.fn(
      async (input: RequestInfo | URL, init?: RequestInit) => {
        const url =
          typeof input === 'string'
            ? input
            : input instanceof URL
              ? input.toString()
              : input.url

        if (url === '/cv/content') {
          return createJSONResponse({
            content: 'latest content',
            updated_at: '2026-04-30T10:00:00Z',
            is_default: false,
          })
        }

        if (url === '/cv/meta') {
          return createJSONResponse({})
        }

        if (url === '/cv/content/history') {
          expect(init).toMatchObject({
            headers: { Authorization: 'Bearer test-token' },
          })

          return createJSONResponse({
            items: [
              {
                version_id: 'v3',
                updated_at: '2026-04-30T10:00:00Z',
                is_latest: true,
              },
              {
                version_id: 'v2',
                updated_at: '2026-04-29T09:00:00Z',
                is_latest: false,
              },
            ],
          })
        }

        if (url === '/cv/content/version?version_id=v2') {
          return createJSONResponse({
            content: 'older content',
            version_id: 'v2',
          })
        }

        throw new Error(`Unhandled fetch: ${url}`)
      },
    )
    vi.stubGlobal('fetch', fetchMock)

    render(<CVPage />)

    await user.click(await screen.findByRole('button', { name: 'Edit' }))

    const textarea = await screen.findByRole('textbox')
    expect(textarea).toHaveValue('latest content')

    const historySelect = await screen.findByLabelText('Saved history')
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        '/cv/content/history',
        expect.objectContaining({
          headers: { Authorization: 'Bearer test-token' },
        }),
      )
    })

    await user.selectOptions(historySelect, 'v2')

    await waitFor(() => {
      expect(screen.getByRole('textbox')).toHaveValue('older content')
    })
  })
})
