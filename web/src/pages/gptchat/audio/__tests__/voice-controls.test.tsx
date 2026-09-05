import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { DefaultSessionConfig, type UserConfig } from '../../types'
import {
  TOOLBAR_BUTTON_LAYOUT,
  toolbarControlClasses,
} from '../../components/toolbar-control'
import { VoiceControls } from '../voice-controls'
import { deferred, FakeSocket, flush, installMedia } from './media-fixtures'

const account: UserConfig = {
  user_name: 'test',
  token: '',
  openai_token: '',
  image_token: '',
  is_free: true,
  byok: false,
  is_admin: false,
  allowed_models: ['*'],
  no_limit_expensive_models: false,
  api_base: 'https://gateway.example.com',
  image_url: '',
}
/** props returns stable callbacks and an explicitly configured account for a voice control. */
function props() {
  return {
    config: {
      ...DefaultSessionConfig,
      api_token: 'account-a-token',
      session_name: 'Session A',
      chat_switch: { ...DefaultSessionConfig.chat_switch, enable_talk: true },
    },
    sessionId: 1,
    user: account,
    disabled: false,
    onDraftText: vi.fn(),
    onError: vi.fn(),
    onBusyChange: vi.fn(),
    onActivityChange: vi.fn(),
    onStatusChange: vi.fn(),
    onConfigChange: vi.fn(),
    onCallSessionChange: vi.fn(),
  }
}
/** startCall uses the visible Voice button and both public transport handshakes. */
async function startCall() {
  fireEvent.click(screen.getByRole('button', { name: 'Voice' }))
  await act(flush)
  const socket = FakeSocket.instances.at(-1)!
  await act(async () => {
    socket.open()
    socket.event({ type: 'session.updated' })
    await flush()
  })
  return socket
}
let media: ReturnType<typeof installMedia>
beforeEach(() => {
  media = installMedia()
})
afterEach(async () => {
  cleanup()
  await act(flush)
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

describe('Composer toolbar consistency', () => {
  it("gives the call and dictation controls the toggles' type scale", () => {
    render(<VoiceControls {...props()} />)

    // These must match the ToggleButton row in chat-input.tsx. Styling them with the
    // generic Button component instead left them at 14px beside 11px neighbours.
    const idle = toolbarControlClasses('idle', TOOLBAR_BUTTON_LAYOUT)
    expect(screen.getByRole('button', { name: 'Voice' }).className).toBe(idle)
  })

  it('offers no plugin picker, because the server owns that choice', () => {
    render(<VoiceControls {...props()} />)

    expect(
      screen.queryByRole('combobox', { name: 'Audio plugin' }),
    ).not.toBeInTheDocument()
  })

  it('marks an active call with the same emphasis a toggled feature gets', async () => {
    render(<VoiceControls {...props()} />)
    await startCall()

    expect(screen.getByRole('button', { name: 'Voice' }).className).toBe(
      toolbarControlClasses('active', TOOLBAR_BUTTON_LAYOUT),
    )
  })
})

describe('Phone-style Voice UX', () => {
  it('never starts a hot microphone from persisted enable_talk; one click starts a call', async () => {
    const options = props()
    render(<VoiceControls {...options} />)
    expect(media.getUserMedia).not.toHaveBeenCalled()
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    await startCall()
    expect(media.getUserMedia).toHaveBeenCalledTimes(1)
    expect(
      screen.getByRole('dialog', { name: 'Voice call' }),
    ).toBeInTheDocument()
    expect(screen.getByRole('status')).toHaveTextContent('Listening')
    // is_free/byok flags do not override a gateway that accepts this user's token.
    expect(options.onError).not.toHaveBeenCalledWith(expect.any(String))
  })

  it('keeps the call connected while muted or minimized, then hangs up explicitly', async () => {
    render(<VoiceControls {...props()} />)
    const socket = await startCall()
    fireEvent.click(screen.getByRole('button', { name: 'Mute microphone' }))
    expect(media.track.enabled).toBe(false)
    fireEvent.click(screen.getByRole('button', { name: 'Minimize call' }))
    expect(socket.close).not.toHaveBeenCalled()
    expect(screen.getByRole('button', { name: 'Hang up' })).toBeEnabled()
    fireEvent.click(screen.getByRole('button', { name: 'Restore call' }))
    fireEvent.click(screen.getByRole('button', { name: 'Unmute microphone' }))
    expect(media.track.enabled).toBe(true)
    fireEvent.click(screen.getByRole('button', { name: 'Hang up' }))
    await act(flush)
    expect(socket.close).toHaveBeenCalledTimes(1)
    expect(media.track.stop).toHaveBeenCalledTimes(1)
    expect(screen.getByRole('status')).toHaveTextContent('Call ended.')
  })

  it('reports the session a call records into, and keeps it across a switch', async () => {
    const options = props()
    const view = render(<VoiceControls {...options} />)
    await startCall()

    // The call pins the session it began in.
    expect(options.onCallSessionChange).toHaveBeenCalledWith(1)

    options.onCallSessionChange.mockClear()
    view.rerender(<VoiceControls {...options} sessionId={2} />)
    await act(flush)

    // Viewing another session must not move where the call records.
    expect(options.onCallSessionChange).not.toHaveBeenCalledWith(2)
  })

  it('clears the recording session when the call ends', async () => {
    const options = props()
    render(<VoiceControls {...options} />)
    const socket = await startCall()
    options.onCallSessionChange.mockClear()

    await act(async () => {
      socket.remoteClose(1000)
      await flush()
    })

    expect(options.onCallSessionChange).toHaveBeenCalledWith(null)
  })

  it('pins the original account, prompt, and call label across text-session switches without redialing', async () => {
    const options = props()
    const view = render(<VoiceControls {...options} />)
    const socket = await startCall()
    view.rerender(
      <VoiceControls
        {...options}
        sessionId={2}
        user={{ ...account, api_base: 'https://other.example.com' }}
        config={{
          ...options.config,
          api_token: 'account-b-token',
          session_name: 'Session B',
          system_prompt: 'A different prompt',
          chat_switch: {
            ...options.config.chat_switch,
            enable_talk: false,
          },
        }}
      />,
    )
    await act(flush)
    expect(socket.close).not.toHaveBeenCalled()
    expect(FakeSocket.instances).toHaveLength(1)
    expect(socket.url).toContain('gateway.example.com')
    expect(socket.protocols).toContain(
      'openai-insecure-api-key.account-a-token',
    )
    expect(screen.getByRole('heading')).toHaveTextContent('Session A (1)')
    expect(media.track.stop).not.toHaveBeenCalled()
  })

  it('allows hang-up during microphone permission and releases a late grant', async () => {
    const permission = deferred<MediaStream>()
    media.getUserMedia.mockReturnValue(permission.promise)
    render(<VoiceControls {...props()} />)
    fireEvent.click(screen.getByRole('button', { name: 'Voice' }))
    fireEvent.click(screen.getByRole('button', { name: 'Hang up' }))
    await act(flush)
    await act(async () => {
      permission.resolve(media.stream)
      await flush()
    })
    expect(media.track.stop).toHaveBeenCalledTimes(1)
    expect(FakeSocket.instances).toHaveLength(0)
  })

  it('does not automatically restart after hang-up or later preference hydration', async () => {
    const options = props()
    const view = render(<VoiceControls {...options} />)
    await startCall()
    fireEvent.click(screen.getByRole('button', { name: 'Hang up' }))
    await act(flush)
    view.rerender(
      <VoiceControls
        {...options}
        config={{ ...options.config, updated_at: Date.now() }}
      />,
    )
    await act(flush)
    expect(media.getUserMedia).toHaveBeenCalledTimes(1)
  })

  it('uses an explicit direct API base without requiring account lookup', async () => {
    const options = props()
    render(
      <VoiceControls
        {...options}
        user={undefined}
        config={{
          ...options.config,
          token_type: 'direct',
          api_base: 'https://direct.example.com',
        }}
      />,
    )
    const socket = await startCall()
    expect(socket.url).toContain('direct.example.com')
  })

  it('blocks accidental page departure while active and removes the guard after hang-up', async () => {
    render(<VoiceControls {...props()} />)
    await startCall()
    const activeEvent = new Event('beforeunload', { cancelable: true })
    window.dispatchEvent(activeEvent)
    expect(activeEvent.defaultPrevented).toBe(true)
    fireEvent.click(screen.getByRole('button', { name: 'Hang up' }))
    await act(flush)
    const endedEvent = new Event('beforeunload', { cancelable: true })
    window.dispatchEvent(endedEvent)
    expect(endedEvent.defaultPrevented).toBe(false)
  })
})
