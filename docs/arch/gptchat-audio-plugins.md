# GPTChat audio plugins

## Purpose

GPTChat audio is implemented behind a small frontend plugin boundary. Users can keep the existing record-and-transcribe behavior or opt into a continuous native-audio conversation with OpenAI Realtime.

The default remains `whisper`, so existing sessions and saved configurations keep their current behavior.

## Implementations

| Plugin | Input path | Model response path | Intended use |
|---|---|---|---|
| `whisper` | `MediaRecorder` → `/v1/audio/transcriptions` → editable draft | Existing GPTChat completion flow; existing per-message speech controls remain available | Backward-compatible, review text before sending |
| `realtime` | Microphone PCM16 → Realtime WebSocket | Native model PCM16 → browser playback | Low-latency, interruptible speech-to-speech conversation |

`ChatInput` depends only on `AudioPluginProps`. A future backend can be registered without adding recording or transport logic back to `ChatInput`.

## Realtime design

The Realtime plugin uses the latest general Realtime model selected for this implementation:

- model: `gpt-realtime-2.1`
- voice: `marin`
- input and output: signed little-endian PCM16 at 24 kHz
- turn detection: semantic VAD
- automatic response creation: enabled
- response interruption: enabled

```text
microphone
   │ browser Web Audio capture
   ▼
48 kHz Float32 (normally)
   │ resample + PCM16 encode
   ▼
OpenAI-compatible /v1/realtime WebSocket
   │ native model audio deltas
   ▼
PCM16 playback queue
   │
   ▼
speakers
```

The plugin does not call GPTChat's transcription endpoint, completion endpoint, or TTS endpoint. The assistant audio transcript emitted by Realtime is used only as compact visual feedback; it is not a separate STT request.

## Why WebSocket

OpenAI recommends WebRTC for browser media when the application talks directly to OpenAI. This deployment already routes and meters Realtime WebSocket sessions through OneAPI, including browser authentication through `Sec-WebSocket-Protocol`. The gateway does not currently expose the newer unified `/v1/realtime/calls` WebRTC bootstrap route.

Using the supported WebSocket route therefore preserves the existing API base, token authorization, model routing, and usage accounting without exposing a server-owned API key or introducing another media proxy.

A future gateway implementation of `/v1/realtime/calls` can be added as another plugin without changing the Whisper plugin or `ChatInput`.

## Authentication and eligibility

- The browser uses the API token already configured by the user for GPTChat.
- The token is sent as the Realtime WebSocket authentication subprotocol supported by OneAPI.
- Server-owned free-tier credentials are never returned to the browser.
- Realtime is rejected for free-tier accounts; those accounts continue to use Whisper.
- Disabling Voice, changing plugin, changing session, or unmounting the input closes the socket, microphone tracks, capture graph, and playback context.

## Interruption behavior

Semantic VAD detects when the user starts speaking while the assistant is playing audio. The client then:

1. immediately stops queued local playback;
2. calculates how much assistant audio was actually heard;
3. sends `conversation.item.truncate` with that playback position.

This keeps the model's conversation state aligned with what the user heard instead of leaving unheard assistant audio in context.

## Conversation scope

A Realtime conversation is owned by the active audio plugin instance. It receives the session system prompt but does not reuse or mutate GPTChat's text-completion history. User speech is not transcribed or persisted by the application. This is intentional: enabling a separate input transcription model would recreate an STT dependency.

## Failure behavior

- Microphone permission, invalid API bases, connection failures, model errors, and playback errors are shown in the existing input error surface.
- Whisper remains available as an immediate fallback.
- The plugin selector is locked while an audio session is active.
- Abnormal socket closure releases local media resources and returns the control to idle.

## Validation

Automated coverage verifies:

- legacy-safe plugin fallback to Whisper;
- Realtime plugin selection;
- API base to WebSocket URL conversion;
- exact `gpt-realtime-2.1` audio-only session configuration;
- semantic VAD and interruption settings;
- 48 kHz to 24 kHz PCM conversion and clipping;
- little-endian PCM16 base64 round-trip;
- API-base precedence and status formatting.

Manual acceptance should verify:

1. Whisper recording still writes editable text into the draft.
2. Realtime starts only after a user gesture and microphone approval.
3. A spoken turn produces streamed audio without GPTChat STT, completion, or TTS requests.
4. Speaking over the assistant stops playback quickly and the following response remains coherent.
5. Turning Voice off releases the microphone indicator.
6. A free-tier session receives a clear fallback message.

## OpenAI references

- [Realtime API guide](https://developers.openai.com/api/docs/guides/realtime)
- [GPT-Realtime-2.1 model](https://developers.openai.com/api/docs/models/gpt-realtime-2.1)
- [Realtime conversations and events](https://developers.openai.com/api/docs/guides/realtime-conversations)
- [Voice activity detection](https://developers.openai.com/api/docs/guides/realtime-vad)
