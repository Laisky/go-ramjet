package http

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/library/egress"
)

// countingConn counts the bytes read from its connection.
type countingConn struct {
	net.Conn
	read *atomic.Int64
}

// Read reads from the connection and adds the bytes read to the count.
func (c *countingConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	c.read.Add(int64(n))
	return n, err
}

// countEgressReads makes caller-selected egress count every byte it reads off
// the wire into read.
func countEgressReads(read *atomic.Int64) {
	dial := tenantEgressDeps.Dial
	tenantEgressDeps.Dial = func(ctx context.Context, network, address string) (net.Conn, error) {
		conn, err := dial(ctx, network, address)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		return &countingConn{Conn: conn, read: read}, nil
	}
}

// TestCallerSelectedErrorBodiesAreBounded guards against a caller-selected
// upstream that answers an error with an endless plain body: the main chat
// path and transcription read the whole error body, so without a bound one
// request could hold gigabytes in memory for the 30-minute exchange budget.
// Each must stop reading after the 1 MiB error bound and fail.
func TestCallerSelectedErrorBodiesAreBounded(t *testing.T) {
	const served = 16 << 20
	const tolerance = 1 << 20
	calls := map[string]func(ctx context.Context, base string) error{
		"transcription": func(ctx context.Context, base string) error {
			_, err := Transcript(ctx, tenantSelectedUser(base),
				&TranscriptRequest{File: nopFile{bytes.NewReader([]byte("RIFF"))}, Model: "whisper-1"})
			return err
		},
	}
	for name, call := range responsesCalls() {
		calls["responses "+name] = func(ctx context.Context, base string) error {
			return call(ctx, tenantSelectedUser(base))
		}
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			var written atomic.Int64
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/plain")
				w.WriteHeader(http.StatusBadRequest)
				chunk := bytes.Repeat([]byte("e"), 64<<10)
				for written.Load() < served {
					n, err := w.Write(chunk)
					written.Add(int64(n))
					if err != nil {
						return
					}
				}
			}))
			t.Cleanup(provider.Close)
			egressTestSetup(t, provider)
			var read atomic.Int64
			countEgressReads(&read)

			err := call(t.Context(), "http://"+egressProviderHost)
			require.Error(t, err)
			require.Less(t, read.Load(), int64(egress.DefaultMaxDecodedErrorBytes+tolerance),
				"an error body must not be read past its bound")
			require.Less(t, len(err.Error()), egress.DefaultMaxDecodedErrorBytes+tolerance)
		})
	}
}
