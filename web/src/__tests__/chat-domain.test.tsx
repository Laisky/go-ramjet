import { renderApp } from '@/test/render'
import { act, cleanup, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

/**
 * ensureMetaTag finds or creates a meta tag for the given name and returns it.
 */
function ensureMetaTag(name: string): HTMLMetaElement {
  const existing = document.querySelector<HTMLMetaElement>(
    `meta[name="${name}"]`,
  )
  if (existing) {
    return existing
  }
  const tag = document.createElement('meta')
  tag.setAttribute('name', name)
  document.head.appendChild(tag)
  return tag
}

/**
 * setSiteMeta sets the site and theme metadata for the current test document using siteId and returns no value.
 */
function setSiteMeta(siteId: string) {
  ensureMetaTag('ramjet-site').setAttribute('content', siteId)
  ensureMetaTag('ramjet-theme').setAttribute('content', siteId)
  document.documentElement.dataset.site = siteId
  document.documentElement.dataset.theme = siteId
}

// The first GPTChatPage render compiles the whole lazy-loaded page tree,
// which can take several seconds when the full suite runs in parallel. The
// test timeout must exceed both waits in waitForChatPageLoaded, or a slow
// load times out the test before waitFor can report it.
const LAZY_PAGE_TIMEOUT_MS = 10_000
const LAZY_PAGE_TEST_TIMEOUT_MS = 25_000

/**
 * waitForChatPageLoaded waits until GPTChatPage has rendered and finished
 * loading its session config. Ending a test while that load is pending lets
 * it set state after the test file's jsdom environment is torn down, which
 * Vitest reports as an unhandled "window is not defined" rejection.
 */
async function waitForChatPageLoaded() {
  // GPTChatPage is lazy-loaded, so wait for its useEffect to set the title
  await waitFor(() => expect(document.title).toBe('Chat'), {
    timeout: LAZY_PAGE_TIMEOUT_MS,
  })
  await waitFor(
    () =>
      expect(
        screen.queryByRole('status', { name: 'Loading chat session' }),
      ).not.toBeInTheDocument(),
    { timeout: LAZY_PAGE_TIMEOUT_MS },
  )
}

describe('chat domain proxying', () => {
  beforeEach(() => {
    setSiteMeta('default')
    document.title = ''
    // Routing tests use deterministic version responses, never live requests.
    vi.stubGlobal(
      'fetch',
      vi
        .fn<typeof fetch>()
        .mockImplementation(async () => Response.json({ Settings: [] })),
    )
  })

  afterEach(async () => {
    cleanup()
    await act(async () => {})
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('renders HomePage on localhost', () => {
    renderApp('/')
    expect(
      screen.getByRole('heading', { name: 'go-ramjet' }),
    ).toBeInTheDocument()
    expect(document.title).toBe('Laisky')
  })

  it(
    'renders GPTChatPage on chat.laisky.com',
    async () => {
      setSiteMeta('chat')
      renderApp('/')

      expect(
        screen.queryByRole('heading', { name: 'go-ramjet' }),
      ).not.toBeInTheDocument()
      await waitForChatPageLoaded()
    },
    LAZY_PAGE_TEST_TIMEOUT_MS,
  )

  it(
    'renders GPTChatPage on chat2.laisky.com',
    async () => {
      setSiteMeta('chat')
      renderApp('/')

      expect(
        screen.queryByRole('heading', { name: 'go-ramjet' }),
      ).not.toBeInTheDocument()
      await waitForChatPageLoaded()
    },
    LAZY_PAGE_TEST_TIMEOUT_MS,
  )
})
