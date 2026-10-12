package ssosession

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestHasLaiskyIssuerCompound retains bounded rejection controls for explicit
// model-option policy; it makes no signature, account or network-disclosure claim.
func TestHasLaiskyIssuerCompound(t *testing.T) {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"EdDSA","typ":"JWT"}`))
	session := header + "." + base64.RawURLEncoding.EncodeToString([]byte(`{"iss":"laisky-sso"}`)) + ".SYNTHETIC_CASE_PRESERVED_SIGNATURE"
	provider := header + "." + base64.RawURLEncoding.EncodeToString([]byte(`{"iss":"model-provider"}`)) + ".SYNTHETIC_PROVIDER_SIGNATURE"
	for _, tc := range []struct {
		name, value string
		reject      bool
	}{
		{"whole_session", session, true},
		{"multiple_jwts", provider + "," + session, true},
		{"session_first", session + "," + provider, true},
		{"dot_joined", provider + "." + session, true},
		{"model_key_compound", "sk-SYNTHETIC:" + provider + "|" + session, true},
		{"bearer_wrapped", "authorization=Bearer " + session + ";other=value", true},
		{"host_authority", session + ".Provider.Example:443", true},
		{"long_opaque_prefix", strings.Repeat("x", MaxTokenLength+1) + ":" + session, true},
		{"provider_only", provider, false},
		{"compound_provider", "sk-SYNTHETIC:" + provider + "|" + provider, false},
		{"opaque_key", "sk-SYNTHETIC-OPAQUE-MODEL-KEY", false},
		{"opaque_url", "https://Internal.Example:8080/provider?flag=", false},
		{"malformed_payload", header + ".not-json.SYNTHETIC", false},
		{"no_compact_token", "model-option-laisky-sso", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.reject, HasLaiskyIssuer(tc.value))
		})
	}
}
