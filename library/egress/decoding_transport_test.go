package egress

import (
	"bytes"
	"io"
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
