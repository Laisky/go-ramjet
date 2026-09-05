import { act, render, screen } from '@testing-library/react'
import { createRef } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { DefaultSessionConfig } from '../../types'
import { pcm16ToBase64 } from '../pcm-audio'
import type { AudioPluginHandle } from '../plugin-types'
import { RealtimeAudioPlugin } from '../realtime-plugin'
import { FakeContext, FakeSocket, flush, installMedia } from './media-fixtures'

/** startCall renders the plugin and brings a call up to the listening state. */
async function startCall() {
  const controlRef = createRef<AudioPluginHandle>()
  render(
    <RealtimeAudioPlugin
      config={{
        ...DefaultSessionConfig,
        api_token: 'test-token',
        token_type: 'direct',
      }}
      disabled={false}
      controlRef={controlRef}
      onDraftText={vi.fn()}
      onError={vi.fn()}
      onBusyChange={vi.fn()}
      onActivityChange={vi.fn()}
      onStatusChange={vi.fn()}
    />,
  )
  await act(async () => {
    controlRef.current?.start()
    await flush()
  })
  const socket = FakeSocket.instances.at(-1)!
  await act(async () => {
    socket.open()
    socket.event({ type: 'session.updated' })
    await flush()
  })
  const panel = screen.getByRole('dialog', { name: 'Voice call' })
  return { socket, orb: panel.querySelector('.voice-orb') as HTMLElement }
}

/** speak delivers one reply's audio, which puts the call into the speaking state. */
async function speak(socket: FakeSocket) {
  await act(async () => {
    socket.event({ type: 'response.created', response: { id: 'r1' } })
    socket.event({
      type: 'response.output_audio.delta',
      response_id: 'r1',
      item_id: 'item-r1',
      content_index: 0,
      delta: pcm16ToBase64(new Int16Array(2400)),
    })
    await flush()
  })
}

/** nextFrame runs the animation frame the orb's follower is waiting on. */
function nextFrame() {
  act(() => {
    vi.advanceTimersByTime(20)
  })
}

beforeEach(() => {
  installMedia()
})
afterEach(() => {
  vi.useRealTimers()
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

describe('Voice call orb', () => {
  it('breathes while the caller has the floor', async () => {
    const { orb } = await startCall()
    expect(orb.className).toContain('voice-orb-listening')
    expect(orb.style.getPropertyValue('--voice-level')).toBe('0')
  })

  it('stops breathing once the assistant takes over', async () => {
    const { socket, orb } = await startCall()
    await speak(socket)
    // The breath means "listening"; leaving it running under the reply would
    // say the caller still has the floor.
    expect(orb.className).not.toContain('voice-orb-listening')
  })

  it('tracks the loudness of the reply while it plays', async () => {
    vi.useFakeTimers()
    const { socket, orb } = await startCall()
    FakeContext.outputAmplitude = 0.2
    await speak(socket)
    nextFrame()
    expect(orb.style.getPropertyValue('--voice-level')).toBe('0.600')

    // A quieter passage has to bring the orb back down within a frame or two,
    // otherwise it reads as a fixed animation rather than the voice.
    FakeContext.outputAmplitude = 0.05
    nextFrame()
    expect(orb.style.getPropertyValue('--voice-level')).toBe('0.150')
  })

  it('settles the orb when the caller regains the floor', async () => {
    vi.useFakeTimers()
    const { socket, orb } = await startCall()
    FakeContext.outputAmplitude = 0.2
    await speak(socket)
    nextFrame()
    expect(orb.style.getPropertyValue('--voice-level')).not.toBe('0')

    await act(async () => {
      socket.event({
        type: 'response.done',
        response: { id: 'r1', status: 'completed' },
      })
      // Generation finishing is not the end of the reply; the call returns to
      // listening only once the scheduled audio has actually been heard.
      FakeContext.instances.at(-1)!.sources.forEach((source) => source.finish())
      await flush()
    })
    // Playback has drained, so the orb must come to rest rather than hold the
    // last syllable's size, and take the listening breath back up.
    expect(orb.style.getPropertyValue('--voice-level')).toBe('0')
    expect(orb.className).toContain('voice-orb-listening')
  })
})
