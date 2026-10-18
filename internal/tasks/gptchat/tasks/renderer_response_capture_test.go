package tasks

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/laisky-blog-graphql/library/crawleregress"
	"github.com/stretchr/testify/require"
)

// TestRendererFailedHTTPResponseCapture exercises actual short HTTP bodies and a complete control.
// All connections remain on loopback behind the existing synthetic public-peer test seam.
func TestRendererFailedHTTPResponseCapture(t *testing.T) {
	prefix := []byte("<html><body>original café 世界")
	for _, mode := range []string{"fixed_length", "chunked", "gzip_trailer", "complete", "gzip_complete", "gzip_header"} {
		t.Run(mode, func(t *testing.T) {
			var compressed bytes.Buffer
			writer := gzip.NewWriter(&compressed)
			_, err := writer.Write(prefix)
			require.NoError(t, err)
			require.NoError(t, writer.Close())
			gzipBody := compressed.Bytes()
			fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Empty(t, r.Header.Get("Authorization"))
				require.Empty(t, r.Header.Get("Cookie"))
				switch mode {
				case "fixed_length":
					w.Header().Set("Content-Length", fmt.Sprint(len(prefix)+47))
					w.Header().Set("Content-Type", "text/html; charset=utf-8")
					_, writeErr := w.Write(prefix)
					require.NoError(t, writeErr)
				case "chunked":
					conn, rw, hijackErr := w.(http.Hijacker).Hijack()
					require.NoError(t, hijackErr)
					defer func() { require.NoError(t, conn.Close()) }()
					_, writeErr := fmt.Fprintf(rw, "HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\nContent-Type: text/html\r\n\r\n%x\r\n%s\r\n", len(prefix), prefix)
					require.NoError(t, writeErr)
					require.NoError(t, rw.Flush())
				case "gzip_trailer":
					body := gzipBody[:len(gzipBody)-8]
					w.Header().Set("Content-Encoding", "gzip")
					w.Header().Set("Content-Length", fmt.Sprint(len(body)))
					_, writeErr := w.Write(body)
					require.NoError(t, writeErr)
				case "gzip_header":
					w.Header().Set("Content-Encoding", "gzip")
					_, writeErr := w.Write([]byte("x"))
					require.NoError(t, writeErr)
				case "gzip_complete":
					w.Header().Set("Content-Encoding", "gzip")
					_, writeErr := w.Write(gzipBody)
					require.NoError(t, writeErr)
				default:
					_, writeErr := w.Write(prefix)
					require.NoError(t, writeErr)
				}
			}))
			t.Cleanup(fixture.Close)
			var hits atomic.Int32
			target := "http://example.com/input?chosen=synthetic-canary"
			ctx, holder := withRendererFailureObservation(context.Background())
			fetcher, err := newRendererEgressFetcher(ctx, "capture-task", target, rendererTestPolicy("example.com", "93.184.216.34"), rendererTestDependencies(t, fixture, "93.184.216.34", &hits))
			require.NoError(t, err)
			headers := http.Header{"Authorization": {"Bearer transport-secret-canary"}, "Cookie": {"transport-cookie-canary"}}
			response, body, err := fetcher.fetch(ctx, "request-id", "", target, crawleregress.OriginDocument, http.MethodGet, headers)
			require.EqualValues(t, 1, hits.Load(), "failure capture must not retry network work")
			if mode == "complete" || mode == "gzip_complete" {
				require.NoError(t, err)
				require.Equal(t, prefix, body)
				require.NotNil(t, response)
				require.Nil(t, holder.Snapshot(), "successful response bodies are not retained")
				return
			}
			require.ErrorIs(t, err, io.ErrUnexpectedEOF)
			require.Nil(t, response)
			require.Nil(t, body, "partial bytes must never be fulfilled to the browser")
			require.Contains(t, err.Error(), map[bool]string{true: "decode renderer gzip", false: "read renderer response"}[mode == "gzip_header"])
			captured := holder.Snapshot()
			require.NotNil(t, captured)
			require.Equal(t, "request-id", captured.RequestID)
			require.Equal(t, crawleregress.OriginDocument, captured.Kind)
			require.Equal(t, http.MethodGet, captured.Method)
			require.Equal(t, target, captured.URL)
			require.Equal(t, http.StatusOK, captured.UpstreamStatus)
			require.Equal(t, "93.184.216.34", captured.PeerAddress)
			require.Equal(t, "HTTP/1.1", captured.Protocol)
			require.Positive(t, captured.EncodedBodyBytes)
			require.False(t, captured.DecodedBodyComplete)
			if mode == "gzip_header" {
				require.Equal(t, "decode", captured.FailurePhase)
				require.NotEmpty(t, captured.DecodeError)
				require.False(t, captured.DecodedBodyObserved)
			} else {
				require.Equal(t, "read", captured.FailurePhase)
				require.Equal(t, prefix, captured.DecodedBody)
				require.NotEmpty(t, captured.ReadError)
			}
			switch mode {
			case "fixed_length":
				require.EqualValues(t, len(prefix)+47, captured.DeclaredContentLength)
				require.Empty(t, captured.TransferEncoding)
				require.Empty(t, captured.ContentEncoding)
			case "chunked":
				require.EqualValues(t, -1, captured.DeclaredContentLength)
				require.Equal(t, []string{"chunked"}, captured.TransferEncoding)
			case "gzip_trailer", "gzip_header":
				require.Equal(t, "gzip", captured.ContentEncoding)
			}
			serialized, marshalErr := json.Marshal(captured)
			require.NoError(t, marshalErr)
			require.NotContains(t, string(serialized), "transport-secret-canary")
			require.NotContains(t, string(serialized), "transport-cookie-canary")
			requireRendererFailureLogFields(t, target, captured)
			captured.URL = "changed snapshot URL"
			mutatedBody := captured.DecodedBody
			if len(mutatedBody) > 0 {
				mutatedBody[0] = 'X'
			}
			require.Equal(t, target, holder.Snapshot().URL, "snapshot bytes have independent ownership")
		})
	}
}

