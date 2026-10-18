# Upstream HTTP failures during verified rendering

A verified-renderer error containing "fetch renderer request: read renderer response"
comes from the pinned Go HTTP client fetching an admitted upstream document or
subresource. It is not a renderer RPC, Chrome stdout, or converter-output read.

On failure, the task's private failure-log record retains the first failed
exchange as renderer. Its task correlation and source process are in the
enclosing record. Selected exchange facts include:

- The exact requested URL, method, intercepted request and parent identities,
  document/subresource kind, and validated remote peer.
- The observed HTTP protocol and status, parsed declared content length
  (-1 means unknown), content encoding, and transfer encoding.
- The count of content-encoded bytes returned by the HTTP response body's
  Read method. Go's transport has already removed chunk framing; this count
  is not TCP size or a literal HTTP wire capture.
- Every available decoded body byte returned before failure, with byte length,
  hash, and reversible base64 representation. No prefix cap or text repair is
  applied by this diagnostic capture.
- The observed failure phase and separate gzip initialization, body read, gzip
  close, and HTTP body close error text. The original returned error chain
  remains intact.

A decoded body in not_reached state differs from a captured empty body.
decoded_body_complete:false means the response was not successfully read to
completion. A truncated gzip stream may yield all expected decoded text while
still failing its trailer or checksum; those bytes alone do not prove a valid,
complete response.

The capture does not archive arbitrary headers, cookies, transport authorization,
application API keys, or process configuration. It does preserve user-selected
URL and available response content faithfully. Successful response bodies are not
retained by this response-failure observer.

The original content-encoded stream, HTTP framing bytes, and missing network
bytes are not captured. An engineer can inspect selected framing and reproduce
a corresponding failure class with a controlled server, such as a short body
against the declared length. They cannot reconstruct the exact original gzip
stream, omitted bytes, remote process state, or incident cause from these facts.
Unexpected EOF alone does not identify an origin-server termination, intermediary
failure, or renderer shutdown. No retry is added.

Regression tests use actual loopback HTTP servers behind the existing pinned-peer
test seam. They cover short fixed-length bodies, missing final chunk framing,
truncated gzip trailers, invalid gzip headers, complete identity/gzip controls,
and cancellation after the body prefix is read. Failed partial bytes remain
unavailable for browser fulfillment and conversion; only the private failure log
retains the observed prefix. The enclosing journal's persistence and retention
requirements still apply.
