import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { DefaultSessionConfig } from '../../types'
import {
  TOOLBAR_BUTTON_LAYOUT,
  toolbarControlClasses,
} from '../../components/toolbar-control'
import { WhisperAudioPlugin } from '../whisper-plugin'

interface Deferred<T> {
  promise: Promise<T>
  resolve: (value: T) => void
}

/** createDeferred creates a manually resolved Promise for permission races. */
function createDeferred<T>(): Deferred<T> {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise
  })
  return { promise, resolve }
}

/** flushMicrotasks lets permission continuations finish after unmount. */
async function flushMicrotasks(): Promise<void> {
  await Promise.resolve()
  await Promise.resolve()
}

describe('WhisperAudioPlugin lifecycle regressions', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('wears the composer toolbar style, not the generic button size', () => {
    render(
      <WhisperAudioPlugin
        config={{ ...DefaultSessionConfig, api_token: 'test-token' }}
        disabled={false}
        onDraftText={vi.fn()}
        onError={vi.fn()}
        onBusyChange={vi.fn()}
        onActivityChange={vi.fn()}
        onStatusChange={vi.fn()}
      />,
    )

    // It sits in the same row as the feature toggles, so it must share their scale.
    expect(
      screen.getByRole('button', { name: 'Start Whisper recording' }).className,
    ).toBe(toolbarControlClasses('idle', TOOLBAR_BUTTON_LAYOUT))
  })

  it('stops a late microphone grant without constructing MediaRecorder after unmount', async () => {
    const permission = createDeferred<MediaStream>()
    const track = { stop: vi.fn() }
    const stream = {
      getTracks: vi.fn(() => [track]),
    } as unknown as MediaStream
    Object.defineProperty(navigator, 'mediaDevices', {
      configurable: true,
      value: {
        getUserMedia: vi.fn(() => permission.promise),
      },
    })
    const mediaRecorderConstructor = vi.fn(function (
      this: Record<string, unknown>,
      input: MediaStream,
    ) {
      this.stream = input
      this.state = 'inactive'
      this.start = vi.fn()
      this.stop = vi.fn()
    })
    vi.stubGlobal('MediaRecorder', mediaRecorderConstructor)

    const rendered = render(
      <WhisperAudioPlugin
        config={{ ...DefaultSessionConfig, api_token: 'test-token' }}
        disabled={false}
        onDraftText={vi.fn()}
        onError={vi.fn()}
        onBusyChange={vi.fn()}
        onActivityChange={vi.fn()}
        onStatusChange={vi.fn()}
      />,
    )

    await userEvent.click(
      screen.getByRole('button', { name: 'Start Whisper recording' }),
    )
    rendered.unmount()
    permission.resolve(stream)
    await flushMicrotasks()

    expect(track.stop).toHaveBeenCalledTimes(1)
    expect(mediaRecorderConstructor).not.toHaveBeenCalled()
  })
})
