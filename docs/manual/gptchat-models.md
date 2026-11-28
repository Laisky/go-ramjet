# GPTChat models, access and billing

The GPTChat picker shows all supported models to free users. Entries outside
the configured usage allowlist are marked **Unavailable**. Discovery does not
grant inference permission. The existing allowlists, free and expensive model
limits (including the 10-minute expensive-model interval), request bounds and
application token reservations still apply to ordinary GPTChat requests.

## Billing ownership

One API handles routing, pricing, usage accounting and settlement for all
platform-managed inference. When a user selects a provider with a caller-owned
key, that provider handles its own billing. Go-ramjet does not look up a price,
check a billing balance, or submit a second inference charge. Ordinary payment
and top-up flows remain separate.

## Credentials and routes

- `/gptchat/oneapi/*` is for caller-owned One API tokens only. Send exactly one
  `Authorization: Bearer <token>` header. The proxy does not accept or replace
  free-tier and session credentials, filter model lists, or retry with a
  shared key. One API enforces the caller token's permissions and quotas.
- Free-tier discovery uses the static picker. Free-tier microphone requests go
  to `POST /gptchat/audio/transcriptions`, which accepts one file (up to 25 MiB)
  and the `whisper-1` model. It applies GPTChat's model policy and sends the
  request to One API. Enable `whisper-1` in the free-tier allowlist and the
  corresponding One API token scope if you offer free-tier microphone input.
  Caller-selected provider audio uses the existing tenant egress checks.
- The SPA continues to show One API's reported request cost for One API
  caller-owned keys. It does not query a shared token's costs, or send a
  different provider's key to One API for cost lookup.

## Operator migration

Point platform-managed model credentials and endpoints at One API. If an Azure
deployment is used for image generation, configure its channel and model
mapping in One API first. The public image generation and editing endpoints,
including the historical Flux aliases, still accept the same client requests.
The private direct Azure/Replicate implementation and local external billing
settings are no longer used. No stored image or billing data is migrated or
deleted by this change.

The existing GET `/gptchat/audio/tts` endpoint still returns WAV audio. Configure
an enabled WAV-capable One API model and voice using
`openai.voice.speech_model` and `openai.voice.speech_voice`. Azure Speech SDK
settings are no longer used. Before publishing, verify the image, transcription
and speech models are enabled for the applicable One API tokens and channels.

Catalog visibility is not a health check or a promise that a provider still
serves a model.
