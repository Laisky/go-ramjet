import { describe, expect, it } from 'vitest'

import {
  buildRealtimeWebSocketURL,
  createRealtimeSessionUpdate,
  REALTIME_AUDIO_MODEL,
  REALTIME_AUDIO_VOICE,
} from '../realtime-client'

describe('buildRealtimeWebSocketURL', () => {
  it('builds the canonical route from an origin', () => {
    const url = new URL(buildRealtimeWebSocketURL('https://proxy.example.com'))

    expect(url.protocol).toBe('wss:')
    expect(url.pathname).toBe('/v1/realtime')
    expect(url.searchParams.get('model')).toBe(REALTIME_AUDIO_MODEL)
  })

  it('preserves a versioned prefix and existing query parameters', () => {
    const url = new URL(
      buildRealtimeWebSocketURL(
        'https://proxy.example.com/custom/v1?tenant=example',
      ),
    )

    expect(url.pathname).toBe('/custom/v1/realtime')
    expect(url.searchParams.get('tenant')).toBe('example')
    expect(url.searchParams.get('model')).toBe(REALTIME_AUDIO_MODEL)
  })

  it('does not duplicate an existing realtime route', () => {
    const url = new URL(
      buildRealtimeWebSocketURL(
        'http://localhost:3000/v1/realtime?model=old-model',
      ),
    )

    expect(url.protocol).toBe('ws:')
    expect(url.pathname).toBe('/v1/realtime')
    expect(url.searchParams.get('model')).toBe(REALTIME_AUDIO_MODEL)
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
})

describe('createRealtimeSessionUpdate', () => {
  it('uses native audio with the latest configured model and semantic VAD', () => {
    const event = createRealtimeSessionUpdate('Be concise.') as {
      type: string
      session: {
        type: string
        model: string
        output_modalities: string[]
        instructions: string
        audio: {
          input: {
            format: { type: string; rate: number }
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
    expect(event.session.model).toBe(REALTIME_AUDIO_MODEL)
    expect(event.session.output_modalities).toEqual(['audio'])
    expect(event.session.instructions).toBe('Be concise.')
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
    expect(JSON.stringify(event)).not.toContain('transcription')
  })
})
