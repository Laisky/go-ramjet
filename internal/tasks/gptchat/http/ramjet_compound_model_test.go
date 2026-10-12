package http

import (
	"encoding/base64"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

// modelPolicyJWT builds only recognizable inert issuer fixtures, not sessions.
func modelPolicyJWT(issuer string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"EdDSA","typ":"JWT"}`)) + "." +
		base64.RawURLEncoding.EncodeToString([]byte(`{"iss":"`+issuer+`"}`)) + ".SYNTHETIC_CASE_PRESERVED_SIGNATURE"
}

// TestRamjetCompoundModelCredentialsRejectWithoutSession enforces the explicit
// session/provider separation policy without claiming production disclosure.
func TestRamjetCompoundModelCredentialsRejectWithoutSession(t *testing.T) {
	session, provider := modelPolicyJWT("laisky-sso"), modelPolicyJWT("model-provider")
	for _, key := range []string{
		"sk-SYNTHETIC:" + provider + "|" + session,
		"sk-SYNTHETIC:" + session + "." + provider,
	} {
		t.Run(key[:15], func(t *testing.T) {
			byokAuditSetup(t)
			ctx := newAuthContext(key)
			require.Empty(t, ctx.Request.Header.Get(ramjetSessionHeader))
			_, err := resolveRamjetUser(ctx)
			require.Error(t, err)
			if err != nil {
				require.NotContains(t, err.Error(), session)
			}
		})
	}
}

// TestRamjetCompoundModelURLsRejectWithoutSession exercises actual model-option
// admission with no separate header, preserving authority case and URL syntax.
func TestRamjetCompoundModelURLsRejectWithoutSession(t *testing.T) {
	session, provider := modelPolicyJWT("laisky-sso"), modelPolicyJWT("model-provider")
	compound := provider + "|" + session
	for _, target := range []struct{ name, value string }{
		{"query", "http://100.64.0.10:3000/provider?tokens=" + url.QueryEscape(compound)},
		{"query_name", "http://100.64.0.10:3000/provider?" + url.QueryEscape(compound) + "=value"},
		{"userinfo", "http://" + url.User(compound).String() + "@100.64.0.10:3000/provider"},
		{"path", "http://100.64.0.10:3000/provider/" + compound},
		{"fragment", "http://100.64.0.10:3000/provider#" + compound},
		{"hostname", "http://" + session + ".Provider.Example/provider"},
		{"authority_port", "http://" + session + ".Provider.Example:3000/provider"},
	} {
		t.Run(target.name, func(t *testing.T) {
			byokAuditSetup(t)
			ctx := newAuthContext("sk-SYNTHETIC-MODEL-KEY")
			require.Empty(t, ctx.Request.Header.Get(ramjetSessionHeader))
			ctx.Request.Header.Set("X-Laisky-Api-Base", target.value)
			require.Error(t, validateRamjetAPIBase(target.value))
			_, err := resolveRamjetUser(ctx)
			require.Error(t, err)
			if err != nil {
				require.NotContains(t, err.Error(), session)
			}
		})
	}
}

// TestRamjetCompoundProviderOptionsRemainSupported excludes restrictions on
// unrelated JWTs, opaque keys, configured destinations and authority case.
func TestRamjetCompoundProviderOptionsRemainSupported(t *testing.T) {
	provider := modelPolicyJWT("model-provider")
	key := "sk-SYNTHETIC:" + provider + "|" + provider
	for _, base := range []string{
		"http://" + provider + ".Provider.Example:3000/provider",
		"http://100.64.0.10:3000/provider?tokens=" + url.QueryEscape(provider+"|"+provider),
		"http://MODEL_USER:OPAQUE_PASS@Internal.Example:8080/provider#opaque",
	} {
		byokAuditSetup(t)
		ctx := newAuthContext(key)
		ctx.Request.Header.Set("X-Laisky-Api-Base", base)
		resolved, err := resolveRamjetUser(ctx)
		require.NoError(t, err)
		require.Equal(t, key, resolved.OpenaiToken)
		require.Equal(t, base, resolved.APIBase)
	}
}
