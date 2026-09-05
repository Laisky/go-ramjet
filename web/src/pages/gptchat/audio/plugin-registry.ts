import type { AudioPluginID } from '../types'
import type { AudioPluginDefinition } from './plugin-types'
import { RealtimeAudioPlugin } from './realtime-plugin'
import { WhisperAudioPlugin } from './whisper-plugin'

export const AUDIO_PLUGIN_DEFINITIONS: Record<
  AudioPluginID,
  AudioPluginDefinition
> = {
  whisper: {
    id: 'whisper',
    label: 'Whisper',
    description: 'Record audio, transcribe it, then send editable text.',
    component: WhisperAudioPlugin,
  },
  realtime: {
    id: 'realtime',
    label: 'Realtime',
    description:
      'Talk directly with GPT-Realtime using native streaming audio.',
    component: RealtimeAudioPlugin,
  },
}

/**
 * resolveAudioPlugin returns the plugin the server selected.
 *
 * Realtime is the default, so an absent or unrecognized id resolves to it rather
 * than silently downgrading a deployment to dictation.
 */
export function resolveAudioPlugin(pluginID?: string): AudioPluginDefinition {
  if (pluginID === 'whisper') {
    return AUDIO_PLUGIN_DEFINITIONS.whisper
  }
  return AUDIO_PLUGIN_DEFINITIONS.realtime
}
