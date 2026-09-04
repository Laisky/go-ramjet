# GPTChat voice calls and dictation

## User contract

Select **Realtime**, then press **Voice** once to start a phone-style conversation.
The microphone stays open across turns. Users can interrupt the AI, mute/unmute,
show/hide AI captions, and minimize/restore the call panel without reconnecting.
The red **Hang up** button ends the call, including while microphone permission
or the connection handshake is still pending. Reloading saved preferences never
starts the microphone automatically.

Realtime is the default for new configurations. An explicitly saved Whisper
selection is preserved; unsupported plugin identifiers fall back to Whisper.
**Whisper** remains the record → transcription → editable draft workflow, with
the existing normal text-completion and per-message speech controls unchanged.

## Call ownership and session changes

`VoiceControls` hosts the Realtime plugin independently of the text session's
`enable_talk` preference. Each explicit call captures its initial API token,
resolved API base, system prompt, and session label. Switching text sessions does
not mutate, restart, or reroute that live call. Its panel continues showing the
original session label. The plugin selector is locked during a call. The Voice
button restores an existing call rather than opening another one.

Whisper dictation is different: it remains keyed to its text session and is
cancelled on a session switch, so a late transcription cannot enter another draft.

Only user hang-up and the model's completed `end_call` tool are normal call ends.
Provider closure, quota/session limits, lost devices, failed media/transport, and
page departure are exceptional ends with explicit feedback. A `beforeunload`
guard reduces accidental departure but cannot prevent process termination or
mobile browser suspension. There is no automatic redial or silent context reset.

## Native audio path

```text
Microphone → AudioWorklet → 20 ms / 24 kHz PCM16 frames
     → authenticated GA Realtime WebSocket → gpt-realtime-2.1
     → native PCM audio deltas → browser playback
```

The call uses `marin`, semantic VAD, automatic responses, and automatic response
interruption. No GPTChat STT, text-completion, or TTS endpoint is called. AI captions
come from native output-audio transcript events. User speech is not separately
transcribed, and the call is not imported into or persisted as text-chat history.
MCP, web search, memory, images, and the text agent loop are not connected to this
voice session. This is not full ChatGPT Voice feature parity.

The output AudioContext is unlocked inside the explicit user gesture, before
permission awaits, and shared with capture. A bundled AudioWorklet handles capture
without deprecated ScriptProcessor nodes. Its resampler retains fractional sample
coverage across blocks. The browser worklet emits silence to the output graph,
not microphone monitoring audio.

This implementation retains the existing deployment's OpenAI-compatible WebSocket
route. It sends only the user-configured token in the authentication subprotocol;
no server-owned credential is returned to the browser. It does not request the
legacy beta protocol while sending GA events. Provider/gateway authorization and
quota policy are authoritative; stale frontend `is_free`/`byok` labels do not
prove whether a particular gateway token has access. Direct mode uses its explicit
API base, rather than accidentally inheriting the account's proxy base.

For a new direct-to-OpenAI browser deployment, prefer server-issued short-lived
credentials and WebRTC. This PR does not introduce that bootstrap service or modify
the OneAPI repository. A custom gateway must support the GA session schema and the
requested model. No live provider compatibility test is implied by mocked tests.

## Correct lifecycle and interruption

Startup owns a cancellable lifetime. Hang-up settles pending setup immediately;
any late microphone grant is stopped, and a late worklet-module load cannot create
a graph. Contexts cannot reopen after disposal. All remote close codes, including
1000, release media. Configuration must be acknowledged with `session.updated`
before microphone frames are transmitted. Setup has a 20-second transport timeout.

`response.done` means generation completed, not that audio finished playing and
not that the call ended. The UI returns to Listening after scheduled audio drains.
VAD speech-start stops local output, invalidates queued deltas, rejects late output
from the interrupted response, and sends `conversation.item.truncate`, including
when zero milliseconds were heard. Heard duration excludes gaps between chunks.

The model can call `end_call({reason})` after a goodbye or when it must end the
conversation. Only a completed response with a valid tool call can initiate normal
AI hang-up; silence and ordinary response completion cannot. The client acknowledges
the tool, deduplicates its call ID, mutes input, drains goodbye audio, and disposes
the call exactly once. A 15-second drain deadline handles suspended/stalled playback.
The user can still hang up immediately while the AI goodbye is draining.

If WebSocket buffered output exceeds 512 KiB, the call ends with a slow-connection
error instead of accumulating indefinitely delayed speech. This is an explicit
transport failure, not a hidden normal hang-up or a claimed seamless recovery.

## Review reproduction and regression coverage

The review had four actual inline findings. Before fixes, deterministic behavior
tests reproduced both late-permission microphone leaks (Realtime and Whisper).
The already-present session-key and normal-remote-close fixes passed their behavior
checks; those findings were not treated as new failures. The pre-fix TypeScript
build also reproduced TS2345 at `AudioBuffer.copyToChannel`.

Regression tests now cover those cases plus permission/resume/worklet/handshake
cancellation, duplicate starts, GA acknowledgement, remote closes, continuous turns,
mute, backpressure, zero-position truncation, stale events, actual playback drain,
validated/deduplicated AI hang-up, user override, call UI controls, pinned account
identity, explicit-start-only behavior, and streaming resampling without drift.
Tests exercise public socket/media/UI boundaries; the older focused lifecycle tests
remain as additional regressions. Normal repository frontend CI runs all of them.

```sh
pnpm -C web test
pnpm -C web build
pnpm -C web exec vitest run src/pages/gptchat/audio src/pages/gptchat/components/__tests__/chat-input-audio-lifecycle.test.tsx
```

Manual release acceptance still needs a real browser and an authorized provider:
start with one Voice click; have several turns; interrupt mid-sentence; mute and
minimize; switch text sessions and verify the call label/account remain unchanged;
ask the AI to hang up; repeat with manual hang-up while permission is pending.
Confirm browser microphone release and no STT/TTS/text-completion requests in the
Realtime path. Mocked tests cannot measure hardware echo, real latency, mobile
suspension behavior, or actual model decisions.

## Sources checked on 2026-09-04

- [ChatGPT Voice](https://help.openai.com/en/articles/20001274): explicit voice entry,
  separate mute/end controls, natural interruptions, and text feedback.
- [GPT-Realtime-2.1](https://developers.openai.com/api/docs/models/gpt-realtime-2.1).
- [Realtime conversations](https://developers.openai.com/api/docs/guides/realtime-conversations):
  session/response separation, GA events, function calls, and client-side truncation.
  The documented API session maximum is 60 minutes; this is not an unlimited call.
- [Realtime WebSocket](https://developers.openai.com/api/docs/guides/realtime-websocket):
  browser authentication and the recommendation for short-lived credentials.
