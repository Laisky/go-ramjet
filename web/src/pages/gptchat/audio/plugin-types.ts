import type { ComponentType } from 'react'

import type {
  AudioPluginID,
  SessionConfig,
  UserConfig,
} from '../types'

/** AudioPluginProps defines the stable boundary between ChatInput and an audio backend. */
export interface AudioPluginProps {
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
