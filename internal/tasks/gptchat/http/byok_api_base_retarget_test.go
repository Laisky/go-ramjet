package http

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// buildBYOKUpstreamRequest authenticates key the way a chat request does,
// with apiBase as X-Laisky-Api-Base when it is not empty, and returns the
// upstream Responses request the server would send for it. It builds the
// request without sending it, so the assertion does not depend on which
// HTTP client delivers caller-selected upstreams.
func buildBYOKUpstreamRequest(t *testing.T, key, apiBase string) (host, path, auth string) {
	t.Helper()
	ctx := newAuthContext(key)
	if apiBase != "" {
		ctx.Request.Header.Set("X-Laisky-Api-Base", apiBase)
	}
	user, err := getUserByAuthHeader(ctx)
	require.NoError(t, err)
	require.True(t, user.BYOK)

	req, err := buildResponsesHTTPRequestCtx(context.Background(), UpstreamDeps{
		User:          user,
		RequestHeader: ctx.Request.Header,
	}, []byte(`{"model":"test-model"}`))
	require.NoError(t, err)
	return req.URL.Host, req.URL.Path, req.Header.Get("Authorization")
}

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

	host, path, auth := buildBYOKUpstreamRequest(t, key, "https://evil.example")
	require.Equal(t, "evil.example", host)
	require.Equal(t, "/v1/responses", path)
	require.Equal(t, "Bearer "+key, auth)
}

// TestBYOKWithoutApiBaseHeaderUsesDefaultUpstream pins the other half: a
// BYOK request without X-Laisky-Api-Base keeps the server's default
// upstream, so a client that withholds an unconfirmed API base keeps the key
// on the endpoint it was stored for.
func TestBYOKWithoutApiBaseHeaderUsesDefaultUpstream(t *testing.T) {
	byokAuditSetup(t)
	const key = "sk-SYNTHETIC-ONLY-DEFAULT-0123456789"

	host, path, auth := buildBYOKUpstreamRequest(t, key, "")
	require.Equal(t, "oneapi.laisky.com", host)
	require.Equal(t, "/v1/responses", path)
	require.Equal(t, "Bearer "+key, auth)
}
