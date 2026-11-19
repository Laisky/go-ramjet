package egress

import (
	"fmt"
	"io"
	"net/http"

	"github.com/Laisky/errors/v2"
)

// DefaultMaxDecodedBytes bounds a 2xx body ContentDecodingTransport returns
// when its MaxDecodedBytes is zero. Deflate expands up to about 1000:1 per
// layer and DecodeContent removes two stacked layers, so a few kilobytes on the
// wire can decode to hundreds of MiB (256 MiB of zeros double-gzips to under
// 3 KB, over 100,000:1); only an absolute bound on decoded bytes holds.
//
// The largest legitimate decoded bodies on these clients are non-streaming
// image-generation answers: at most 8 images (the handler's clamp), each a PNG
// of a few MiB (8 MiB is already beyond what image APIs return), base64-encoded
// in JSON, so about 8 x 8 MiB x 4/3 = 85 MiB. Responses event streams bound each
// event line at 20 MiB and carry far less in total. 256 MiB leaves three times
// the largest of those while capping a gzip bomb at a fixed size.
const DefaultMaxDecodedBytes = 256 << 20

// DefaultMaxDecodedErrorBytes bounds the body of an answer outside 2xx, decoded or not.
// Error paths read the whole body and copy it into logs and error documents,
// several times over, while legitimate error documents are a few KiB; 1 MiB
// keeps every real one and stops an expanded body from multiplying in memory.
const DefaultMaxDecodedErrorBytes = 1 << 20

// DecodedSizeError reports a response body that grew past its bound, counted
// after any content decoding. It is returned by Read, after the bytes within
// the bound, so a caller can tell a refused body from a transport failure with
// errors.As.
type DecodedSizeError struct {
	// Limit is the bound, in decoded bytes, that the body exceeded.
	Limit int64
}

// Error names the bound without echoing any response content.
func (e *DecodedSizeError) Error() string {
	return fmt.Sprintf("decoded content exceeds %d bytes", e.Limit)
}

// ContentDecodingTransport negotiates and removes gzip around Base for callers
// that read the decoded representation rather than the wire body, much like
// net/http's transparent gzip. PinnedTransport and ProxiedTransport are
// wire-level; WithContentDecoding wraps them in this.
//
// Like net/http, a request that sets no Accept-Encoding, asks for no Range and
// is not a HEAD advertises AcceptEncoding (gzip); a request that sets
// Accept-Encoding itself is passed through unchanged, and so is a response
// without a body (1xx, 204, 304 or Content-Length: 0), headers included. A
// decoded response loses Content-Encoding and Content-Length, which described
// the wire body, and gets Uncompressed set.
//
// Unlike net/http, it uses DecodeContent's rules: "x-gzip" counts as gzip and
// up to two stacked gzip layers are removed, while a coding DecodeContent
// refuses is passed through unchanged with its header. It also bounds every
// body it returns, decoded or not, to MaxDecodedBytes (DefaultMaxDecodedBytes
// when zero): Read fails with *DecodedSizeError past the bound. An answer
// outside 2xx is bounded to DefaultMaxDecodedErrorBytes, or MaxDecodedBytes
// when that is smaller. Plain bodies are bounded too because the callers of
// these clients read error and JSON bodies whole. The bound counts bytes as
// they are read and never buffers, so streams stay incremental. A request
// without a header map is refused, as net/http refuses it.
type ContentDecodingTransport struct {
	Base http.RoundTripper
	// MaxDecodedBytes bounds every 2xx body the transport returns, decoded or
	// not; zero means DefaultMaxDecodedBytes. A body outside 2xx is bounded to
	// the smaller of it and DefaultMaxDecodedErrorBytes.
	MaxDecodedBytes int64
}

// RoundTrip sends request through Base, negotiating and decoding gzip when
// request leaves content negotiation to the transport. request is not modified.
func (t ContentDecodingTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.Header == nil {
		return nil, closeRequestBody(request, errors.New("nil request header"))
	}
	if request.Header.Get("Accept-Encoding") != "" || request.Header.Get("Range") != "" ||
		request.Method == http.MethodHead {
		response, err := t.Base.RoundTrip(request)
		if err != nil {
			return nil, errors.Wrap(err, "exchange without content decoding")
		}
		return t.bound(response), nil
	}

	negotiated := request.Clone(request.Context())
	negotiated.Header.Set("Accept-Encoding", AcceptEncoding)
	response, err := t.Base.RoundTrip(negotiated)
	if err != nil {
		return nil, errors.Wrap(err, "exchange with content decoding")
	}
	if bodiless(response) {
		return response, nil
	}
	layers, ok := removableGzipLayers(response.Header)
	if !ok || layers == 0 {
		return t.bound(response), nil
	}
	response.Body = &decodedBody{source: response.Body, layers: layers}
	response.Header.Del("Content-Encoding")
	response.Header.Del("Content-Length")
	response.ContentLength = -1
	response.Uncompressed = true
	return t.bound(response), nil
}

// bound limits the body of response, unless it has none, to the bound for its
// status, and returns response.
func (t ContentDecodingTransport) bound(response *http.Response) *http.Response {
	if bodiless(response) {
		return response
	}
	limit := t.decodedLimit()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		limit = min(limit, DefaultMaxDecodedErrorBytes)
	}
	response.Body = &boundedBody{ReadCloser: response.Body, remaining: limit, limit: limit}
	return response
}

// decodedLimit returns MaxDecodedBytes, or DefaultMaxDecodedBytes when it is not positive.
func (t ContentDecodingTransport) decodedLimit() int64 {
	if t.MaxDecodedBytes <= 0 {
		return DefaultMaxDecodedBytes
	}
	return t.MaxDecodedBytes
}

// bodiless reports whether response carries no body, which net/http never decodes.
func bodiless(response *http.Response) bool {
	switch {
	case response.StatusCode < http.StatusOK,
		response.StatusCode == http.StatusNoContent,
		response.StatusCode == http.StatusNotModified,
		response.ContentLength == 0,
		response.Body == nil,
		response.Body == http.NoBody:
		return true
	default:
		return false
	}
}

// boundedBody returns at most limit bytes of its ReadCloser and then fails
// with *DecodedSizeError if more remain. It reads through, never buffering.
type boundedBody struct {
	io.ReadCloser
	remaining int64
	limit     int64
}

// Read returns the next bytes within the bound. It reads one byte past the
// bound at most, to tell a body that ends exactly at the bound from a longer one.
// The comparison avoids remaining+1, which overflows for a math.MaxInt64 bound.
func (b *boundedBody) Read(p []byte) (int, error) {
	if b.remaining < 0 {
		return 0, errors.WithStack(&DecodedSizeError{Limit: b.limit})
	}
	if int64(len(p)) > b.remaining {
		p = p[:b.remaining+1]
	}
	n, err := b.ReadCloser.Read(p)
	b.remaining -= int64(n)
	if b.remaining < 0 {
		return n + int(b.remaining), errors.WithStack(&DecodedSizeError{Limit: b.limit})
	}
	return n, err
}

// removableGzipLayers returns how many gzip layers DecodeContent removes from a
// response with header, and false when DecodeContent would refuse its codings.
func removableGzipLayers(header http.Header) (int, bool) {
	layers, err := countGzipLayers(header)
	if err != nil {
		return 0, false
	}
	return layers, true
}
