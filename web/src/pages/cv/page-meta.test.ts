import {
  LEGACY_CV_FAVICON_URL,
  mergeCVPageMeta,
  resolveCVPageMeta,
} from '@/pages/cv/page-meta'
import { beforeEach, describe, expect, it } from 'vitest'

describe('resolveCVPageMeta', () => {
  beforeEach(() => {
    document.head.innerHTML = ''
  })

  it('prefers document favicon and og:image when present', () => {
    document.head.innerHTML = `
      <link rel="icon" href="https://s3.laisky.com/public/favicon.ico" />
      <meta property="og:image" content="https://s3.laisky.com/public/cover.png" />
    `

    const meta = resolveCVPageMeta()
    expect(meta.faviconHref).toBe('https://s3.laisky.com/public/favicon.ico')
    expect(meta.ogImage).toBe('https://s3.laisky.com/public/cover.png')
  })

  it('uses favicon as og:image when og:image is missing', () => {
    document.head.innerHTML =
      '<link rel="icon" href="https://s3.laisky.com/public/favicon.ico" />'

    const meta = resolveCVPageMeta()
    expect(meta.faviconHref).toBe('https://s3.laisky.com/public/favicon.ico')
    expect(meta.ogImage).toBe('https://s3.laisky.com/public/favicon.ico')
  })

  it('falls back to legacy favicon when no metadata exists', () => {
    const meta = resolveCVPageMeta()
    expect(meta.faviconHref).toBe(LEGACY_CV_FAVICON_URL)
    expect(meta.ogImage).toBe(LEGACY_CV_FAVICON_URL)
  })

  it('uses provided fallback when document metadata is empty', () => {
    const fallback = 'https://example.com/custom.ico'
    const meta = resolveCVPageMeta(fallback)
    expect(meta.faviconHref).toBe(fallback)
    expect(meta.ogImage).toBe(fallback)
  })
})

describe('mergeCVPageMeta', () => {
  it('overrides both fields when payload provides both', () => {
    const merged = mergeCVPageMeta(
      {
        faviconHref: LEGACY_CV_FAVICON_URL,
        ogImage: LEGACY_CV_FAVICON_URL,
      },
      {
        favicon: 'https://s3.laisky.com/public/favicon.ico',
        og_image: 'https://s3.laisky.com/public/og.png',
      },
    )

    expect(merged.faviconHref).toBe('https://s3.laisky.com/public/favicon.ico')
    expect(merged.ogImage).toBe('https://s3.laisky.com/public/og.png')
  })

  it('uses payload favicon as og:image when payload og:image is absent', () => {
    const merged = mergeCVPageMeta(
      {
        faviconHref: LEGACY_CV_FAVICON_URL,
        ogImage: LEGACY_CV_FAVICON_URL,
      },
      {
        favicon: 'https://s3.laisky.com/public/favicon.ico',
      },
    )

    expect(merged.faviconHref).toBe('https://s3.laisky.com/public/favicon.ico')
    expect(merged.ogImage).toBe('https://s3.laisky.com/public/favicon.ico')
  })

  it('keeps current values when payload is empty', () => {
    const merged = mergeCVPageMeta(
      {
        faviconHref: 'https://s3.laisky.com/public/favicon.ico',
        ogImage: 'https://s3.laisky.com/public/og.png',
      },
      {},
    )

    expect(merged.faviconHref).toBe('https://s3.laisky.com/public/favicon.ico')
    expect(merged.ogImage).toBe('https://s3.laisky.com/public/og.png')
  })
})
