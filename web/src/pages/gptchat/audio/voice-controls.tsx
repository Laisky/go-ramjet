import { Mic, Phone } from 'lucide-react'
import { useCallback, useRef, useState } from 'react'
import {
  TOOLBAR_BUTTON_LAYOUT,
  TOOLBAR_ICON_CLASS,
  toolbarControlClasses,
} from '../components/toolbar-control'
import type { SessionConfig } from '../types'
import type { AudioPluginHandle, AudioPluginProps } from './plugin-types'
import { AudioPluginControl } from './audio-plugin-control'
import { resolveAudioPlugin } from './plugin-registry'
import { RealtimeAudioPlugin } from './realtime-plugin'

/** VoiceControlsProps adds session identity and preference persistence to the plugin boundary. */
interface VoiceControlsProps extends AudioPluginProps {
  onConfigChange?: (updates: Partial<SessionConfig['chat_switch']>) => void
}

/** VoiceControls keeps the active phone call mounted independently of text-session preferences. */
export function VoiceControls({
  sessionId,
  onConfigChange,
  onCallSessionChange,
  ...props
}: VoiceControlsProps) {
  const callRef = useRef<AudioPluginHandle>(null)
  const [callActive, setCallActive] = useState(false)
  const [dictationActive, setDictationActive] = useState(false)
  // The server owns this choice; there is deliberately no picker in the UI.
  const plugin = resolveAudioPlugin(props.user?.voice?.plugin)
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
      <button
        type="button"
        className={toolbarControlClasses(
          callActive ? 'active' : 'idle',
          TOOLBAR_BUTTON_LAYOUT,
        )}
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
        <Phone className={TOOLBAR_ICON_CLASS} />
        <span className="hidden sm:inline">
          {callActive ? 'In call' : 'Voice'}
        </span>
      </button>
      {plugin.id === 'whisper' &&
        props.config.chat_switch.enable_talk &&
        !callActive && (
          <span className="inline-flex items-center gap-1">
            <Mic className={TOOLBAR_ICON_CLASS} />
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
        sessionId={sessionId}
        onCallSessionChange={onCallSessionChange}
        controlRef={callRef}
        sessionLabel={`${props.config.session_name || 'Chat session'} (${sessionId ?? 'current'})`}
        onActivityChange={callActivity}
      />
    </>
  )
}
