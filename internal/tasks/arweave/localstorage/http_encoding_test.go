package localstorage

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/library/egress"
	"github.com/Laisky/go-ramjet/library/web"
)

// gzipBytes compresses content as one gzip member.
func gzipBytes(t *testing.T, content string) []byte {
	t.Helper()
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	_, err := io.WriteString(writer, content)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	return compressed.Bytes()
}

// TestCacheHandlerDecodesUnsolicitedGzip guards against relaying compressed bytes
// without their encoding: the proxy drops upstream headers, so an origin that
// gzips regardless of Accept-Encoding must still be served as its decoded content,
// with the type detected from the decoded bytes. HTML stays sandboxed and PDFs
// stay viewable.
func TestCacheHandlerDecodesUnsolicitedGzip(t *testing.T) {
	for _, tc := range []struct {
		name, content, contentType, policy string
	}{
		{"html", "<html><body><p>decoded page</p></body></html>", "text/html; charset=utf-8", web.UntrustedContentPolicy},
		{"pdf", "%PDF-1.7\n" + strings.Repeat("0", 4096), "application/pdf", web.PassiveContentPolicy},
	} {
		t.Run(tc.name, func(t *testing.T) {
			useArchiveSeams(t, notArchived, nil, nil)
			compressed := gzipBytes(t, tc.content)
			origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Encoding", "gzip")
				_, err := w.Write(compressed)
				require.NoError(t, err)
			}))
			t.Cleanup(origin.Close)
			usePublicTargetFixture(t, origin)

			recorder := serveCacheHandler("http://public.example/" + tc.name)
			require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
			require.Equal(t, "DYNAMIC", recorder.Header().Get("X-Laisky-Cache-Status"))
			require.Equal(t, tc.content, recorder.Body.String(), "the decoded content must be relayed, not the gzip bytes")
			require.Equal(t, tc.contentType, recorder.Header().Get("Content-Type"))
			require.Equal(t, tc.policy, recorder.Header().Get("Content-Security-Policy"))
			require.Empty(t, recorder.Header().Get("Content-Encoding"), "decoded content must not claim an encoding")
		})
	}
}

// TestCacheHandlerNegotiatesGzip keeps the compression the proxy negotiated before
// it moved onto the pinned transport: the live fetch advertises gzip, as net/http's
// default transport did, and serves a negotiated gzip response decoded.
func TestCacheHandlerNegotiatesGzip(t *testing.T) {
	useArchiveSeams(t, notArchived, nil, nil)
	const content = "<html><body>negotiated</body></html>"
	compressed := gzipBytes(t, content)
	var negotiated atomic.Int32
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			_, err := io.WriteString(w, content)
			require.NoError(t, err)
			return
		}
		negotiated.Add(1)
		w.Header().Set("Content-Encoding", "gzip")
		_, err := w.Write(compressed)
		require.NoError(t, err)
	}))
	t.Cleanup(origin.Close)
	usePublicTargetFixture(t, origin)

	recorder := serveCacheHandler("http://public.example/page")
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Equal(t, content, recorder.Body.String())
	require.EqualValues(t, 1, negotiated.Load(), "the live fetch must advertise gzip")
}

// TestLoadContentByUrlRefusesUndecodableEncoding fails the live source, instead of
// relaying bytes the client cannot interpret, when the origin answers with a
// content coding the proxy cannot decode. The response body is released.
func TestLoadContentByUrlRefusesUndecodableEncoding(t *testing.T) {
	useArchiveSeams(t, notArchived, nil, nil)
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Encoding", "br")
		_, err := io.WriteString(w, "\x1b\x03brotli-bytes")
		require.NoError(t, err)
	}))
	t.Cleanup(origin.Close)
	usePublicTargetFixture(t, origin)

	body, err := LoadContentByUrl(context.Background(), "http://public.example/page")
	require.Error(t, err)
	require.Nil(t, body)
	var unsupported *egress.UnsupportedEncodingError
	require.ErrorAs(t, err, &unsupported)
	require.Equal(t, "br", unsupported.Coding)
}
