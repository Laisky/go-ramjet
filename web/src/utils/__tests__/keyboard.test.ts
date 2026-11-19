import { describe, expect, it } from 'vitest'
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
    [{ platform: 'Win32' }, 'Ctrl'],
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

  it('prefers client hints over a stale navigator.platform', () => {
    expect(
      isApplePlatform({
        platform: 'Win32',
        userAgentData: { platform: 'macOS' },
      }),
    ).toBe(true)
  })
})
