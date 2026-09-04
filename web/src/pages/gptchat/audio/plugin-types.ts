import type { ComponentType, Ref } from 'react'

import type { AudioPluginID, SessionConfig, UserConfig } from '../types'

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
