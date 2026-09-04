import type { AudioPluginID } from '../types'
import type {
  AudioPluginDefinition,
  AudioPluginProps,
} from './plugin-types'
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
    description: 'Talk directly with GPT-Realtime using native streaming audio.',
    component: RealtimeAudioPlugin,
  },
}

/** resolveAudioPlugin returns a valid plugin definition with a legacy-safe fallback. */
export function resolveAudioPlugin(
  pluginID?: string,
): AudioPluginDefinition {
  if (pluginID === 'realtime') {
    return AUDIO_PLUGIN_DEFINITIONS.realtime
  }
  return AUDIO_PLUGIN_DEFINITIONS.whisper
}

interface AudioPluginControlProps extends AudioPluginProps {
  pluginID?: string
}

/** AudioPluginControl renders the selected implementation behind one stable contract. */
export function AudioPluginControl({
  pluginID,
  ...props
}: AudioPluginControlProps) {
  const definition = resolveAudioPlugin(pluginID)
  const Component = definition.component
  return <Component {...props} />
}
