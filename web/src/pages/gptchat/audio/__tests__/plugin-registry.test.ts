import { describe, expect, it } from 'vitest'

import { resolveAudioPlugin } from '../plugin-registry'
import {
  clampFloatingPosition,
  formatRealtimeStatus,
  resolveRealtimeAPIBase,
  VIEWPORT_MARGIN,
} from '../realtime-plugin-utils'
import { DefaultSessionConfig, type UserConfig } from '../../types'

describe('resolveAudioPlugin', () => {
  it('defaults to Realtime when the server sent no choice', () => {
    // A deployment that never configured voice must still get live calls, and an
    // unrecognized value must not silently downgrade it to dictation.
    expect(resolveAudioPlugin().id).toBe('realtime')
    expect(resolveAudioPlugin('unknown').id).toBe('realtime')
    expect(resolveAudioPlugin('realtime').id).toBe('realtime')
  })

  it('selects Whisper only when the server asks for it', () => {
    expect(resolveAudioPlugin('whisper').id).toBe('whisper')
  })
})

describe('resolveRealtimeAPIBase', () => {
  const user: UserConfig = {
    user_name: 'example',
    token: '',
    openai_token: '',
    image_token: '',
    is_free: false,
    byok: true,
    is_admin: false,
    allowed_models: ['*'],
    no_limit_expensive_models: true,
    api_base: 'https://account.example.com',
    image_url: '',
  }

  it('uses the account base when the session still has the default origin', () => {
    expect(resolveRealtimeAPIBase(DefaultSessionConfig, user)).toBe(
      'https://account.example.com',
    )
  })

  it('prefers an explicit session API base override', () => {
    expect(
      resolveRealtimeAPIBase(
        {
          ...DefaultSessionConfig,
          api_base: 'https://session.example.com',
        },
        user,
      ),
    ).toBe('https://session.example.com')
  })
})

describe('formatRealtimeStatus', () => {
  it('reports lifecycle states without a transcript', () => {
    expect(formatRealtimeStatus('connecting', '')).toBe('Connecting…')
    expect(formatRealtimeStatus('thinking', '')).toBe('Thinking…')
    expect(formatRealtimeStatus('idle', '')).toBeNull()
  })

  it('adds a compact transcript preview while speaking', () => {
    expect(formatRealtimeStatus('speaking', 'Hello   from GPT.')).toBe(
      'Speaking… Hello from GPT.',
    )
  })
})

describe('clampFloatingPosition', () => {
  const size = { width: 384, height: 240 }
  const viewport = { width: 1280, height: 800 }

  it('leaves a window that already sits inside the viewport alone', () => {
    expect(clampFloatingPosition({ x: 400, y: 300 }, size, viewport)).toEqual({
      x: 400,
      y: 300,
    })
  })

  it('pulls a window dropped past an edge back into reach', () => {
    // Dragged off the bottom-right, the hang-up button would be unclickable.
    expect(clampFloatingPosition({ x: 5000, y: 5000 }, size, viewport)).toEqual(
      {
        x: viewport.width - size.width - VIEWPORT_MARGIN,
        y: viewport.height - size.height - VIEWPORT_MARGIN,
      },
    )
    expect(clampFloatingPosition({ x: -300, y: -300 }, size, viewport)).toEqual(
      { x: VIEWPORT_MARGIN, y: VIEWPORT_MARGIN },
    )
  })

  it('keeps the header on screen when the window is taller than the viewport', () => {
    // A short viewport makes the upper bound smaller than the lower one; the
    // top edge has to win, or the window hides its own controls off the top.
    expect(
      clampFloatingPosition({ x: 40, y: 40 }, size, {
        width: 320,
        height: 160,
      }),
    ).toEqual({ x: VIEWPORT_MARGIN, y: VIEWPORT_MARGIN })
  })
})
