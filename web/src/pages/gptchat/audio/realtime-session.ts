// The realtime model is a server setting delivered with the user config. This is the
// fallback used when the server did not send one.
export const DEFAULT_REALTIME_AUDIO_MODEL = 'gpt-realtime-2.1-mini'
export const REALTIME_AUDIO_VOICE = 'marin'
// Input transcription is a separate ASR job, billed on its own and run out of band
// from the spoken reply, so it adds no latency to the voice path. Without it the
// API never reports what the user said and the call cannot be written to the chat.
export const REALTIME_INPUT_TRANSCRIPTION_MODEL = 'gpt-4o-mini-transcribe'

/**
 * buildRealtimeWebSocketURL resolves a configured API base to the GA Realtime route.
 *
 * The model belongs only in this query parameter. Repeating it inside session.update
 * makes gateways reject the call as a mid-session model switch.
 */
export function buildRealtimeWebSocketURL(
  apiBase: string,
  model: string = DEFAULT_REALTIME_AUDIO_MODEL,
): string {
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
  url.searchParams.set('model', model.trim() || DEFAULT_REALTIME_AUDIO_MODEL)
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
          // No `language` here: the caller may switch languages mid-call, and
          // pinning one forces the transcriber to mishear the others.
          transcription: { model: REALTIME_INPUT_TRANSCRIPTION_MODEL },
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
