# Crawler failure context: source findings (2026-10-18)

The accepted worker source `94dc8b3d4d24df72d44563439695cf33a66b3e37`
reports `empty_markdown` after successful local conversion when trimming the
returned Markdown leaves no text. The producer also rejects a verified-success
result with zero Markdown bytes. These branches establish an empty requested
result; the producer branch does not itself prove converter execution. Neither
branch establishes the original website's upstream cause.

The [html-to-markdown project](https://github.com/JohannesKaufmann/html-to-markdown)
documents local HTML conversion. A nonempty HTML document can contain no parsed
body text, or conversion can return no requested text. Future incidents therefore
need observed rendered/input/output sizes and parsed-text counts, rather than an
inferred historical target, authentication state or readiness explanation.

## Header observations

[Go 1.27.2 mime.ParseMediaType](https://pkg.go.dev/mime@go1.27.2#ParseMediaType)
also parses media-disposition values such as `inline`. A successful parse alone
does not prove a MIME type/subtype. Diagnostic projection additionally requires
a slash, keeps the parsed base media type and removes parameters. A malformed
or token-only header leaves content type absent; independently observed status
and conversion counts remain available.

Primary-document response headers are observations even when later gzip/body
processing fails. They are captured after the existing pinned HTTP exchange and
before those fallible operations, so a redirect followed by a failed response
does not inherit the redirect's status. Subresource/subframe responses cannot
overwrite the root document. None of this changes HTTP acceptance, body limits,
connection evidence, redirects or retries.

## Origin and conversion semantics

[Go URL.Hostname](https://pkg.go.dev/net/url@go1.27.2#URL.Hostname) removes an
authority's port and IPv6 brackets; it does not resolve DNS. Diagnostics use the
existing shared `crawleregress.RequestURL` representation: HTTP(S), no userinfo,
lowercase hostname and one terminal dot removed. They do not introduce IDNA or
IP-spelling normalization into task/policy binding or connection behavior.
Paths, query and fragments are not fields in this optional Redis object. The
original selected URL is separately emitted in ordinary failure logs.

The optional scalar `failure_context` keeps an observed zero distinct from an
unreached stage. Body text counts describe trimmed parsed text nodes outside
script/style; they do not claim CSS visibility. The existing converter, output
trim, error codes, task lifecycle, policy verification and success data remain
unchanged. The supplied incident is not yet bound to an originating instance or
target, so the added observations are prospective evidence, not a retrospective
upstream diagnosis.


## Original input in ordinary logs and alerts (updated requirement)

Scalar context remains a backward-compatible Redis diagnostic envelope. It is
not sufficient to reproduce conversion. Ordinary structured failure logs and
the existing alert path select the original queued URL, the last successfully
observed browser DOM snapshot and the single serialized BODY argument actually
passed to the pinned converter. An observed DOM snapshot may precede a later
rendering failure and is not necessarily returned, final or stable. Caller
diagnostics separately select the decoded declared URL before trimming, the
submitted URL and declared/resolved output option, joined by task ID.

Selected input is emitted without application-side previewing, filtering,
replacement or truncation. Unobserved stages remain distinguishable from
observed empty values. Source service, hostname, process-run identity and public
build provenance identify the emitter; no task/API key, arbitrary request
envelope, authorization headers or configuration is serialized.

There is no application-managed archive, new private API, replay CLI, storage
mount or capture-directory setting. Full-field transport through the existing
logger, SDK, alert receiver and final destination is a separate fidelity gate
owned by those components. Their limits must not be hidden by a successful
application fixture or silently worked around with another storage path.

Script-only HTML can reproduce empty Markdown with the real local converter.
Missing historical input remains an instrumentation/retention gap, not evidence
of a particular target or upstream cause. Reproduction fixtures use synthetic
input and do not retry the failed target.
