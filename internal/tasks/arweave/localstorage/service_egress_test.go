package localstorage

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/library/egress"
)

// TestLoadContentByUrlRefusesNonPublicDestinations pins the request-forgery
// boundary at the fetch itself, the sink code scanning reports, rather than at
// CacheHandler's earlier admission: any caller that passes a user-selected URL
// is refused before a connection is made to a non-public destination.
func TestLoadContentByUrlRefusesNonPublicDestinations(t *testing.T) {
	var contacted atomic.Int32
	private := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		contacted.Add(1)
		_, err := io.WriteString(w, "private-content")
		require.NoError(t, err)
	}))
	t.Cleanup(private.Close)
	_, port, err := net.SplitHostPort(private.Listener.Addr().String())
	require.NoError(t, err)

	for _, raw := range []string{
		private.URL + "/private-path?token=private-query",
		"http://localhost:" + port + "/metadata",
		"http://169.254.169.254/latest/meta-data/",
		"http://[::1]:" + port + "/",
		"http://0x7f000001:" + port + "/",
	} {
		t.Run(raw, func(t *testing.T) {
			body, err := LoadContentByUrl(context.Background(), raw)
			require.Error(t, err)
			require.Nil(t, body)
			reason, refused := egress.ReasonOf(err)
			require.True(t, refused, "refusal must carry an admission reason: %v", err)
			require.True(t, reason.Expected(), "reason %q", reason)
			require.Zero(t, contacted.Load(), "non-public destination must never be contacted")
		})
	}
}

// TestLoadContentByUrlRefusesPrivateDNSAnswer refuses a public-looking hostname
// whose DNS answer is private, so a rebinding name cannot reach internal hosts.
func TestLoadContentByUrlRefusesPrivateDNSAnswer(t *testing.T) {
	var dialed atomic.Int32
	deps := egress.Dependencies{
		Resolve: func(context.Context, string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP("10.0.0.7")}}, nil
		},
		Dial: func(context.Context, string, string) (net.Conn, error) {
			dialed.Add(1)
			return nil, net.ErrClosed
		},
	}
	requireNoProxyGoroutines(t)
	oldClient := targetHTTPClient
	targetHTTPClient = egress.NewHTTPClient(5*time.Second, deps)
	t.Cleanup(func() {
		requireNoProxyGoroutines(t)
		targetHTTPClient = oldClient
	})

	body, err := LoadContentByUrl(context.Background(), "http://rebind.example/internal")
	require.Error(t, err)
	require.Nil(t, body)
	reason, refused := egress.ReasonOf(err)
	require.True(t, refused, "%v", err)
	require.Equal(t, egress.ReasonDestinationDenied, reason)
	require.Zero(t, dialed.Load(), "a private DNS answer must never be dialed")
}
