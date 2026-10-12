package ssosession

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestStrictSessionHeaderAndFieldAmbiguities excludes unsupported JWT headers
// and duplicate/case-aliased required fields before online authority calls.
func TestStrictSessionHeaderAndFieldAmbiguities(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	valid := testSignedSession(t, private, testClaims(time.Now().UTC()), "EdDSA")
	parts := strings.Split(valid, ".")
	rawPayload, err := base64.RawURLEncoding.DecodeString(parts[1])
	require.NoError(t, err)
	for _, tc := range []struct{ name, header, payload string }{
		{"wrong_typ", `{"alg":"EdDSA","typ":"model"}`, string(rawPayload)},
		{"null_typ", `{"alg":"EdDSA","typ":null}`, string(rawPayload)},
		{"critical_header", `{"alg":"EdDSA","crit":[]}`, string(rawPayload)},
		{"unencoded_payload", `{"alg":"EdDSA","b64":false}`, string(rawPayload)},
		{"string_b64", `{"alg":"EdDSA","b64":"true"}`, string(rawPayload)},
		{"duplicate_algorithm", `{"alg":"HS256","alg":"EdDSA"}`, string(rawPayload)},
		{"uppercase_only_algorithm", `{"ALG":"EdDSA"}`, string(rawPayload)},
		{"duplicate_subject", `{"alg":"EdDSA"}`, strings.TrimSuffix(string(rawPayload), "}") + `,"sub":"` + testAccount + `"}`},
		{"uppercase_only_issuer", `{"alg":"EdDSA"}`, strings.Replace(string(rawPayload), `"iss":`, `"ISS":`, 1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			token := base64.RawURLEncoding.EncodeToString([]byte(tc.header)) + "." + base64.RawURLEncoding.EncodeToString([]byte(tc.payload)) + "." + parts[2]
			authority := &strictTestAuthority{public: public}
			_, err := NewStrictVerifier(authority).Verify(t.Context(), token)
			require.ErrorIs(t, err, ErrSessionInvalid)
			require.Zero(t, authority.calls)
		})
	}
}

// TestStrictEnvelopeParsingDoesNotChangeLegacyCV preserves the established CV
// parser while private sessions reject duplicate or aliased identity responses.
func TestStrictEnvelopeParsingDoesNotChangeLegacyCV(t *testing.T) {
	_, private, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	token := testSignedSession(t, private, testClaims(time.Now().UTC()), "EdDSA")
	for _, tc := range []struct {
		name, body    string
		legacyAccepts bool
	}{
		{"duplicate_id", `{"data":{"WhoAmI":{"id":"` + testAccount + `","id":"` + testAccount + `"}}}`, true},
		{"aliased_id", `{"data":{"WhoAmI":{"ID":"` + testAccount + `"}}}`, true},
		{"aliased_data", `{"DATA":{"WhoAmI":{"id":"` + testAccount + `"}}}`, true},
		{"noncanonical_account", `{"data":{"WhoAmI":{"id":"` + strings.ReplaceAll(testAccount, "-", "") + `"}}}`, true},
		{"nil_account", `{"data":{"WhoAmI":{"id":"00000000-0000-0000-0000-000000000000"}}}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := NewVerifier(DefaultEndpoint, DefaultTimeout)
			base.client.Transport = whoAmITestTransport(func(req *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(tc.body)), Request: req}, nil
			})
			legacy, err := base.Verify(t.Context(), token)
			if tc.legacyAccepts {
				require.NoError(t, err)
				require.NotNil(t, legacy)
			}
			_, err = NewStrictVerifier(base).Verify(t.Context(), token)
			require.ErrorIs(t, err, ErrUnavailable)
			require.False(t, base.strict, "strict wrapping must never mutate CV behavior")
		})
	}
}
