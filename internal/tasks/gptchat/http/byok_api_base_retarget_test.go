package http

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestBYOKApiBaseHeaderRetargetsUpstreamCredential pins the server half of
// the link-supplied API base chain: for a BYOK key the server trusts the
// caller's X-Laisky-Api-Base and sends that key, as the upstream
// Authorization header, to whatever host the header names. The web client is
// therefore the only place that can keep a link from re-targeting a stored
// key, and it must never send a link-supplied API base the user has not
// confirmed (see use-config-url-endpoint.test.ts).
func TestBYOKApiBaseHeaderRetargetsUpstreamCredential(t *testing.T) {
	byokAuditSetup(t)
	const key = "sk-SYNTHETIC-ONLY-RETARGET-0123456789"
	const attackerBase = "https://evil.example"

	ctx := newAuthContext(key)
	ctx.Request.Header.Set("X-Laisky-Api-Base", attackerBase)
	user, err := getUserByAuthHeader(ctx)
	require.NoError(t, err)
	require.True(t, user.BYOK)

	var upstreamHost, upstreamPath, upstreamAuth string
	httpcli = &http.Client{Transport: byokAuditTransport(func(req *http.Request) (*http.Response, error) {
		upstreamHost = req.URL.Host
		upstreamPath = req.URL.Path
		upstreamAuth = req.Header.Get("Authorization")
		return byokAuditResponse(req, `{"id":"resp_synthetic","output":[]}`), nil
	})}

	_, _, err = callUpstreamResponses(ctx, user, &OpenAIResponsesReq{Model: "test-model"})
	require.NoError(t, err)
	require.Equal(t, "evil.example", upstreamHost)
	require.Equal(t, "/v1/responses", upstreamPath)
	require.Equal(t, "Bearer "+key, upstreamAuth)
}

// TestBYOKWithoutApiBaseHeaderUsesDefaultUpstream pins the other half: a
// BYOK request without X-Laisky-Api-Base keeps the server's default
// upstream, so a client that withholds an unconfirmed API base keeps the key
// on the endpoint it was stored for.
func TestBYOKWithoutApiBaseHeaderUsesDefaultUpstream(t *testing.T) {
	byokAuditSetup(t)
	const key = "sk-SYNTHETIC-ONLY-DEFAULT-0123456789"

	ctx := newAuthContext(key)
	user, err := getUserByAuthHeader(ctx)
	require.NoError(t, err)

	var upstreamHost string
	httpcli = &http.Client{Transport: byokAuditTransport(func(req *http.Request) (*http.Response, error) {
		upstreamHost = req.URL.Host
		require.True(t, strings.HasPrefix(req.Header.Get("Authorization"), "Bearer "))
		_, rerr := io.Copy(io.Discard, req.Body)
		require.NoError(t, rerr)
		return byokAuditResponse(req, `{"id":"resp_synthetic","output":[]}`), nil
	})}

	_, _, err = callUpstreamResponses(ctx, user, &OpenAIResponsesReq{Model: "test-model"})
	require.NoError(t, err)
	require.Equal(t, "oneapi.laisky.com", upstreamHost)
}
