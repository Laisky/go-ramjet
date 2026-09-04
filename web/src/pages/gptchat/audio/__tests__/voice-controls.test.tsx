import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { DefaultSessionConfig, type UserConfig } from '../../types'
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
    sessionId: 'A',
    user: account,
    disabled: false,
    onDraftText: vi.fn(),
    onError: vi.fn(),
    onBusyChange: vi.fn(),
    onActivityChange: vi.fn(),
    onStatusChange: vi.fn(),
    onConfigChange: vi.fn(),
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

  it('pins the original account, prompt, and call label across text-session switches without redialing', async () => {
    const options = props()
    const view = render(<VoiceControls {...options} />)
    const socket = await startCall()
    view.rerender(
      <VoiceControls
        {...options}
        sessionId="B"
        user={{ ...account, api_base: 'https://other.example.com' }}
        config={{
          ...options.config,
          api_token: 'account-b-token',
          session_name: 'Session B',
          system_prompt: 'A different prompt',
          chat_switch: {
            ...options.config.chat_switch,
            enable_talk: false,
            audio_plugin: 'whisper',
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
    expect(screen.getByRole('heading')).toHaveTextContent('Session A (A)')
    expect(
      screen.getByRole('combobox', { name: 'Audio plugin' }),
    ).toBeDisabled()
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
