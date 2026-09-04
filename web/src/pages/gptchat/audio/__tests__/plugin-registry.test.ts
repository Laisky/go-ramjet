import { describe, expect, it } from 'vitest'

import { resolveAudioPlugin } from '../plugin-registry'
import {
  formatRealtimeStatus,
  resolveRealtimeAPIBase,
} from '../realtime-plugin-utils'
import { DefaultSessionConfig, type UserConfig } from '../../types'

describe('resolveAudioPlugin', () => {
  it('keeps Whisper as the legacy-safe default', () => {
    expect(resolveAudioPlugin().id).toBe('whisper')
    expect(resolveAudioPlugin('unknown').id).toBe('whisper')
  })

  it('selects the Realtime implementation explicitly', () => {
    expect(resolveAudioPlugin('realtime').id).toBe('realtime')
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
