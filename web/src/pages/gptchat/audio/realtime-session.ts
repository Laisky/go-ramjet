export const REALTIME_AUDIO_MODEL = 'gpt-realtime-2.1'
export const REALTIME_AUDIO_VOICE = 'marin'

/** buildRealtimeWebSocketURL resolves a configured API base to the GA Realtime route. */
export function buildRealtimeWebSocketURL(apiBase: string): string {
  const url = new URL(apiBase)
  if (url.protocol !== 'https:' && url.protocol !== 'http:') {
    throw new Error('Realtime API base must use HTTP or HTTPS')
  }
  if (url.username || url.password)
    throw new Error('Realtime API base must not contain credentials')
  if (url.hash)
    throw new Error('Realtime API base must not contain a URL fragment')
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:'
  let path = url.pathname.replace(/\/+$/, '')
  if (!path) path = '/v1/realtime'
  else if (path.endsWith('/v1')) path += '/realtime'
  else if (!path.endsWith('/v1/realtime')) path += '/v1/realtime'
  url.pathname = path
  url.searchParams.set('model', REALTIME_AUDIO_MODEL)
  return url.toString()
}

/** createRealtimeSessionUpdate configures continuous native audio and an explicit hang-up tool. */
export function createRealtimeSessionUpdate(instructions: string) {
  return {
    type: 'session.update',
    session: {
      type: 'realtime',
      output_modalities: ['audio'],
      instructions: `${instructions}\n\nYou are in a live voice call. Speak naturally and concisely, without markdown. Listen between turns. Never end the call merely because an answer is complete, the user is silent, or the microphone is muted. When the user explicitly asks to end the call or says goodbye, or you need to end a conversation you cannot continue, say a brief goodbye and call end_call. Only end_call ends the call; saying goodbye alone does not.`,
      audio: {
        input: {
          format: { type: 'audio/pcm', rate: 24_000 },
          turn_detection: {
            type: 'semantic_vad',
            create_response: true,
            interrupt_response: true,
          },
        },
        output: {
          format: { type: 'audio/pcm', rate: 24_000 },
          voice: REALTIME_AUDIO_VOICE,
        },
      },
      tools: [
        {
          type: 'function',
          name: 'end_call',
          description:
            'End this voice call after saying goodbye. Use only for an explicit goodbye, a request to hang up, or when the conversation cannot continue. Never use for silence or a completed answer.',
          parameters: {
            type: 'object',
            properties: {
              reason: {
                type: 'string',
                description: 'Brief reason shown to the user.',
              },
            },
            required: ['reason'],
            additionalProperties: false,
          },
        },
      ],
      tool_choice: 'auto',
    },
  }
}
