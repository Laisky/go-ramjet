package localstorage

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
)

// closeTrackingConn records when the client side of a connection is closed.
type closeTrackingConn struct {
	net.Conn
	once   sync.Once
	closed *atomic.Int32
}

// Close records the first close and closes the connection.
func (c *closeTrackingConn) Close() error {
	c.once.Do(func() { c.closed.Add(1) })
	return errors.WithStack(c.Conn.Close())
}

// TestOpenArchivedObjectReleasesConnectionOnArchiveMiss guards against leaving an
// archive miss (or any other non-200 answer) unclosed: the old lookup returned
// an error without closing the body, so every uncached proxy request kept an
// S3 connection and its transport goroutines busy until the client timeout.
func TestOpenArchivedObjectReleasesConnectionOnArchiveMiss(t *testing.T) {
	archive := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, err := io.WriteString(w, "<Error><Code>NoSuchKey</Code></Error>")
		require.NoError(t, err)
	}))
	t.Cleanup(archive.Close)

	var dialed, closed atomic.Int32
	client := &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			var dialer net.Dialer
			conn, err := dialer.DialContext(ctx, network, address)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			dialed.Add(1)
			return &closeTrackingConn{Conn: conn, closed: &closed}, nil
		},
	}}
	t.Cleanup(client.CloseIdleConnections)

	body, err := openArchivedObject(context.Background(), client, archive.URL+"/bucket/arweave/urlcache/ab/cd/abcd")
	require.Error(t, err)
	require.Contains(t, err.Error(), "unexpected status code 404")
	require.Nil(t, body)

	// A released response leaves its connection idle or closed, and closing idle
	// connections then closes it; an unreleased response keeps it busy and open.
	client.CloseIdleConnections()
	require.Eventually(t, func() bool { return closed.Load() == dialed.Load() }, 2*time.Second, 5*time.Millisecond,
		"the archive connection must be released after a non-200 answer")
}
