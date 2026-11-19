package egress

import (
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/Laisky/errors/v2"
)

// AcceptEncoding is the Accept-Encoding value that callers of PinnedTransport
// advertise when they decode responses with DecodeContent. It lists exactly the
// codings DecodeContent supports and matches what net/http's default transport
// requests, so moving a caller onto PinnedTransport keeps its negotiation.
const AcceptEncoding = "gzip"

// maxContentCodings is how many stacked gzip layers DecodeContent removes. Every
// layer costs a decoder of tens of kilobytes and a pass over the body, so an
// unbounded stack would let one response header exhaust memory. net/http removes
// one layer; two also covers an application and a front proxy both compressing.
const maxContentCodings = 2

// UnsupportedEncodingError reports a response Content-Encoding that DecodeContent
// refuses: a coding it cannot remove, or more stacked gzip layers than it removes.
// Relaying such a body without its Content-Encoding would hand the reader bytes
// it cannot interpret, so the response must be refused instead.
type UnsupportedEncodingError struct {
	// Coding is the refused coding, lower-cased as it appeared in Content-Encoding.
	Coding string
	// Layers is how many layers of Coding were listed when the stack was refused
	// for its depth, and zero when Coding itself is unsupported.
	Layers int
}

// Error names the refused coding without echoing the header, which the response controls.
func (e *UnsupportedEncodingError) Error() string {
	if e.Layers > 0 {
		return fmt.Sprintf("unsupported content encoding: %d stacked %q layers, at most %d are decoded",
			e.Layers, e.Coding, maxContentCodings)
	}
	return "unsupported content encoding " + strconv.Quote(e.Coding)
}

// DecodeContent returns body with the content codings listed in header's
// Content-Encoding removed, so the caller reads the representation itself.
//
// Codings are listed in the order they were applied and are removed in reverse
// (RFC 9110 section 8.4). Empty entries and "identity" are ignored, and "x-gzip"
// is treated as "gzip" (RFC 9110 section 8.4.1.3). At most maxContentCodings
// layers are removed; a deeper stack is refused. When nothing remains, body is
// returned unchanged. Decoders are created on the first Read, as net/http's own
// transparent gzip does, so an empty or never-read body does not fail.
//
// The returned reader's Close closes body. When a listed coding is unsupported,
// DecodeContent returns *UnsupportedEncodingError and body stays owned by the caller.
// Decoded output is not size-bounded here; callers bound it as they bound bodies.
func DecodeContent(header http.Header, body io.ReadCloser) (io.ReadCloser, error) {
	layers, err := countGzipLayers(header)
	if err != nil {
		return nil, err
	}
	if layers == 0 {
		return body, nil
	}
	return &decodedBody{source: body, layers: layers}, nil
}

// countGzipLayers counts the gzip layers named by every Content-Encoding field line
// in header. It refuses the first coding that cannot be removed and a stack
// deeper than maxContentCodings, reporting how many layers were listed.
func countGzipLayers(header http.Header) (int, error) {
	layers := 0
	for _, line := range header.Values("Content-Encoding") {
		for coding := range strings.SplitSeq(line, ",") {
			switch coding = strings.ToLower(strings.TrimSpace(coding)); coding {
			case "", "identity":
			case "gzip", "x-gzip":
				layers++
			default:
				return 0, errors.WithStack(&UnsupportedEncodingError{Coding: coding})
			}
		}
	}
	if layers > maxContentCodings {
		return 0, errors.WithStack(&UnsupportedEncodingError{Coding: "gzip", Layers: layers})
	}
	return layers, nil
}

// decodedBody removes layers of gzip from source, building its decoders lazily.
type decodedBody struct {
	source  io.ReadCloser
	layers  int
	started bool
	reader  io.Reader
	err     error
}

// Read returns decoded bytes. A body that ends before the first gzip header is
// an empty representation and reports io.EOF; any other decoder error is returned.
func (b *decodedBody) Read(p []byte) (int, error) {
	if !b.started {
		b.started = true
		b.reader, b.err = b.decoders()
	}
	if b.err != nil {
		return 0, b.err
	}
	return b.reader.Read(p)
}

// decoders stacks one gzip reader per layer over source, outermost coding last.
func (b *decodedBody) decoders() (io.Reader, error) {
	var reader io.Reader = b.source
	for range b.layers {
		decoder, err := gzip.NewReader(reader)
		if errors.Is(err, io.EOF) {
			// io.Reader callers compare against the bare sentinel, so it is not wrapped.
			return nil, io.EOF
		}
		if err != nil {
			return nil, errors.Wrap(err, "decode gzip content")
		}
		reader = decoder
	}
	return reader, nil
}

// Close closes the source body. The gzip decoders hold no other resources.
func (b *decodedBody) Close() error {
	return errors.WithStack(b.source.Close())
}
