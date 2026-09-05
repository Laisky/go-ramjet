import type { SessionConfig, UserConfig } from '../types'
import type { RealtimeAudioState } from './realtime-client'

/** resolveRealtimeAPIBase selects an explicit session override before the account default. */
export function resolveRealtimeAPIBase(
  config: SessionConfig,
  user?: UserConfig,
): string {
  const configured = config.api_base?.trim() || ''
  if (config.token_type === 'direct')
    return configured || 'https://api.openai.com'
  if (configured && configured !== 'https://api.openai.com') {
    return configured
  }
  return user?.api_base?.trim() || configured
}

/** formatRealtimeStatus combines transport state with a compact native transcript preview. */
export function formatRealtimeStatus(
  state: RealtimeAudioState,
  transcript: string,
): string | null {
  const labels: Record<RealtimeAudioState, string | null> = {
    idle: null,
    ending: 'Ending call…',
    connecting: 'Connecting…',
    listening: 'Listening…',
    thinking: 'Thinking…',
    speaking: 'Speaking…',
  }
  const label = labels[state]
  if (!label) {
    return null
  }

  const compactTranscript = transcript.replace(/\s+/g, ' ').trim()
  if (!compactTranscript || (state !== 'speaking' && state !== 'listening')) {
    return label
  }
  const preview =
    compactTranscript.length > 84
      ? `…${compactTranscript.slice(-83)}`
      : compactTranscript
  return `${label} ${preview}`
}

/** FloatingPosition is the call window's viewport offset once it has been dragged. */
export interface FloatingPosition {
  x: number
  y: number
}

/** VIEWPORT_MARGIN keeps a dragged call window from resting flush against an edge. */
export const VIEWPORT_MARGIN = 8

/**
 * clampFloatingPosition keeps the dragged call window reachable.
 *
 * A window dropped past an edge, or left behind by a viewport that shrank,
 * would otherwise hide its own hang-up button with no way to bring it back.
 * The lower bound wins on a viewport too small to fit the window, so the header
 * stays visible rather than the window being pinned off the top-left.
 */
export function clampFloatingPosition(
  position: FloatingPosition,
  size: { width: number; height: number },
  viewport: { width: number; height: number },
): FloatingPosition {
  const limit = (value: number, extent: number, available: number) =>
    Math.min(
      Math.max(value, VIEWPORT_MARGIN),
      Math.max(VIEWPORT_MARGIN, available - extent - VIEWPORT_MARGIN),
    )
  return {
    x: limit(position.x, size.width, viewport.width),
    y: limit(position.y, size.height, viewport.height),
  }
}
