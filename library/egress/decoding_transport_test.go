package egress

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
)

// recordingRoundTripper answers every request with respond and records the
// Accept-Encoding each request carried.
type recordingRoundTripper struct {
	respond   func(*http.Request) *http.Response
	encodings []string
}

// RoundTrip records request's Accept-Encoding and returns the canned response.
func (r *recordingRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	r.encodings = append(r.encodings, request.Header.Get("Accept-Encoding"))
	return r.respond(request), nil
}

// encodedResponse returns a response to request carrying body with the given Content-Encoding.
func encodedResponse(request *http.Request, coding string, body []byte) *http.Response {
	header := http.Header{"Content-Length": []string{strconv.Itoa(len(body))}}
	if coding != "" {
		header.Set("Content-Encoding", coding)
	}
	return &http.Response{
		StatusCode: http.StatusOK, Header: header, ContentLength: int64(len(body)),
		Body: io.NopCloser(bytes.NewReader(body)), Request: request,
	}
}

// TestContentDecodingTransportNegotiatesAndDecodes advertises gzip on a request
// that sets no Accept-Encoding, without modifying the caller's request, and
// returns the decoded representation with the wire-only headers removed, as
// net/http's default transport does.
func TestContentDecodingTransportNegotiatesAndDecodes(t *testing.T) {
	const content = "decoded representation"
	for _, coding := range []string{"gzip", "x-gzip", "gzip, gzip"} {
		t.Run(coding, func(t *testing.T) {
			layers := 1
			if coding == "gzip, gzip" {
				layers = 2
			}
			wire := gzipLayers(t, []byte(content), layers)
			base := &recordingRoundTripper{respond: func(request *http.Request) *http.Response {
				return encodedResponse(request, coding, wire)
			}}
			request := httptest.NewRequest(http.MethodGet, "http://public.example/page", nil)
			response, err := ContentDecodingTransport{Base: base}.RoundTrip(request)
			require.NoError(t, err)
			body, err := io.ReadAll(response.Body)
			require.NoError(t, errors.Join(err, response.Body.Close()))

			require.Equal(t, content, string(body))
			require.Equal(t, []string{AcceptEncoding}, base.encodings)
			require.Empty(t, request.Header.Get("Accept-Encoding"), "the caller's request must not be modified")
			require.Empty(t, response.Header.Get("Content-Encoding"))
			require.Empty(t, response.Header.Get("Content-Length"))
			require.EqualValues(t, -1, response.ContentLength)
			require.True(t, response.Uncompressed)
		})
	}
}

// TestContentDecodingTransportKeepsTheWireBody leaves negotiation to a request
// that sets Accept-Encoding itself, does not negotiate HEAD or Range requests,
// and relays a coding it cannot remove unchanged with its header, as net/http does.
func TestContentDecodingTransportKeepsTheWireBody(t *testing.T) {
	wire := gzipLayers(t, []byte("content"), 1)
	for _, tc := range []struct {
		name     string
		method   string
		header   http.Header
		coding   string
		expected string
	}{
		{"caller negotiates", http.MethodGet, http.Header{"Accept-Encoding": []string{"br"}}, "gzip", "br"},
		{"head", http.MethodHead, http.Header{}, "gzip", ""},
		{"range", http.MethodGet, http.Header{"Range": []string{"bytes=0-1"}}, "gzip", ""},
		{"unsupported coding", http.MethodGet, http.Header{}, "br", AcceptEncoding},
		{"too many layers", http.MethodGet, http.Header{}, "gzip, gzip, gzip", AcceptEncoding},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := &recordingRoundTripper{respond: func(request *http.Request) *http.Response {
				return encodedResponse(request, tc.coding, wire)
			}}
			request := httptest.NewRequest(tc.method, "http://public.example/page", nil)
			request.Header = tc.header
			response, err := ContentDecodingTransport{Base: base}.RoundTrip(request)
			require.NoError(t, err)
			body, err := io.ReadAll(response.Body)
			require.NoError(t, errors.Join(err, response.Body.Close()))

			require.Equal(t, wire, body)
			require.Equal(t, []string{tc.expected}, base.encodings)
			require.Equal(t, tc.coding, response.Header.Get("Content-Encoding"))
			require.Equal(t, strconv.Itoa(len(wire)), response.Header.Get("Content-Length"))
			require.False(t, response.Uncompressed)
		})
	}
}

