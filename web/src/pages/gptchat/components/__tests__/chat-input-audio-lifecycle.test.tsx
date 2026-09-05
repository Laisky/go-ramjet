import { useEffect } from 'react'
import { render } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import { DefaultSessionConfig } from '../../types'
import { ChatInput } from '../chat-input'

const audioCleanup = vi.hoisted(() => vi.fn())

// The dictation control only renders when the SERVER selects the whisper plugin,
// so this deployment config is what puts the plugin under test on screen at all.
vi.mock('../../hooks/use-user', () => ({
  useUser: () => ({
    user: {
      is_free: false,
      byok: true,
      api_base: 'https://proxy.example.com',
      voice: { plugin: 'whisper' },
    },
  }),
}))

vi.mock('../../audio/audio-plugin-control', () => ({
  AudioPluginControl: () => {
    useEffect(() => audioCleanup, [])
    return <button type="button">Mock audio client</button>
  },
}))

vi.mock('../message-input', () => ({
  MessageInput: () => <textarea aria-label="Message" />,
}))

describe('ChatInput audio lifecycle regressions', () => {
  it('remounts and cleans up the audio plugin when the dictation session changes', () => {
    audioCleanup.mockReset()
    const config = {
      ...DefaultSessionConfig,
      chat_switch: {
        ...DefaultSessionConfig.chat_switch,
        enable_talk: true,
      },
    }
    const props = {
      config,
      onSend: vi.fn(),
      onConfigChange: vi.fn(),
    }

    const rendered = render(<ChatInput {...props} sessionId="session-a" />)
    expect(audioCleanup).not.toHaveBeenCalled()

    rendered.rerender(<ChatInput {...props} sessionId="session-b" />)

    expect(audioCleanup).toHaveBeenCalledTimes(1)
  })
})
