# GPTChat voice calls and dictation

## User contract

Select **Realtime**, then press **Voice** once to start a phone-style conversation.
The microphone stays open across turns. Users can interrupt the AI, mute/unmute,
show/hide AI captions, and minimize/restore the call panel without reconnecting.
The red **Hang up** button ends the call, including while microphone permission
or the connection handshake is still pending. Reloading saved preferences never
starts the microphone automatically.

The audio implementation is a server setting, not a user preference. `openai.voice.plugin`
selects `realtime` or `whisper` and defaults to realtime; an absent or unrecognized value
resolves to realtime rather than silently downgrading a deployment to dictation. There is
no picker in the web UI. `openai.voice.realtime_model` selects the call model and defaults
to `gpt-realtime-2.1-mini`. Both ride the existing user-config response, are server-owned,
and overwrite anything a per-user entry holds, so a stale value cannot pin an old model.
An unknown plugin id fails at boot rather than leaving the browser with no audio.
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
     → authenticated GA Realtime WebSocket → the configured realtime model
     → native PCM audio deltas → browser playback
```

The call uses `marin`, semantic VAD, automatic responses, and automatic response
interruption. No GPTChat STT, text-completion, or TTS endpoint is called. AI captions
come from native output-audio transcript events.
MCP, web search, memory, images, and the text agent loop are not connected to this
voice session. This is not full ChatGPT Voice feature parity.

The output AudioContext is unlocked inside the explicit user gesture, before
permission awaits, and shared with capture. An AudioWorklet handles capture without
deprecated ScriptProcessor nodes.

That worklet is served verbatim from `public/audio/pcm-worklet.js` and is deliberately
never bundled. `AudioWorkletGlobalScope` cannot execute `import` statements, and the
bundler's worker transform injects one of its own in development on top of any the
source already has. The failure is quiet: the module body throws, `addModule()` still
resolves, `registerProcessor` never runs, and the node constructor fails with "The node
name 'gptchat-microphone' is not defined in AudioWorkletGlobalScope". The file therefore
carries its own copy of the resampler and contains no module syntax. Its tests evaluate
the shipped file directly, assert the registered processor name, and compare its output
against the shared resampler so the two cannot drift. Its resampler retains fractional sample
coverage across blocks. The browser worklet emits silence to the output graph,
not microphone monitoring audio.

This implementation retains the existing deployment's OpenAI-compatible WebSocket
route. It sends only the user-configured token in the authentication subprotocol;
no server-owned credential is returned to the browser. It does not request the
legacy beta protocol while sending GA events.

`gpt-realtime-2.1` is the current flagship Realtime model and is the id the gateway
serves; `gpt-realtime`, `-1.5`, `-2`, `-mini`, `-2.1-mini`, `-translate` and `-whisper`
are the other live members of that family. The legacy `gpt-4o[-mini]-realtime-preview`
models were shut down upstream on 2026-05-07, along with the whole Realtime beta
interface on 2026-05-12, so no `OpenAI-Beta` header and no `openai-beta.realtime-v1`
subprotocol may be sent.

The model is selected once, by the `model` query parameter of the connection URL.
`session.update` must never carry a `session.model` field. OpenAI accepts a value
matching the connection model, but its own event schema lists `model` as
non-updatable, and the OneAPI gateway rejects the frame outright with
`ws_model_switch_denied` and closes the socket with policy-violation code 1008.
Because the gateway checks for the key rather than comparing values, sending the
identical model string still ends the call before it starts. Provider/gateway authorization and
quota policy are authoritative; stale frontend `is_free`/`byok` labels do not
prove whether a particular gateway token has access. Direct mode uses its explicit
API base, rather than accidentally inheriting the account's proxy base.

For a new direct-to-OpenAI browser deployment, prefer server-issued short-lived
credentials and WebRTC. This PR does not introduce that bootstrap service or modify
the OneAPI repository. A custom gateway must support the GA session schema and the
requested model. No live provider compatibility test is implied by mocked tests.

## Writing the call into the text chat

A call is transcribed into the ordinary chat history of the session it started from,
so a voice conversation reads afterwards like a typed one. Both sides are recorded: the
assistant from its native output-audio transcript, and the caller from an input
transcription the session explicitly enables. Input transcription is a separate ASR job,
billed on its own and run out of band, so it adds no latency to the spoken reply. Its
text is the transcriber's opinion, not what the model heard, so treat it as a record of
the conversation rather than of the model's input.

The destination session is pinned when the call starts, like the rest of the routing
context. Transcripts follow the call, not the screen, so switching to another session
mid-call never splices a conversation into an unrelated one, and the rendered list is
only touched while the pinned session is the one on display.

Ordering does not follow arrival. Transcription runs asynchronously from the reply, so
the caller's text routinely lands after the assistant has already answered. Each turn
therefore reserves its slot when the input buffer is committed, before any text exists,
and is filled in later. A turn that fails to transcribe is marked rather than left open.
A turn holds the caller's utterance and the reply under one chat id, the same shape a
typed exchange uses. Interrupted answers are recorded from the partial transcript, which
is otherwise discarded on barge-in.

Live text streams into the view as it arrives and is written to storage at turn
boundaries, mirroring text chat, which streams into React state and persists once at the
end. Storage writes are serialized per session because a message and the session's
ordering index are separate keys; concurrent unserialized commits lose an index entry and
leave a stored message that nothing renders.

While a call is recording, the session it records into stops accepting manual history
changes: sending, editing, regenerating, deleting, and inserting a turn are all held, and
each control says why. This is a clarity measure, not the concurrency fix. It keeps a
typed turn from landing in the middle of one being transcribed and stops a user editing a
message the call is still writing. The per-session write queue is still required, because
the call itself issues overlapping writes. The hold applies only to the pinned session;
every other session stays fully editable, including while the call continues.

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

Realtime `error` events are fatal only before the configuration handshake completes,
because a call that was never configured cannot continue. Once the call is live,
an error event is reported to the user and the call stays open: these events are
per-event and mostly recoverable, and a provider that considers the call over closes
the socket, which the close handler already turns into an end. Treating every error
as fatal previously dropped otherwise healthy calls.

Microphone capture is gated on a secure context. When `navigator.mediaDevices` is
absent, the client distinguishes an insecure origin from a genuinely unsupported
browser, because plain HTTP on a LAN or tailnet address hides the API entirely.

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
