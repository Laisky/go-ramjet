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

Set the managed model endpoint to `openai.one_api_base` (default:
`https://oneapi.laisky.com`). To enable native GPTChat free tier, configure
exactly one `openai.user_tokens` entry whose `token` is
`DEFAULT_PROXY_TOKEN`, with its own restricted One API `openai_token`
and explicit `allowed_models`. Omit the entry to disable free inference.
The free entry cannot disable its limits or use another provider base; no
global `openai.token` or separate image credential/URL is used. Configure
positive model rate-limit intervals (the expensive model default is 600
seconds). Independently restrict the shared credential by model and quota
in One API. A missing or unavailable local policy denies shared-key inference.

The public `POST /gptchat/chat/oneshot` keeps its request and response
format but now requires an explicit caller-owned model key. Anonymous
and free-tier credentials are not accepted. Your selected provider receives
the original key, including on requests that fail upstream. Native
free-tier chat uses one admitted turn budget across model rounds, retries,
subagents and auxiliary model calls. Legacy Google Search and Ramjet URL
chunking require BYOK and are skipped in free-tier chat; neither service
receives the shared inference key.

If an Azure
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
For free-tier speech, allow both the GPTChat capability `tts` and the exact
configured speech model in GPTChat; enable the actual model in One API.

Catalog visibility is not a health check or a promise that a provider still
serves a model.