// TestRendererFailedResponseCancellation retains the available prefix without misclassifying cancellation.
func TestRendererFailedResponseCancellation(t *testing.T) {
	prefix := []byte("partial before cancellation")
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "10000")
		_, err := w.Write(prefix)
		require.NoError(t, err)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	t.Cleanup(fixture.Close)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx, holder := withRendererFailureObservation(ctx)
	var hits atomic.Int32
	target := "http://example.com/input"
	deps := rendererTestDependencies(t, fixture, "93.184.216.34", &hits)
	baseDial := deps.dial
	deps.dial = func(dialCtx context.Context, network, address string) (net.Conn, error) {
		conn, err := baseDial(dialCtx, network, address)
		if err != nil {
			return nil, err
		}
		return &rendererCancelAfterPrefixConn{Conn: conn, prefix: prefix, cancel: cancel}, nil
	}
	fetcher, err := newRendererEgressFetcher(ctx, "cancel-task", target, rendererTestPolicy("example.com", "93.184.216.34"), deps)
	require.NoError(t, err)
	_, _, fetchErr := fetcher.fetch(ctx, "cancel-request", "", target, crawleregress.OriginDocument, http.MethodGet, nil)
	require.ErrorIs(t, fetchErr, context.Canceled)
	require.False(t, errors.Is(fetchErr, io.ErrUnexpectedEOF))
	captured := holder.Snapshot()
	require.NotNil(t, captured)
	require.Equal(t, "read", captured.FailurePhase)
	require.Contains(t, captured.ReadError, "context canceled")
	require.False(t, captured.DecodedBodyComplete)
	require.Equal(t, prefix, captured.DecodedBody)
	require.EqualValues(t, 1, hits.Load())
}

// rendererCancelAfterPrefixConn cancels on the first read after the complete synthetic
// prefix was received, proving cancellation happens during body reading, not header admission.
type rendererCancelAfterPrefixConn struct {
	net.Conn
	prefix   []byte
	observed []byte
	cancel   context.CancelFunc
}

// Read preserves actual socket bytes and cancels before the next otherwise blocking body read.
func (c *rendererCancelAfterPrefixConn) Read(buffer []byte) (int, error) {
	if bytes.Contains(c.observed, c.prefix) {
		c.cancel()
	}
	n, err := c.Conn.Read(buffer)
	c.observed = append(c.observed, buffer[:n]...)
	return n, err
}

// TestRendererFailureObservationFirstAndIndependent preserves the first failed exchange only.
func TestRendererFailureObservationFirstAndIndependent(t *testing.T) {
	ctx, holder := withRendererFailureObservation(context.Background())
	first := newRendererFailureFixture("first")
	observeRendererFailure(ctx, first)
	first.URL = "changed source URL"
	first.DecodedBody[0] = 'X'
	observeRendererFailure(ctx, newRendererFailureFixture("second"))
	require.Equal(t, "first", holder.Snapshot().RequestID)
	require.Equal(t, "http://example.com/input", holder.Snapshot().URL)
	require.Equal(t, []byte("partial"), holder.Snapshot().DecodedBody)
}

// newRendererFailureFixture returns synthetic, credential-free selected response facts.
func newRendererFailureFixture(id string) *rendererFailureResponse {
	return &rendererFailureResponse{
		RequestID: id, Kind: crawleregress.OriginDocument, Method: http.MethodGet,
		URL:            "http://example.com/input",
		UpstreamStatus: http.StatusOK, DeclaredContentLength: 100, FailurePhase: "read",
		DecodedBody: []byte("partial"), DecodedBodyObserved: true, ReadError: "unexpected EOF",
	}
}

// requireRendererFailureLogFields verifies the real failed HTTP observation passes
// intact through the ordinary Zap JSON core, without archive schemas or private files.
func requireRendererFailureLogFields(t *testing.T, target string, captured *rendererFailureResponse) {
	t.Helper()
	logged := crawlerFieldsThroughJSONCore(t, captured.fields())
	require.Equal(t, target, logged["renderer_request_url"])
	require.Equal(t, captured.DecodedBodyObserved, logged["renderer_decoded_body_observed"])
	if captured.DecodedBodyObserved {
		require.Equal(t, string(captured.DecodedBody), logged["renderer_decoded_body"])
		require.EqualValues(t, len(captured.DecodedBody), logged["renderer_decoded_body_bytes"])
	}
	require.Equal(t, captured.FailurePhase, logged["renderer_failure_phase"])
	require.Equal(t, captured.ReadError, logged["renderer_read_error"])
	require.Equal(t, captured.DecodeError, logged["renderer_decode_error"])
	require.Equal(t, captured.DecodedBodyComplete, logged["renderer_decoded_body_complete"])
	raw, err := json.Marshal(logged)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "transport-secret-canary")
	require.NotContains(t, string(raw), "transport-cookie-canary")
}
