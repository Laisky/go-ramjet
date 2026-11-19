package egress

import (
	"net/http"

	"github.com/Laisky/errors/v2"
)

// ContentDecodingTransport negotiates and removes gzip around Base the way
// net/http's default transport does, for callers that read the decoded
// representation rather than the wire body. PinnedTransport and
// ProxiedTransport are wire-level; WithContentDecoding wraps them in this.
//
// A request that sets no Accept-Encoding, asks for no Range and is not a HEAD
// advertises AcceptEncoding. Its response is read through the same decoder as
// DecodeContent and loses Content-Encoding and Content-Length, which described
// the wire body; Uncompressed is set. A request that sets Accept-Encoding
// itself, and a response in a coding the decoder cannot remove, pass through
// unchanged with their headers, as with net/http. Decoded output is not
// size-bounded here; callers bound bodies as they would without decoding.
type ContentDecodingTransport struct {
	Base http.RoundTripper
}

// RoundTrip sends request through Base, negotiating and decoding gzip when
// request leaves content negotiation to the transport. request is not modified.
func (t ContentDecodingTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.Header.Get("Accept-Encoding") != "" || request.Header.Get("Range") != "" ||
		request.Method == http.MethodHead {
		response, err := t.Base.RoundTrip(request)
		if err != nil {
			return nil, errors.Wrap(err, "exchange without content decoding")
		}
		return response, nil
	}

	negotiated := request.Clone(request.Context())
	negotiated.Header.Set("Accept-Encoding", AcceptEncoding)
	response, err := t.Base.RoundTrip(negotiated)
	if err != nil {
		return nil, errors.Wrap(err, "exchange with content decoding")
	}
	layers, ok := removableGzipLayers(response.Header)
	if !ok || layers == 0 {
		return response, nil
	}
	response.Body = &decodedBody{source: response.Body, layers: layers}
	response.Header.Del("Content-Encoding")
	response.Header.Del("Content-Length")
	response.ContentLength = -1
	response.Uncompressed = true
	return response, nil
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
