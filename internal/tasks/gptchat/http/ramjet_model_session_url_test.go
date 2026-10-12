package http

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRamjetModelURLRejectsOriginalAccountSessions protects only URL routing
// components and never dispatches identifiable Laisky credentials to a provider.
func TestRamjetModelURLRejectsOriginalAccountSessions(t *testing.T) {
	for _, component := range []string{"query", "query_name", "username", "password", "path", "fragment", "bearer_query"} {
		t.Run(component, func(t *testing.T) {
			byokAuditSetup(t)
			token, authority := ramjetSessionTestSetup(t, ramjetTestAccountA)
			target := &url.URL{Scheme: "http", Host: "100.64.0.10:3000", Path: "/provider"}
			switch component {
			case "query":
				target.RawQuery = url.Values{"access_token": []string{token}}.Encode()
			case "query_name":
				target.RawQuery = url.Values{token: []string{"value"}}.Encode()
			case "username":
				target.User = url.User(token)
			case "password":
				target.User = url.UserPassword("model-user", token)
			case "path":
				target.Path = "/provider/" + token
			case "fragment":
				target.Fragment = token
			case "bearer_query":
				target.RawQuery = url.Values{"authorization": []string{"Bearer " + token}}.Encode()
			}
			ctx := newAuthContext("sk-SYNTHETIC-MODEL-KEY")
			ctx.Request.Method = http.MethodPost
			ctx.Request.URL.Path = "/gptchat/ramjet/gptchat/query/chunks"
			ctx.Request.Body = http.NoBody
			ctx.Request.Header.Set("X-Laisky-Api-Base", target.String())
			ctx.Request.Header.Set(ramjetSessionHeader, "Bearer "+token)
			calls := 0
			httpcli = &http.Client{Transport: byokAuditTransport(func(req *http.Request) (*http.Response, error) { calls++; return byokAuditResponse(req, "{}"), nil })}
			RamjetProxyHandler(ctx)
			require.GreaterOrEqual(t, ctx.Writer.Status(), http.StatusBadRequest)
			require.Zero(t, calls)
			require.Zero(t, authority.calls)
			require.Error(t, validateRamjetAPIBase(target.String()))
		})
	}
}

// TestRamjetModelURLPreservesUnrelatedProviderCredentials excludes false-positive
// restrictions on internal HTTP destinations, provider JWTs and query syntax.
func TestRamjetModelURLPreservesUnrelatedProviderCredentials(t *testing.T) {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload, err := json.Marshal(map[string]string{"iss": "model-provider", "sub": "synthetic"})
	require.NoError(t, err)
	providerJWT := header + "." + base64.RawURLEncoding.EncodeToString(payload) + ".SYNTHETIC_SIGNATURE"
	for _, target := range []string{
		"http://100.64.0.10:3000/provider?access_token=" + providerJWT + "&flag=",
		"http://" + providerJWT + "@100.64.0.10:3000/provider",
		"http://model-user:" + providerJWT + "@100.64.0.10:3000/provider",
		"http://100.64.0.10:3000/provider/" + providerJWT,
		"http://100.64.0.10:3000/provider#" + providerJWT,
	} {
		t.Run(target, func(t *testing.T) {
			require.NoError(t, validateRamjetAPIBase(target))
			byokAuditSetup(t)
			ctx := newAuthContext("sk-SYNTHETIC-MODEL-KEY")
			ctx.Request.Header.Set("X-Laisky-Api-Base", target)
			resolved, err := resolveRamjetUser(ctx)
			require.NoError(t, err)
			require.Equal(t, target, resolved.APIBase)
		})
	}
}

// TestRamjetModelURLRejectsExactSeparateSession rejects an explicitly supplied
// original token in URL components even when it is opaque to the issuer heuristic.
func TestRamjetModelURLRejectsExactSeparateSession(t *testing.T) {
	byokAuditSetup(t)
	original := "SYNTHETIC_OPAQUE_ACCOUNT_SESSION"
	ctx := newAuthContext("sk-SYNTHETIC-MODEL-KEY")
	ctx.Request.Header.Set(ramjetSessionHeader, "Bearer "+original)
	ctx.Request.Header.Set("X-Laisky-Api-Base", "http://provider.fixture?access_token="+original)
	_, err := resolveRamjetUser(ctx)
	require.Error(t, err)
	require.NotContains(t, err.Error(), original)
	require.False(t, strings.Contains(err.Error(), "provider.fixture"))
}
