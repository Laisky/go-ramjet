import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { CVPage } from './index'

/**
 * jsonResponse builds a JSON fetch response with the given status.
 */
function jsonResponse(payload: unknown, status = 200) {
  return new Response(JSON.stringify(payload), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

/**
 * requestURL normalizes a fetch input to its URL string.
 */
function requestURL(input: RequestInfo | URL): string {
  if (typeof input === 'string') return input
  if (input instanceof URL) return input.toString()
  return input.url
}

type SessionReply = { status: number; body: unknown }

/**
 * stubCVBackend answers the public CV routes and replies to the session check
 * with sessionReply. It returns the fetch mock and records the browser URL seen
 * by the first request.
 */
function stubCVBackend(sessionReply: SessionReply, saveStatus = 200) {
  const seen: { firstRequestURL: string | null } = { firstRequestURL: null }
  const fetchMock = vi.fn(
    async (input: RequestInfo | URL, init?: RequestInit) => {
      seen.firstRequestURL ??= window.location.href
      const url = requestURL(input)
      if (url === '/cv/content' && (!init?.method || init.method === 'GET')) {
        return jsonResponse({
          content: '# CV',
          updated_at: '2026-10-07T00:00:00Z',
          is_default: false,
        })
      }
      if (url === '/cv/meta') return jsonResponse({})
      if (url === '/cv/auth/session')
        return jsonResponse(sessionReply.body, sessionReply.status)
      if (url === '/cv/content/history') return jsonResponse({ items: [] })
      if (url === '/cv/content' && init?.method === 'PUT') {
        return saveStatus === 200
          ? jsonResponse({
              content: '# CV edited',
              updated_at: '2026-10-07T01:00:00Z',
            })
          : jsonResponse({ code: 'cv_owner_required' }, saveStatus)
      }
      throw new Error(`Unhandled fetch: ${url}`)
    },
  )
  vi.stubGlobal('fetch', fetchMock)
  return { fetchMock, seen }
}

// slowUI gives async session checks room when the whole suite runs in parallel.
const slowUI = { timeout: 5000 }

const ownerSession: SessionReply = {
  status: 200,
  body: {
    uid: '0199b2a4-0000-7000-8000-00000000c0de',
    username: 'owner',
    owner: true,
  },
}

describe('CVPage SSO owner session', () => {
  beforeEach(() => {
    window.localStorage.clear()
    document.head.innerHTML = ''
    window.history.replaceState({}, '', '/')
  })

  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
    window.history.replaceState({}, '', '/')
  })

  it('consumes the callback token before any request and enables editing for the verified owner', async () => {
    window.history.replaceState({}, '', '/?sso_token=owner-jwt&tab=cv')
    const { fetchMock, seen } = stubCVBackend(ownerSession)

    render(<CVPage />)

    expect(
      await screen.findByRole('button', { name: 'Edit' }, slowUI),
    ).toBeInTheDocument()
    expect(seen.firstRequestURL).not.toContain('sso_token')
    expect(window.location.search).toBe('?tab=cv')
    expect(window.localStorage.getItem('cv_sso_token')).toBe('owner-jwt')
    expect(fetchMock).toHaveBeenCalledWith(
      '/cv/auth/session',
      expect.objectContaining({
        headers: { Authorization: 'Bearer owner-jwt' },
      }),
    )
  })

  it('does not offer editing just because a token is present', async () => {
    window.localStorage.setItem('cv_sso_token', 'pending-jwt')
    let release: (value: Response) => void = () => {}
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = requestURL(input)
      if (url === '/cv/auth/session') {
        return new Promise<Response>((resolve) => {
          release = resolve
        })
      }
      if (url === '/cv/content')
        return jsonResponse({ content: '# CV', is_default: false })
      return jsonResponse({})
    })
    vi.stubGlobal('fetch', fetchMock)

    render(<CVPage />)

    await waitFor(() =>
      expect(fetchMock).toHaveBeenCalledWith(
        '/cv/auth/session',
        expect.anything(),
      ),
    )
    expect(
      screen.queryByRole('button', { name: 'Edit' }),
    ).not.toBeInTheDocument()
    release(jsonResponse(ownerSession.body))
    expect(
      await screen.findByRole('button', { name: 'Edit' }, slowUI),
    ).toBeInTheDocument()
  })

  it('tells a signed-in non-owner that the account cannot edit and offers a different sign-in', async () => {
    window.localStorage.setItem('cv_sso_token', 'visitor-jwt')
    stubCVBackend({ status: 403, body: { code: 'cv_owner_required' } })

    render(<CVPage />)

    expect(
      await screen.findByText(
        /this account is not allowed to edit the cv/i,
        undefined,
        slowUI,
      ),
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Edit' }),
    ).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: /login/i })).toBeInTheDocument()
    expect(window.localStorage.getItem('cv_sso_token')).toBeNull()
  })

  it('asks to sign in again when SSO rejects the session', async () => {
    window.localStorage.setItem('cv_sso_token', 'expired-jwt')
    stubCVBackend({ status: 401, body: { code: 'sso_session_invalid' } })

    render(<CVPage />)

    expect(
      await screen.findByText(
        /sso session is invalid or has expired/i,
        undefined,
        slowUI,
      ),
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Edit' }),
    ).not.toBeInTheDocument()
    expect(window.localStorage.getItem('cv_sso_token')).toBeNull()
  })

  it('keeps the session but explains when SSO is unavailable', async () => {
    window.localStorage.setItem('cv_sso_token', 'owner-jwt')
    stubCVBackend({ status: 503, body: { code: 'sso_unavailable' } })

    render(<CVPage />)

    expect(
      await screen.findByText(
        /sso is temporarily unavailable/i,
        undefined,
        slowUI,
      ),
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Edit' }),
    ).not.toBeInTheDocument()
    expect(window.localStorage.getItem('cv_sso_token')).toBe('owner-jwt')
  })

  it('reports a rejected save as a permission problem instead of an expired token', async () => {
    const user = userEvent.setup()
    window.localStorage.setItem('cv_sso_token', 'owner-jwt')
    stubCVBackend(ownerSession, 403)

    render(<CVPage />)
    await user.click(
      await screen.findByRole('button', { name: 'Edit' }, slowUI),
    )
    const textarea = await screen.findByRole('textbox')
    await waitFor(() => expect(textarea).toHaveValue('# CV'))
    await user.type(textarea, ' edited')
    await user.click(screen.getByRole('button', { name: /save/i }))

    expect(
      await screen.findByText(
        /this account is not allowed to edit the cv/i,
        undefined,
        slowUI,
      ),
    ).toBeInTheDocument()
    expect(screen.queryByText(/expired/i)).not.toBeInTheDocument()
  })
})
