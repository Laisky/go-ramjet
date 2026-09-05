import { describe, expect, it } from 'vitest'

import {
  buildRealtimeWebSocketURL,
  createRealtimeSessionUpdate,
  DEFAULT_REALTIME_AUDIO_MODEL,
  REALTIME_AUDIO_VOICE,
  REALTIME_INPUT_TRANSCRIPTION_MODEL,
} from '../realtime-client'

describe('buildRealtimeWebSocketURL', () => {
  it('builds the canonical route from an origin', () => {
    const url = new URL(buildRealtimeWebSocketURL('https://proxy.example.com'))

    expect(url.protocol).toBe('wss:')
    expect(url.pathname).toBe('/v1/realtime')
    expect(url.searchParams.get('model')).toBe(DEFAULT_REALTIME_AUDIO_MODEL)
  })

  it('preserves a versioned prefix and existing query parameters', () => {
    const url = new URL(
      buildRealtimeWebSocketURL(
        'https://proxy.example.com/custom/v1?tenant=example',
      ),
    )

    expect(url.pathname).toBe('/custom/v1/realtime')
    expect(url.searchParams.get('tenant')).toBe('example')
    expect(url.searchParams.get('model')).toBe(DEFAULT_REALTIME_AUDIO_MODEL)
  })

  it('does not duplicate an existing realtime route', () => {
    const url = new URL(
      buildRealtimeWebSocketURL(
        'http://localhost:3000/v1/realtime?model=old-model',
      ),
    )

    expect(url.protocol).toBe('ws:')
    expect(url.pathname).toBe('/v1/realtime')
    expect(url.searchParams.get('model')).toBe(DEFAULT_REALTIME_AUDIO_MODEL)
  })

  it('rejects unsafe or unsupported API bases', () => {
    expect(() => buildRealtimeWebSocketURL('ftp://example.com')).toThrow(
      'Realtime API base must use HTTP or HTTPS',
    )
    expect(() =>
      buildRealtimeWebSocketURL('https://user:secret@example.com'),
    ).toThrow('Realtime API base must not contain credentials')
    expect(() =>
      buildRealtimeWebSocketURL('https://example.com/#fragment'),
    ).toThrow('Realtime API base must not contain a URL fragment')
  })

  it('uses the server-configured model instead of the built-in default', () => {
    // The realtime model is deployment configuration, so it must reach the only
    // place the API accepts it: the connection query parameter.
    const url = new URL(
      buildRealtimeWebSocketURL(
        'https://gateway.example.com',
        'gpt-realtime-2.1',
      ),
    )
    expect(url.searchParams.get('model')).toBe('gpt-realtime-2.1')
  })

  it('falls back to the default when the server sent a blank model', () => {
    const url = new URL(
      buildRealtimeWebSocketURL('https://gateway.example.com', '  '),
    )
    expect(url.searchParams.get('model')).toBe(DEFAULT_REALTIME_AUDIO_MODEL)
  })
})

describe('createRealtimeSessionUpdate', () => {
  it('uses native audio with the latest configured model and semantic VAD', () => {
    const event = createRealtimeSessionUpdate('Be concise.') as {
      type: string
      session: {
        type: string
        model?: string
        output_modalities: string[]
        instructions: string
        audio: {
          input: {
            format: { type: string; rate: number }
            transcription?: { model: string }
            turn_detection: {
              type: string
              create_response: boolean
              interrupt_response: boolean
            }
          }
          output: {
            format: { type: string; rate: number }
            voice: string
          }
        }
      }
    }

    expect(event.type).toBe('session.update')
    expect(event.session.type).toBe('realtime')
    // The model is fixed by the connection query parameter. Sending it again in
    // session.update makes gateways reject the call as a mid-session model switch.
    expect(event.session.model).toBeUndefined()
    expect(event.session.output_modalities).toEqual(['audio'])
    expect(event.session.instructions).toContain('Be concise.')
    expect(event.session.audio.input.format).toEqual({
      type: 'audio/pcm',
      rate: 24_000,
    })
    expect(event.session.audio.input.turn_detection).toEqual({
      type: 'semantic_vad',
      create_response: true,
      interrupt_response: true,
    })
    expect(event.session.audio.output).toEqual({
      format: { type: 'audio/pcm', rate: 24_000 },
      voice: REALTIME_AUDIO_VOICE,
    })
    // Input transcription is what lets the call be written into the text chat.
    expect(event.session.audio.input.transcription).toEqual({
      model: REALTIME_INPUT_TRANSCRIPTION_MODEL,
    })
  })
})
