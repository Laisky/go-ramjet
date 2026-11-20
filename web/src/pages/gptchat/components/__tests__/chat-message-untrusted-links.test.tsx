import { render } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import type { ChatMessageData } from '../../types'
import { ChatMessage } from '../chat-message'

/**
 * SCRIPT_SCHEME matches a URL a browser would run as script once leading
 * controls and spaces and embedded tabs and newlines are ignored.
 */
const SCRIPT_SCHEME = /^(?:javascript|vbscript):/i

/** urlForCheck normalizes value the way a browser reads a URL attribute. */
function urlForCheck(value: string): string {
  // eslint-disable-next-line no-control-regex
  return value.replace(/^[\u0000- ]+/, '').replace(/[\t\n\r]/g, '')
}

describe('ChatMessage untrusted links', () => {
  it('never renders a runnable script URL from upstream citations', () => {
    // Reference URLs come straight from upstream url_citation annotations
    // and are rendered outside the markdown sanitizer. React 19 replaces a
    // javascript: href with a URL that only throws, which this pins.
    const message: ChatMessageData = {
      chatID: 'refs',
      role: 'assistant',
      content: 'See the sources.',
      references: [
        { index: 1, url: 'javascript:window.__xss=1', title: 'one' },
        { index: 2, url: ' JavaScript:window.__xss=1', title: 'two' },
        { index: 3, url: 'https://example.com/source', title: 'three' },
      ],
    }

    const { container } = render(<ChatMessage message={message} />)

    const anchors = Array.from(container.querySelectorAll('a[href]'))
    expect(anchors.length).toBeGreaterThanOrEqual(3)
    for (const anchor of anchors) {
      const href = urlForCheck(anchor.getAttribute('href') ?? '')
      if (SCRIPT_SCHEME.test(href)) {
        expect(href).toContain('React has blocked a javascript: URL')
      }
      if (anchor.getAttribute('target') === '_blank') {
        expect(anchor.getAttribute('rel') ?? '').toMatch(/noreferrer|noopener/)
      }
    }
    expect(
      container.querySelector('a[href="https://example.com/source"]'),
    ).not.toBeNull()
  })
})
