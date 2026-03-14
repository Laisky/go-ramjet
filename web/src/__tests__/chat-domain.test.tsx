import { renderApp } from '@/test/render'
import { screen, waitFor } from '@testing-library/react'
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

describe('chat domain proxying', () => {
  beforeEach(() => {
    setSiteMeta('default')
  })

  afterEach(() => {
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

  it('renders GPTChatPage on chat.laisky.com', async () => {
    setSiteMeta('chat')
    renderApp('/')

    expect(
      screen.queryByRole('heading', { name: 'go-ramjet' }),
    ).not.toBeInTheDocument()
    // GPTChatPage is lazy-loaded, so wait for its useEffect to set the title
    await waitFor(() => expect(document.title).toBe('Chat'), {
      timeout: 5000,
    })
  })

  it('renders GPTChatPage on chat2.laisky.com', async () => {
    setSiteMeta('chat')
    renderApp('/')

    expect(
      screen.queryByRole('heading', { name: 'go-ramjet' }),
    ).not.toBeInTheDocument()
    await waitFor(() => expect(document.title).toBe('Chat'), {
      timeout: 5000,
    })
  })
})
