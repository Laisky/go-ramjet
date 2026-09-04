import type { SessionConfig, UserConfig } from '../types'
import type { RealtimeAudioState } from './realtime-client'

/** resolveRealtimeAPIBase selects an explicit session override before the account default. */
export function resolveRealtimeAPIBase(
  config: SessionConfig,
  user?: UserConfig,
): string {
  const configured = config.api_base?.trim() || ''
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
