import { renderApp } from '@/test/render'
import { screen } from '@testing-library/react'
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

  it('renders GPTChatPage on chat.laisky.com', () => {
    setSiteMeta('chat')
    renderApp('/')

    expect(
      screen.queryByRole('heading', { name: 'go-ramjet' }),
    ).not.toBeInTheDocument()
    expect(document.title).toBe('Chat')
  })

  it('renders GPTChatPage on chat2.laisky.com', () => {
    setSiteMeta('chat')
    renderApp('/')

    expect(
      screen.queryByRole('heading', { name: 'go-ramjet' }),
    ).not.toBeInTheDocument()
    expect(document.title).toBe('Chat')
  })
})
