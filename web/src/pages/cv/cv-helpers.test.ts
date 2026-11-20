import { describe, expect, it } from 'vitest'

import { parseCvContent } from './cv-helpers'

/**
 * cvWithLinks builds minimal CV markdown containing the given URLs.
 *
 * @param urls - URLs to place in the contact line.
 * @returns CV markdown text.
 */
function cvWithLinks(...urls: string[]): string {
  return `# Jane Doe\nRemote\n\n## Contact\n${urls.join(' | ')}\n`
}

describe('parseCvContent link extraction', () => {
  it('labels well-known profile links', () => {
    const { links, email } = parseCvContent(
      `${cvWithLinks(
        'https://www.linkedin.com/in/jane',
        'https://github.com/jane',
        'https://blog.example.com/',
        'https://example.org/portfolio',
      )}Email: jane@example.com\n`,
    )

    expect(links).toEqual([
      { label: 'LinkedIn', href: 'https://www.linkedin.com/in/jane' },
      { label: 'GitHub', href: 'https://github.com/jane' },
      { label: 'Blog', href: 'https://blog.example.com/' },
      { label: 'Link', href: 'https://example.org/portfolio' },
      { label: 'Email', href: 'mailto:jane@example.com' },
    ])
    expect(email).toBe('jane@example.com')
  })

  // CodeQL alerts #91, #92 and #93 flag the substring checks as incomplete URL
  // sanitization. They are label heuristics over owner-authored content (PUT
  // /cv/content is owner-only), not an allowlist: every http(s) URL becomes a
  // link with its exact href whatever the check returns, so a crafted host
  // changes only the display label.
  it.each([
    'https://github.com.evil.example/',
    'https://evil.example/?github.com',
    'https://evilgithub.com/',
    'https://linkedin.com.evil.example/',
    'https://evil.example/?linkedin.com',
  ])('never gates whether %s is linked or rewrites its href', (url) => {
    const { links } = parseCvContent(cvWithLinks(url))

    expect(links).toHaveLength(1)
    expect(links[0].href).toBe(url)
  })

  it('links a non-matching host exactly like a matching one', () => {
    const matching = parseCvContent(cvWithLinks('https://evilgithub.com/'))
    const plain = parseCvContent(cvWithLinks('https://evil.example/'))

    expect(matching.links.map((link) => link.href)).toEqual([
      'https://evilgithub.com/',
    ])
    expect(plain.links.map((link) => link.href)).toEqual([
      'https://evil.example/',
    ])
  })

  it('only extracts http(s) URLs, so no script URL can become a link', () => {
    const { links } = parseCvContent(
      cvWithLinks(
        'javascript:alert(1)//github.com',
        'data:text/html,github.com',
      ),
    )

    expect(links).toEqual([])
  })

  it('keeps only the first GitHub-labelled link', () => {
    const { links } = parseCvContent(
      cvWithLinks('https://github.com/jane', 'https://github.com/jane/repo'),
    )

    expect(links).toEqual([
      { label: 'GitHub', href: 'https://github.com/jane' },
    ])
  })
})