// TestClientsDecodeContentOnRequest checks that WithContentDecoding gives the
// pinned and the proxied client the same decoded view of a gzip origin, while
// clients built without it keep the wire body.
func TestClientsDecodeContentOnRequest(t *testing.T) {
	const content = "decoded through the client"
	wire := gzipLayers(t, []byte(content), 1)
	var dials atomic.Int32
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Encoding", "gzip")
		_, _ = w.Write(wire)
	}))
	t.Cleanup(origin.Close)
	proxyURL, err := url.Parse(origin.URL)
	require.NoError(t, err)

	fetch := func(t *testing.T, client *http.Client) []byte {
		t.Helper()
		response, err := client.Get("http://public.example/page")
		require.NoError(t, err)
		body, err := io.ReadAll(response.Body)
		require.NoError(t, errors.Join(err, response.Body.Close()))
		return body
	}
	deps := publicFixture(t, origin, &dials)
	pinned := NewHTTPClient(5*time.Second, deps, WithContentDecoding())
	proxied, err := NewProxiedHTTPClient(5*time.Second, proxyURL, Dependencies{Resolve: publicResolver}, WithContentDecoding())
	require.NoError(t, err)
	require.Equal(t, content, string(fetch(t, pinned)))
	require.Equal(t, content, string(fetch(t, proxied)))

	wirePinned := NewHTTPClient(5*time.Second, deps)
	wireProxied, err := NewProxiedHTTPClient(5*time.Second, proxyURL, Dependencies{Resolve: publicResolver})
	require.NoError(t, err)
	require.Equal(t, wire, fetch(t, wirePinned))
	require.Equal(t, wire, fetch(t, wireProxied), "the proxied transport is wire-level like the pinned one")
}

// TestContentDecodingTransportSkipsBodilessResponses leaves responses that carry
// no body untouched, headers included, as net/http does for 1xx, 204, 304 and
// an explicit zero Content-Length.
func TestContentDecodingTransportSkipsBodilessResponses(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		length int64
		body   io.ReadCloser
	}{
		{"no content", http.StatusNoContent, 0, http.NoBody},
		{"not modified", http.StatusNotModified, 0, http.NoBody},
		{"switching protocols", http.StatusSwitchingProtocols, -1, http.NoBody},
		{"zero length", http.StatusOK, 0, http.NoBody},
		{"zero length with a body reader", http.StatusOK, 0, io.NopCloser(bytes.NewReader(nil))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := &recordingRoundTripper{respond: func(request *http.Request) *http.Response {
				return &http.Response{StatusCode: tc.status, ContentLength: tc.length, Body: tc.body, Request: request,
					Header: http.Header{"Content-Encoding": []string{"gzip"}, "Content-Length": []string{"0"}}}
			}}
			response, err := ContentDecodingTransport{Base: base}.RoundTrip(httptest.NewRequest(http.MethodGet, "http://public.example/", nil))
			require.NoError(t, err)
			require.Equal(t, []string{AcceptEncoding}, base.encodings)
			require.Equal(t, "gzip", response.Header.Get("Content-Encoding"))
			require.Equal(t, "0", response.Header.Get("Content-Length"))
			require.Equal(t, tc.length, response.ContentLength)
			require.False(t, response.Uncompressed)
			body, err := io.ReadAll(response.Body)
			require.NoError(t, errors.Join(err, response.Body.Close()))
			require.Empty(t, body)
		})
	}
}

// TestContentDecodingTransportBoundsDecodedSize stops a compressed body from
// expanding without bound in memory: a body the transport decodes fails with
// *DecodedSizeError past the bound, after at most the bound has been returned.
// A body at the bound, and a body the transport did not decode, are untouched.
func TestContentDecodingTransportBoundsDecodedSize(t *testing.T) {
	const limit = 64 << 10
	respond := func(coding string, body io.Reader) *recordingRoundTripper {
		return &recordingRoundTripper{respond: func(request *http.Request) *http.Response {
			header := http.Header{}
			if coding != "" {
				header.Set("Content-Encoding", coding)
			}
			return &http.Response{StatusCode: http.StatusOK, ContentLength: -1, Header: header,
				Body: io.NopCloser(body), Request: request}
		}}
	}
	read := func(t *testing.T, transport ContentDecodingTransport) ([]byte, error) {
		t.Helper()
		response, err := transport.RoundTrip(httptest.NewRequest(http.MethodGet, "http://public.example/", nil))
		require.NoError(t, err)
		body, readErr := io.ReadAll(response.Body)
		require.NoError(t, response.Body.Close())
		return body, readErr
	}

	body, err := read(t, ContentDecodingTransport{Base: respond("gzip", bytes.NewReader(gzipLayers(t, make([]byte, limit+1), 1))), MaxDecodedBytes: limit})
	var sizeErr *DecodedSizeError
	require.ErrorAs(t, err, &sizeErr)
	require.EqualValues(t, limit, sizeErr.Limit)
	require.Len(t, body, limit, "the bytes within the bound are delivered before the error")

	body, err = read(t, ContentDecodingTransport{Base: respond("gzip", bytes.NewReader(gzipLayers(t, make([]byte, limit), 1))), MaxDecodedBytes: limit})
	require.NoError(t, err)
	require.Len(t, body, limit)

	body, err = read(t, ContentDecodingTransport{Base: respond("", bytes.NewReader(make([]byte, 2*limit))), MaxDecodedBytes: limit})
	require.NoError(t, err, "a body the transport did not decode keeps its caller's own bound")
	require.Len(t, body, 2*limit)

	// Expanding DefaultMaxDecodedBytes takes too long for a unit test, so the
	// default is checked where it is chosen; the bound itself is exercised above.
	require.EqualValues(t, DefaultMaxDecodedBytes, ContentDecodingTransport{}.decodedLimit())
	require.EqualValues(t, limit, ContentDecodingTransport{MaxDecodedBytes: limit}.decodedLimit())
	client := NewHTTPClient(time.Second, Dependencies{}, WithContentDecoding())
	decoding, ok := client.Transport.(ContentDecodingTransport)
	require.True(t, ok)
	require.EqualValues(t, DefaultMaxDecodedBytes, decoding.decodedLimit(), "WithContentDecoding uses the default bound")
}

