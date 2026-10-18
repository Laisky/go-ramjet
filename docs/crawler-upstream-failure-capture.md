# Upstream HTTP failures during verified rendering

A verified-renderer error containing "fetch renderer request: read renderer response"
comes from the pinned Go HTTP client fetching an admitted upstream document or
subresource. It is not a renderer RPC, Chrome stdout, or converter-output read.

The ordinary structured failure log and existing alert carry the first observed
failed exchange alongside task/request and application source correlation:

- The requested URL, method, intercepted request and parent identities,
  document/subresource kind, and validated remote peer.
- The observed HTTP protocol and status, parsed declared content length
  (-1 means unknown), content encoding, and transfer encoding.
- The count of content-encoded bytes returned by the HTTP response body's
  Read method. Go's transport has already removed chunk framing; this count
  is not TCP size or a literal HTTP wire capture.
- Every available decoded body byte returned before failure, its observed length
  and whether decoding was reached. The observer does not shorten or replace
  these selected bytes.
- The observed failure phase and separate gzip initialization, body read, gzip
  close, and HTTP body close error text. The original returned error chain
  remains intact.

An unobserved decoded body differs from an observed empty body.
A body not read successfully to completion remains partial. A truncated gzip
stream may yield all expected decoded text while still failing its trailer or
checksum; those bytes alone do not prove a valid, complete response.

The observer does not select arbitrary headers, cookies, transport authorization,
application API keys, or process configuration. It does select the requested URL
and available response content faithfully. Successful response bodies are not
added by this response-failure observer.

Original content-encoded streams, HTTP framing bytes and missing network bytes
are not observed. An engineer can inspect selected framing and reproduce a
corresponding failure class with a controlled server, such as a short body
against its declared length. These facts cannot reconstruct the exact original
gzip stream, omitted bytes, remote process state or historical incident cause.
Unexpected EOF alone does not identify an origin-server termination, intermediary
failure or renderer shutdown. Joined read/close errors can repeat EOF text
without proving repeated fetches. No retry is added.

Regression fixtures use actual loopback HTTP servers behind the existing
pinned-peer test seam. They cover short fixed-length bodies, missing final chunk
framing, truncated gzip trailers, invalid gzip headers, complete identity/gzip
controls and cancellation after a body prefix is read. Failed partial bytes
remain unavailable for browser fulfillment and conversion; they are diagnostic
fields in ordinary logs and alerts.

There is no application archive, new storage directory, private archive API,
mount requirement or custom payload codec. Complete field delivery through the
existing logging/alert pipeline must be qualified separately with its owners;
a successful local HTTP fixture does not establish real notification delivery,
backend retention or absence of downstream size limits.
