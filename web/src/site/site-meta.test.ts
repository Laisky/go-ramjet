import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { bootstrapSite, resolveSiteId } from './site-meta'

describe('site-meta', () => {
  const originalEnv = process.env

  beforeEach(() => {
    vi.resetModules()
    document.documentElement.removeAttribute('data-site')
    document.documentElement.removeAttribute('data-theme')
    // Reset import.meta.env mock if needed (vitest usually handles it via vi.stubEnv or similar, but here we might rely on document logic more)
  })

  afterEach(() => {
    process.env = originalEnv
  })

  it('bootstrapSite sets data attributes on document element', () => {
    // Mock environment if possible, or rely on defaults
    // Since we can't easily mock import.meta.env in simple way without setup,
    // we focus on DOM interaction.

    // If no env/meta, defaults to 'default'
    bootstrapSite()
    expect(document.documentElement.getAttribute('data-site')).toBe('default')
    expect(document.documentElement.getAttribute('data-theme')).toBe('default')
  })

  it('resolveSiteId reads from meta tag if present', () => {
    const meta = document.createElement('meta')
    meta.name = 'ramjet-site'
    meta.content = 'test-site'
    document.head.appendChild(meta)

    expect(resolveSiteId()).toBe('test-site')

    document.head.removeChild(meta)
  })
})
