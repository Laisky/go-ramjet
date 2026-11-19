package egress

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

// closeTrackingBody is an in-memory body that records whether it was closed.
type closeTrackingBody struct {
	io.Reader
	closed atomic.Bool
}

// Close records the close and always succeeds.
func (b *closeTrackingBody) Close() error {
	b.closed.Store(true)
	return nil
}

// gzipLayers applies layers of gzip to content, innermost first.
func gzipLayers(t *testing.T, content []byte, layers int) []byte {
	t.Helper()
	for range layers {
		var compressed bytes.Buffer
		writer := gzip.NewWriter(&compressed)
		_, err := writer.Write(content)
		require.NoError(t, err)
		require.NoError(t, writer.Close())
		content = compressed.Bytes()
	}
	return content
}

// headerWith returns a header holding each value as its own Content-Encoding field line.
func headerWith(values ...string) http.Header {
	header := http.Header{}
	for _, value := range values {
		header.Add("Content-Encoding", value)
	}
	return header
}

// TestDecodeContentRemovesSupportedCodings decodes every spelling of the supported
// coding, stacked codings across one or several field lines, and leaves bodies
// without a coding untouched.
func TestDecodeContentRemovesSupportedCodings(t *testing.T) {
	const content = "<html>representation</html>"
	for _, tc := range []struct {
		name   string
		header http.Header
		layers int
	}{
		{"absent", http.Header{}, 0},
		{"identity", headerWith("identity"), 0},
		{"gzip", headerWith("gzip"), 1},
		{"case and spacing", headerWith(" GZip "), 1},
		{"x-gzip alias", headerWith("x-gzip"), 1},
		{"identity around gzip", headerWith("identity, gzip,"), 1},
		{"stacked in one line", headerWith("gzip, gzip"), 2},
		{"stacked across lines", headerWith("gzip", "x-gzip"), 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := &closeTrackingBody{Reader: bytes.NewReader(gzipLayers(t, []byte(content), tc.layers))}
			body, err := DecodeContent(tc.header, source)
			require.NoError(t, err)
			if tc.layers == 0 {
				require.Same(t, source, body, "a body without codings is returned unchanged")
			}
			decoded, err := io.ReadAll(body)
			require.NoError(t, err)
			require.Equal(t, content, string(decoded))
			require.NoError(t, body.Close())
			require.True(t, source.closed.Load(), "closing the decoded body closes the source")
		})
	}
}

// TestDecodeContentRefusesUnsupportedCodings refuses, before reading anything, every
// coding it cannot remove, including one stacked behind a supported coding, and
// leaves the source with the caller.
func TestDecodeContentRefusesUnsupportedCodings(t *testing.T) {
	for _, tc := range []struct{ header, coding string }{
		{"br", "br"},
		{"deflate", "deflate"},
		{"zstd", "zstd"},
		{"compress", "compress"},
		{"gzip, br", "br"},
		{"Br, gzip", "br"},
	} {
		t.Run(tc.header, func(t *testing.T) {
			source := &closeTrackingBody{Reader: strings.NewReader("encoded")}
			body, err := DecodeContent(headerWith(tc.header), source)
			require.Nil(t, body)
			var unsupported *UnsupportedEncodingError
			require.ErrorAs(t, err, &unsupported)
			require.Equal(t, tc.coding, unsupported.Coding)
			require.False(t, source.closed.Load(), "a refused body stays owned by the caller")
		})
	}
}

// TestDecodeContentBoundsStackedCodings guards against a response that lists many
// gzip layers: each layer costs a decoder of tens of kilobytes and a pass over the
// body, so an origin could exhaust memory with one header. More layers than the
// bound are refused before any decoder is built or any byte is read.
func TestDecodeContentBoundsStackedCodings(t *testing.T) {
	for _, header := range []http.Header{
		headerWith("gzip, gzip, gzip"),
		headerWith("gzip, identity, gzip", "x-gzip"),
		headerWith(strings.TrimSuffix(strings.Repeat("gzip,", 10000), ",")),
	} {
		source := &closeTrackingBody{Reader: &failingReader{t: t}}
		body, err := DecodeContent(header, source)
		require.Nil(t, body)
		var unsupported *UnsupportedEncodingError
		require.ErrorAs(t, err, &unsupported)
		require.Equal(t, "gzip", unsupported.Coding)
		require.Greater(t, unsupported.Layers, maxContentCodings)
		require.Less(t, len(err.Error()), 200, "the refusal must not echo the whole header")
		require.False(t, source.closed.Load())
	}
}

// failingReader fails the test when it is read.
type failingReader struct{ t *testing.T }

// Read reports an unexpected read.
func (r *failingReader) Read([]byte) (int, error) {
	r.t.Error("a refused body must not be read")
	return 0, io.EOF
}

// TestDecodeContentDecodesLazily mirrors net/http's transparent gzip: an empty body
// labeled gzip is an empty representation, a body that is never read never fails,
// and a corrupt body fails on read rather than on construction.
func TestDecodeContentDecodesLazily(t *testing.T) {
	empty, err := DecodeContent(headerWith("gzip"), io.NopCloser(strings.NewReader("")))
	require.NoError(t, err)
	decoded, err := io.ReadAll(empty)
	require.NoError(t, err)
	require.Empty(t, decoded)

	unread := &closeTrackingBody{Reader: strings.NewReader("not gzip")}
	body, err := DecodeContent(headerWith("gzip"), unread)
	require.NoError(t, err, "a body is not read before the caller reads it")
	require.NoError(t, body.Close())
	require.True(t, unread.closed.Load())

	corrupt, err := DecodeContent(headerWith("gzip"), io.NopCloser(strings.NewReader("not gzip at all")))
	require.NoError(t, err)
	_, err = io.ReadAll(corrupt)
	require.ErrorIs(t, err, gzip.ErrHeader)

	truncated := gzipLayers(t, []byte(strings.Repeat("x", 4096)), 1)
	cut, err := DecodeContent(headerWith("gzip"), io.NopCloser(bytes.NewReader(truncated[:len(truncated)/2])))
	require.NoError(t, err)
	_, err = io.ReadAll(cut)
	require.ErrorIs(t, err, io.ErrUnexpectedEOF, "a truncated stream must not read as complete")
}
