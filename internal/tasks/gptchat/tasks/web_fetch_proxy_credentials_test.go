package tasks

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestVerifiedProviderHTTPCredentialsRequireTLS exercises the real adapters to
// reject credentialed cleartext endpoints before DNS or network activity while
// retaining explicitly configured anonymous HTTP providers.
func TestVerifiedProviderHTTPCredentialsRequireTLS(t *testing.T) {
	for _, fixture := range providerHTTPFixtures() {
		t.Run(fixture.name, func(t *testing.T) {
			var requests, resolutions, dials atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				requests.Add(1)
				require.Equal(t, fixture.authorization, request.Header.Get("Authorization"))
				require.Equal(t, fixture.apiToken, request.Header.Get("X-Api-Token"))
				_, err := io.WriteString(w, fixture.response)
				require.NoError(t, err)
			}))
			t.Cleanup(server.Close)
			deps := rendererTestDependencies(t, server, "93.184.216.34", &dials)
			deps.resolve = func(context.Context, string) ([]net.IPAddr, error) {
				resolutions.Add(1)
				return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
			}
			ctx := withVerifiedWebFetchClient(t.Context(), deps)
			body, err := fixture.provider("http://example.com").Fetch(ctx, "https://example.org/document")
			if fixture.authorization != "" || fixture.apiToken != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), "provider credentials require HTTPS")
				require.NotContains(t, err.Error(), "provider-token-only")
				require.Empty(t, body)
				require.Zero(t, resolutions.Load())
				require.Zero(t, dials.Load())
				require.Zero(t, requests.Load())
				return
			}
			require.NoError(t, err)
			require.Equal(t, "provider-content", body)
			require.Positive(t, resolutions.Load())
			require.EqualValues(t, 1, dials.Load())
			require.EqualValues(t, 1, requests.Load())
		})
	}
}

// TestLegacyProviderHTTPCredentials preserves unscoped configured providers'
// existing HTTP behavior while the verified caller uses the stricter transport.
func TestLegacyProviderHTTPCredentials(t *testing.T) {
	for _, fixture := range providerHTTPFixtures() {
		t.Run(fixture.name, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				requests.Add(1)
				require.Equal(t, fixture.authorization, request.Header.Get("Authorization"))
				require.Equal(t, fixture.apiToken, request.Header.Get("X-Api-Token"))
				_, err := io.WriteString(w, fixture.response)
				require.NoError(t, err)
			}))
			t.Cleanup(server.Close)
			body, err := fixture.provider(server.URL).Fetch(t.Context(), "https://example.org/document")
			require.NoError(t, err)
			require.Equal(t, "provider-content", body)
			require.EqualValues(t, 1, requests.Load())
		})
	}
}
