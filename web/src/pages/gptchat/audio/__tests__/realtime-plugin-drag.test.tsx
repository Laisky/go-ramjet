import { act, fireEvent, render, screen } from '@testing-library/react'
import { createRef } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { DefaultSessionConfig } from '../../types'
import type { AudioPluginHandle } from '../plugin-types'
import { RealtimeAudioPlugin } from '../realtime-plugin'
import { VIEWPORT_MARGIN } from '../realtime-plugin-utils'
import { FakeSocket, flush, installMedia } from './media-fixtures'

let media: ReturnType<typeof installMedia>

/**
 * TestPointerEvent supplies the PointerEvent jsdom does not implement.
 *
 * Without it every synthesized pointer event arrives as a bare Event with no
 * button or coordinates, so a drag would look like a press of an unknown mouse
 * button and the handler would correctly ignore it.
 */
class TestPointerEvent extends MouseEvent {
  readonly pointerId: number
  readonly pointerType: string
  constructor(type: string, init: PointerEventInit = {}) {
    super(type, init)
    this.pointerId = init.pointerId ?? 0
    this.pointerType = init.pointerType ?? 'mouse'
  }
}

/**
 * startCall renders the plugin and brings a call up to the listening state.
 *
 * The window only exists during a call, so there is nothing to drag until the
 * server has acknowledged the session.
 */
async function startCall() {
  const controlRef = createRef<AudioPluginHandle>()
  render(
    <RealtimeAudioPlugin
      config={{
        ...DefaultSessionConfig,
        api_token: 'test-token',
        // Direct routing needs no account lookup, so the call starts without a
        // loaded user and the window renders.
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
  return {
    panel: screen.getByRole('dialog', { name: 'Voice call' }),
    handle: screen.getByRole('group', { name: 'Move the call window' }),
  }
}

/**
 * drag moves one pointer from one viewport point to another across the handle.
 *
 * pointerType is the only difference between a mouse and a finger here, which is
 * the point: both input methods travel the same code path.
 */
function drag(
  handle: HTMLElement,
  from: { x: number; y: number },
  to: { x: number; y: number },
  pointerType = 'mouse',
  pointerId = 1,
) {
  fireEvent.pointerDown(handle, {
    pointerId,
    pointerType,
    button: 0,
    clientX: from.x,
    clientY: from.y,
  })
  fireEvent.pointerMove(handle, {
    pointerId,
    pointerType,
    clientX: to.x,
    clientY: to.y,
  })
  fireEvent.pointerUp(handle, {
    pointerId,
    pointerType,
    clientX: to.x,
    clientY: to.y,
  })
}

beforeEach(() => {
  media = installMedia()
  vi.stubGlobal('PointerEvent', TestPointerEvent)
})
afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

describe('Voice call window placement', () => {
  it('sits in the corner until it is moved', async () => {
    const { panel } = await startCall()
    expect(panel.className).toContain('bottom-4')
    expect(panel.className).toContain('right-4')
    expect(panel.style.left).toBe('')
    expect(media.getUserMedia).toHaveBeenCalled()
  })

  it('follows the pointer and drops the corner anchor', async () => {
    const { panel, handle } = await startCall()
    drag(handle, { x: 100, y: 100 }, { x: 340, y: 260 })

    // The grab point is preserved, so the window moves by the pointer's delta
    // rather than jumping its corner under the cursor.
    expect(panel.style.left).toBe('240px')
    expect(panel.style.top).toBe('160px')
    // Corner classes have to go, or they would fight the explicit offset.
    expect(panel.className).not.toContain('bottom-4')
    expect(panel.className).not.toContain('right-4')
  })

  it('follows a finger the same way it follows a mouse', async () => {
    const { panel, handle } = await startCall()
    drag(handle, { x: 80, y: 500 }, { x: 200, y: 300 }, 'touch')
    expect(panel.style.left).toBe('120px')
    expect(panel.style.top).toBe('8px')
  })

  it('keeps touch scrolling off the drag handle', async () => {
    const { handle } = await startCall()
    // Without touch-action: none the browser claims the gesture and scrolls the
    // page, so a finger can never move the window on a phone.
    expect(handle.className).toContain('touch-none')
  })

  it('leaves the window with the finger that grabbed it', async () => {
    const { panel, handle } = await startCall()
    fireEvent.pointerDown(handle, {
      pointerId: 1,
      pointerType: 'touch',
      button: 0,
      clientX: 100,
      clientY: 100,
    })
    // A second finger lands somewhere else; the window must not jump to it.
    fireEvent.pointerDown(handle, {
      pointerId: 2,
      pointerType: 'touch',
      button: 0,
      clientX: 300,
      clientY: 400,
    })
    fireEvent.pointerMove(handle, {
      pointerId: 1,
      pointerType: 'touch',
      clientX: 260,
      clientY: 220,
    })
    expect(panel.style.left).toBe('160px')
    expect(panel.style.top).toBe('120px')
  })

  it('drops the drag when the browser cancels the gesture', async () => {
    const { panel, handle } = await startCall()
    fireEvent.pointerDown(handle, {
      pointerId: 1,
      pointerType: 'touch',
      button: 0,
      clientX: 100,
      clientY: 100,
    })
    fireEvent.pointerCancel(handle, { pointerId: 1, pointerType: 'touch' })
    // Moves after a cancelled gesture belong to the page, not the window.
    fireEvent.pointerMove(handle, {
      pointerId: 1,
      pointerType: 'touch',
      clientX: 500,
      clientY: 500,
    })
    expect(panel.style.left).toBe(`${VIEWPORT_MARGIN}px`)
  })

  it('will not let the window be dropped out of reach', async () => {
    const { panel, handle } = await startCall()
    drag(handle, { x: 100, y: 100 }, { x: -900, y: -900 })
    expect(panel.style.left).toBe(`${VIEWPORT_MARGIN}px`)
    expect(panel.style.top).toBe(`${VIEWPORT_MARGIN}px`)
  })

  it('ignores a press that lands on a control inside the handle', async () => {
    const { panel, handle } = await startCall()
    const minimize = screen.getByRole('button', { name: 'Minimize call' })
    fireEvent.pointerDown(minimize, {
      pointerId: 1,
      button: 0,
      clientX: 100,
      clientY: 100,
    })
    fireEvent.pointerMove(handle, { pointerId: 1, clientX: 400, clientY: 400 })
    // Minimizing must stay a click; the window must not slide away under it.
    expect(panel.style.left).toBe('')
  })

  it('moves from the keyboard for callers without a pointer', async () => {
    const { panel, handle } = await startCall()
    drag(handle, { x: 100, y: 100 }, { x: 340, y: 260 })

    // Nudging continues from where the pointer left the window rather than
    // restarting from its corner.
    fireEvent.keyDown(handle, { key: 'ArrowRight' })
    fireEvent.keyDown(handle, { key: 'ArrowUp' })
    expect(panel.style.left).toBe('256px')
    expect(panel.style.top).toBe('144px')
  })
})
