/**
 * Keyboard helpers shared by components that advertise shortcuts, so every
 * hint names the modifier the user actually presses on their platform.
 */

/**
 * NavigatorPlatformSource is the subset of Navigator used to detect the
 * platform. userAgentData is optional because only Chromium exposes it.
 */
export interface NavigatorPlatformSource {
  platform?: string
  userAgent?: string
  userAgentData?: { platform?: string }
}

/**
 * isApplePlatform reports whether source describes macOS or iOS, where the
 * primary shortcut modifier is Command instead of Control. It prefers the
 * User-Agent Client Hints platform, then navigator.platform, then the user
 * agent string, and returns false when nothing is known.
 */
export function isApplePlatform(
  source: NavigatorPlatformSource | undefined = globalThis.navigator,
): boolean {
  if (!source) {
    return false
  }
  const platform =
    source.userAgentData?.platform || source.platform || source.userAgent || ''
  return /mac|iphone|ipad|ipod/i.test(platform)
}

/**
 * primaryModifierLabel returns the label of the modifier used for app
 * shortcuts on source's platform: "Cmd" on Apple platforms, "Ctrl" elsewhere.
 * It defaults to the current browser's navigator.
 */
export function primaryModifierLabel(
  source?: NavigatorPlatformSource,
): 'Cmd' | 'Ctrl' {
  return isApplePlatform(source) ? 'Cmd' : 'Ctrl'
}
