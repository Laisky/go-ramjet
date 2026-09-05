import type { ComponentType, Ref } from 'react'

import type {
  AudioPluginID,
  ChatMessageData,
  SessionConfig,
  UserConfig,
} from '../types'

/** AudioPluginProps defines the stable boundary between ChatInput and an audio backend. */
export interface AudioPluginProps {
  controlRef?: Ref<AudioPluginHandle>
  sessionLabel?: string
  config: SessionConfig
  user?: UserConfig
  disabled: boolean
  onDraftText: (text: string) => void
  onError: (message: string | null) => void
  onBusyChange: (busy: boolean) => void
  onActivityChange: (active: boolean) => void
  onStatusChange: (status: string | null) => void
  /** sessionId identifies the text session a call started from, pinned for its lifetime. */
  sessionId?: number
  /**
   * onCallSessionChange reports the session a live call is recording into, or null
   * when none is. It reports the id pinned at call start, never the session the
   * user happens to be viewing.
   */
  onCallSessionChange?: (pinnedSessionId: number | null) => void
  /**
   * onVoiceMessage writes one voice turn into the pinned session's chat history.
   * persist=false streams a growing turn into the view; persist=true commits it.
   */
  onVoiceMessage?: (
    targetSessionId: number,
    message: ChatMessageData,
    persist: boolean,
  ) => void
}

/** AudioPluginDefinition describes one selectable GPTChat audio implementation. */
export interface AudioPluginDefinition {
  id: AudioPluginID
  label: string
  description: string
  component: ComponentType<AudioPluginProps>
}

/** AudioPluginHandle exposes explicit user-gesture call controls without relying on saved preferences. */
export interface AudioPluginHandle {
  start: () => void
  reveal: () => void
}
