import { afterEach, describe, expect, it, vi } from 'vitest'
import { isApplePlatform, primaryModifierLabel } from '../keyboard'

describe('primaryModifierLabel', () => {
  it.each([
    [{ platform: 'MacIntel' }, 'Cmd'],
    [{ platform: 'iPad' }, 'Cmd'],
    [{ platform: '', userAgentData: { platform: 'macOS' } }, 'Cmd'],
    [
      {
        platform: '',
        userAgent:
          'Mozilla/5.0 (Macintosh; Intel Mac OS X 15_0) AppleWebKit/605.1.15',
      },
      'Cmd',
    ],
    [{ platform: 'iPhone' }, 'Cmd'],
    // iPadOS Safari requests desktop sites and reports itself as a Mac; iPad
    // hardware keyboards use Command, so Cmd is still the right label.
    [
      {
        platform: 'MacIntel',
        userAgent:
          'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Safari/605.1.15',
      },
      'Cmd',
    ],
    [{ platform: 'Win32' }, 'Ctrl'],
    [
      { platform: 'Linux armv8l', userAgentData: { platform: 'Android' } },
      'Ctrl',
    ],
    [
      { platform: 'Linux x86_64', userAgentData: { platform: 'Chrome OS' } },
      'Ctrl',
    ],
    [{ platform: 'Linux x86_64' }, 'Ctrl'],
    [
      {
        platform: '',
        userAgent:
          'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/157.0 Safari/537.36',
      },
      'Ctrl',
    ],
    [{}, 'Ctrl'],
  ])('maps %j to %s', (source, expected) => {
    expect(primaryModifierLabel(source)).toBe(expected)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('falls back to Ctrl without throwing when there is no navigator', () => {
    vi.stubGlobal('navigator', undefined)
    expect(primaryModifierLabel()).toBe('Ctrl')
  })

  it('prefers client hints over a stale navigator.platform', () => {
    expect(
      isApplePlatform({
        platform: 'Win32',
        userAgentData: { platform: 'macOS' },
      }),
    ).toBe(true)
  })
})
