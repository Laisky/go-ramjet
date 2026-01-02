import { renderApp } from '@/test/render'
import { screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

describe('chat domain proxying', () => {
  beforeEach(() => {
    vi.stubGlobal('location', {
      ...window.location,
      hostname: 'localhost',
    })
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
    vi.stubGlobal('location', {
      ...window.location,
      hostname: 'chat.laisky.com',
    })
    renderApp('/')

    expect(
      screen.queryByRole('heading', { name: 'go-ramjet' }),
    ).not.toBeInTheDocument()
    expect(document.title).toBe('Chat')
  })

  it('renders GPTChatPage on chat2.laisky.com', () => {
    vi.stubGlobal('location', {
      ...window.location,
      hostname: 'chat2.laisky.com',
    })
    renderApp('/')

    expect(
      screen.queryByRole('heading', { name: 'go-ramjet' }),
    ).not.toBeInTheDocument()
    expect(document.title).toBe('Chat')
  })
})