// TestContentDecodingClientStreamsWithoutBuffering keeps compressed event
// streams incremental: each flushed event reaches the reader before the origin
// writes the next one, so neither decoding nor its size bound buffers the body.
func TestContentDecodingClientStreamsWithoutBuffering(t *testing.T) {
	release := make(chan struct{})
	var dials atomic.Int32
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept-Encoding") != AcceptEncoding {
			http.Error(w, "gzip not negotiated", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Content-Encoding", "gzip")
		flusher, ok := w.(http.Flusher)
		if !ok {
			return
		}
		writer := gzip.NewWriter(w)
		for _, event := range []string{"first", "second"} {
			if _, err := io.WriteString(writer, "data: "+event+"\n\n"); err != nil {
				return
			}
			if err := writer.Flush(); err != nil {
				return
			}
			flusher.Flush()
			if event == "first" {
				select {
				case <-release:
				case <-r.Context().Done():
					return
				}
			}
		}
		_ = writer.Close()
	}))
	t.Cleanup(origin.Close)

	client := NewHTTPClient(5*time.Second, publicFixture(t, origin, &dials), WithContentDecoding())
	response, err := client.Get("http://public.example/stream")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, response.Body.Close()) })
	reader := bufio.NewReader(response.Body)
	line, err := reader.ReadString('\n')
	require.NoError(t, err)
	require.Equal(t, "data: first\n", line, "the first event must arrive before the origin sends the next")
	close(release)
	rest, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, "\ndata: second\n\n", string(rest))
}

// TestContentDecodingTransportBoundsDecodedErrorBodies gives answers outside
// 2xx a small decoded bound: error paths read the whole body and copy it into
// logs and error documents, and two stacked gzip layers expand by more than
// 100,000:1, so a few kilobytes must not become hundreds of MiB there.
func TestContentDecodingTransportBoundsDecodedErrorBodies(t *testing.T) {
	respond := func(status, size int) *recordingRoundTripper {
		wire := gzipLayers(t, make([]byte, size), 2)
		return &recordingRoundTripper{respond: func(request *http.Request) *http.Response {
			return &http.Response{StatusCode: status, ContentLength: int64(len(wire)), Request: request,
				Header: http.Header{"Content-Encoding": []string{"gzip, gzip"}}, Body: io.NopCloser(bytes.NewReader(wire))}
		}}
	}
	read := func(t *testing.T, base http.RoundTripper) ([]byte, error) {
		t.Helper()
		response, err := ContentDecodingTransport{Base: base}.RoundTrip(httptest.NewRequest(http.MethodGet, "http://public.example/", nil))
		require.NoError(t, err)
		body, readErr := io.ReadAll(response.Body)
		require.NoError(t, response.Body.Close())
		return body, readErr
	}
	for _, status := range []int{http.StatusBadRequest, http.StatusInternalServerError, http.StatusFound} {
		body, err := read(t, respond(status, DefaultMaxDecodedErrorBytes+1))
		var sizeErr *DecodedSizeError
		require.ErrorAs(t, err, &sizeErr, "status %d", status)
		require.EqualValues(t, DefaultMaxDecodedErrorBytes, sizeErr.Limit)
		require.Len(t, body, DefaultMaxDecodedErrorBytes)

		body, err = read(t, respond(status, DefaultMaxDecodedErrorBytes))
		require.NoError(t, err, "status %d", status)
		require.Len(t, body, DefaultMaxDecodedErrorBytes)
	}
	body, err := read(t, respond(http.StatusOK, DefaultMaxDecodedErrorBytes+1))
	require.NoError(t, err, "a 2xx body keeps the larger bound")
	require.Len(t, body, DefaultMaxDecodedErrorBytes+1)
}

// TestContentDecodingTransportAcceptsTheLargestBound keeps the bound from
// overflowing: math.MaxInt64 means effectively unbounded, not a panic.
func TestContentDecodingTransportAcceptsTheLargestBound(t *testing.T) {
	wire := gzipLayers(t, []byte("content"), 1)
	base := &recordingRoundTripper{respond: func(request *http.Request) *http.Response {
		return encodedResponse(request, "gzip", wire)
	}}
	transport := ContentDecodingTransport{Base: base, MaxDecodedBytes: math.MaxInt64}
	response, err := transport.RoundTrip(httptest.NewRequest(http.MethodGet, "http://public.example/", nil))
	require.NoError(t, err)
	body, err := io.ReadAll(response.Body)
	require.NoError(t, errors.Join(err, response.Body.Close()))
	require.Equal(t, "content", string(body))
}
