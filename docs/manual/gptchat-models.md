# GPTChat model visibility and billing

The model picker shows GPTChat's supported models even for free users with a
narrow `allowed_models` list. Models outside a user's usage permissions are
marked **Unavailable**. The proxied One API model list also no longer filters
out names for free users; it still shows only what the upstream returns for the
existing token. Neither change expands the models a shared token can use or
removes quotas, rate limits, or request-time authorization.

One API prices and settles the models served by its relay, including
token-priced image models. GPTChat does not issue a second external charge for
image requests it sends to the configured HTTPS One API relay.

Existing direct image integrations that enable external billing read the
fixed `image_price` from One API's `/api/models/display` on the configured
billing origin. The single per-request quote applies to both upfront and
batch settlement. GPTChat does not send user credentials to the public
catalog, substitute an old price, or treat a missing price as free. If a
published fixed price is absent or ambiguous, use the One API relay rather
than a direct externally billed integration. For Azure's legacy generation
route, the deployment name from the configured URL identifies the model
because Azure does not use the request's model field; the name must exist in
the One API catalog for a fixed quote. The legacy Azure speech service still
uses its existing operation fee; it is not billed as a One API model.

Catalog visibility is not a live health check or a promise that an upstream
will accept any given request, especially when a provider has retired a model.
