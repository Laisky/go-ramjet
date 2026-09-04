import { Mic, Phone } from 'lucide-react'
import { useCallback, useRef, useState } from 'react'
import { Button } from '@/components/ui/button'
import type { AudioPluginID, SessionConfig } from '../types'
import type { AudioPluginHandle, AudioPluginProps } from './plugin-types'
import { AudioPluginControl } from './audio-plugin-control'
import { AUDIO_PLUGIN_DEFINITIONS, resolveAudioPlugin } from './plugin-registry'
import { RealtimeAudioPlugin } from './realtime-plugin'

/** VoiceControlsProps adds session identity and preference persistence to the plugin boundary. */
interface VoiceControlsProps extends AudioPluginProps {
  sessionId?: string | number
  onConfigChange?: (updates: Partial<SessionConfig['chat_switch']>) => void
}

/** VoiceControls keeps the active phone call mounted independently of text-session preferences. */
export function VoiceControls({
  sessionId,
  onConfigChange,
  ...props
}: VoiceControlsProps) {
  const callRef = useRef<AudioPluginHandle>(null)
  const [callActive, setCallActive] = useState(false)
  const [dictationActive, setDictationActive] = useState(false)
  const plugin = resolveAudioPlugin(props.config.chat_switch.audio_plugin)
  const { onActivityChange } = props
  const callActivity = useCallback(
    (active: boolean) => {
      setCallActive(active)
      onActivityChange(active)
    },
    [onActivityChange, setCallActive],
  )
  const dictationActivity = useCallback(
    (active: boolean) => {
      setDictationActive(active)
      onActivityChange(active)
    },
    [onActivityChange, setDictationActive],
  )
  return (
    <>
      <Button
        size="sm"
        variant={callActive ? 'default' : 'outline'}
        aria-label="Voice"
        aria-pressed={
          callActive ||
          (plugin.id === 'whisper' && props.config.chat_switch.enable_talk)
        }
        disabled={props.disabled && !callActive && !dictationActive}
        onClick={() => {
          if (callActive) callRef.current?.reveal()
          else if (plugin.id === 'realtime') callRef.current?.start()
          else
            onConfigChange?.({
              enable_talk: !props.config.chat_switch.enable_talk,
            })
        }}
      >
        <Phone className="h-3 w-3" />
        {callActive ? 'In call' : 'Voice'}
      </Button>
      <select
        aria-label="Audio plugin"
        value={plugin.id}
        disabled={callActive || dictationActive}
        onChange={(event) =>
          onConfigChange?.({
            audio_plugin: event.target.value as AudioPluginID,
            enable_talk: false,
          })
        }
        className="h-8 rounded-md border border-border bg-background px-2 text-xs disabled:opacity-60"
      >
        {Object.values(AUDIO_PLUGIN_DEFINITIONS).map((definition) => (
          <option key={definition.id} value={definition.id}>
            {definition.label}
          </option>
        ))}
      </select>
      {plugin.id === 'whisper' &&
        props.config.chat_switch.enable_talk &&
        !callActive && (
          <span className="inline-flex items-center gap-1">
            <Mic className="h-3 w-3" />
            <AudioPluginControl
              key={sessionId}
              {...props}
              pluginID="whisper"
              onActivityChange={dictationActivity}
            />
          </span>
        )}
      <RealtimeAudioPlugin
        {...props}
        controlRef={callRef}
        sessionLabel={`${props.config.session_name || 'Chat session'} (${sessionId ?? 'current'})`}
        onActivityChange={callActivity}
      />
    </>
  )
}
