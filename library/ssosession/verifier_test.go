package ssosession

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
)

const testAccount = "0199b2a4-0000-7000-8000-00000000c0de"

// testSignedSession signs only local fixture claims with an ephemeral key.
func testSignedSession(t *testing.T, private ed25519.PrivateKey, claims map[string]any, algorithm string) string {
	t.Helper()
	header, err := json.Marshal(map[string]string{"alg": algorithm, "typ": "JWT"})
	require.NoError(t, err)
	payload, err := json.Marshal(claims)
	require.NoError(t, err)
	unsigned := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(ed25519.Sign(private, []byte(unsigned)))
}

// testClaims builds an inert account fixture valid around the requested time.
func testClaims(now time.Time) map[string]any {
	return map[string]any{"iss": "laisky-sso", "sub": testAccount, "uid": testAccount,
		"iat": now.Add(-time.Minute).Unix(), "exp": now.Add(time.Hour).Unix(),
		"jti": "0199b2a4-0000-7000-8000-00000000a111", "username": "synthetic@example.test", "display_name": "Synthetic"}
}

// strictTestAuthority implements only the independent signature/account check.
type strictTestAuthority struct {
	public      ed25519.PublicKey
	calls       int
	inactive    bool
	unavailable bool
	override    string
}

// Verify rejects foreign signatures and inactive fixtures without network access.
func (a *strictTestAuthority) Verify(_ context.Context, token string) (*Identity, error) {
	a.calls++
	if a.unavailable {
		return nil, ErrUnavailable
	}
	if a.inactive {
		return nil, ErrSessionInvalid
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, ErrSessionInvalid
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !ed25519.Verify(a.public, []byte(parts[0]+"."+parts[1]), signature) {
		return nil, ErrSessionInvalid
	}
	uid, err := StrictClaimedUID(token, time.Now().UTC())
	if err != nil {
		return nil, errors.Wrap(err, "fixture authority")
	}
	if a.override != "" {
		uid = a.override
	}
	return &Identity{UID: uid, Username: "synthetic@example.test"}, nil
}

// TestStrictSessionClaimsRejectBeforeAuthority excludes unsupported session
// shapes without changing CV's legacy claim parser or making authority calls.
func TestStrictSessionClaimsRejectBeforeAuthority(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	for _, tc := range []struct {
		name      string
		algorithm string
		change    func(map[string]any)
	}{
		{"valid", "EdDSA", func(map[string]any) {}},
		{"wrong_algorithm", "HS256", func(map[string]any) {}},
		{"foreign_issuer", "EdDSA", func(c map[string]any) { c["iss"] = "foreign" }},
		{"missing_expiry", "EdDSA", func(c map[string]any) { delete(c, "exp") }},
		{"null_expiry", "EdDSA", func(c map[string]any) { c["exp"] = nil }},
		{"expired", "EdDSA", func(c map[string]any) { c["exp"] = time.Now().Add(-time.Hour).Unix() }},
		{"string_expiry", "EdDSA", func(c map[string]any) { c["exp"] = "9999999999" }},
		{"boolean_expiry", "EdDSA", func(c map[string]any) { c["exp"] = true }},
		{"missing_issued_at", "EdDSA", func(c map[string]any) { delete(c, "iat") }},
		{"future_issued_at", "EdDSA", func(c map[string]any) { c["iat"] = time.Now().Add(2 * time.Hour).Unix() }},
		{"issued_after_expiry", "EdDSA", func(c map[string]any) { c["iat"] = time.Now().Add(time.Hour).Unix() }},
		{"boolean_issued_at", "EdDSA", func(c map[string]any) { c["iat"] = false }},
		{"subject_mismatch", "EdDSA", func(c map[string]any) { c["sub"] = "0199b2a4-0000-7000-8000-0000000f0e1d" }},
		{"missing_subject", "EdDSA", func(c map[string]any) { delete(c, "sub") }},
		{"valid_uppercase", "EdDSA", func(c map[string]any) { c["uid"] = strings.ToUpper(testAccount); c["sub"] = c["uid"] }},
		{"noncanonical_compact", "EdDSA", func(c map[string]any) { c["uid"] = strings.ReplaceAll(testAccount, "-", ""); c["sub"] = c["uid"] }},
		{"invalid_uid", "EdDSA", func(c map[string]any) { c["uid"] = "synthetic-not-uuid"; c["sub"] = c["uid"] }},
		{"nil_uid", "EdDSA", func(c map[string]any) { c["uid"] = "00000000-0000-0000-0000-000000000000"; c["sub"] = c["uid"] }},
		{"missing_jti", "EdDSA", func(c map[string]any) { delete(c, "jti") }},
		{"empty_jti", "EdDSA", func(c map[string]any) { c["jti"] = "" }},
		{"missing_username", "EdDSA", func(c map[string]any) { delete(c, "username") }},
		{"invalid_display_name", "EdDSA", func(c map[string]any) { c["display_name"] = 42 }},
		{"valid_clock_skew", "EdDSA", func(c map[string]any) { c["iat"] = time.Now().Add(30 * time.Second).Unix() }},
		{"valid_empty_display", "EdDSA", func(c map[string]any) { c["display_name"] = "" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			claims := testClaims(time.Now().UTC())
			tc.change(claims)
			token := testSignedSession(t, private, claims, tc.algorithm)
			authority := &strictTestAuthority{public: public}
			identity, err := NewStrictVerifier(authority).Verify(t.Context(), token)
			if strings.HasPrefix(tc.name, "valid") {
				require.NoError(t, err)
				require.Equal(t, testAccount, identity.UID)
				require.Equal(t, 1, authority.calls)
			} else {
				require.ErrorIs(t, err, ErrSessionInvalid)
				require.Nil(t, identity)
				require.Zero(t, authority.calls)
				require.NotContains(t, err.Error(), token)
			}
		})
	}
}

// TestStrictSessionMalformedEncodingRejects excludes invalid signature encodings,
// oversized values and non-finite expiry representations without authorization.
func TestStrictSessionMalformedEncodingRejects(t *testing.T) {
	_, private, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	valid := testSignedSession(t, private, testClaims(time.Now().UTC()), "EdDSA")
	parts := strings.Split(valid, ".")
	overflow := base64.RawURLEncoding.EncodeToString([]byte(`{"iss":"laisky-sso","sub":"` + testAccount + `","uid":"` + testAccount + `","iat":1,"exp":1e9999}`))
	for _, tc := range []struct{ name, token string }{
		{"empty", ""}, {"not_jwt", "synthetic"}, {"oversized", strings.Repeat("a", MaxTokenLength+1)},
		{"short_signature", parts[0] + "." + parts[1] + ".YQ"},
		{"invalid_signature", parts[0] + "." + parts[1] + ".%"},
		{"invalid_header", "%." + parts[1] + "." + parts[2]},
		{"invalid_payload", parts[0] + ".%." + parts[2]},
		{"overflow_expiry", parts[0] + "." + overflow + "." + parts[2]},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := StrictClaimedUID(tc.token, time.Now().UTC())
			require.ErrorIs(t, err, ErrSessionInvalid)
		})
	}
}

// TestStrictSessionAuthorityBinding requires independent active-account proof
// even when the submitted claims look correct.
func TestStrictSessionAuthorityBinding(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	token := testSignedSession(t, private, testClaims(time.Now().UTC()), "EdDSA")
	otherPublic, otherPrivate, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	require.NotEqual(t, public, otherPublic)
	foreign := testSignedSession(t, otherPrivate, testClaims(time.Now().UTC()), "EdDSA")
	for _, tc := range []struct {
		name, token           string
		inactive, unavailable bool
		override              string
		expected              error
	}{
		{"active", token, false, false, "", nil},
		{"inactive", token, true, false, "", ErrSessionInvalid},
		{"foreign_signer", foreign, false, false, "", ErrSessionInvalid},
		{"wrong_account", token, false, false, "0199b2a4-0000-7000-8000-0000000f0e1d", ErrSessionInvalid},
		{"unavailable", token, false, true, "", ErrUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			authority := &strictTestAuthority{public: public, inactive: tc.inactive, unavailable: tc.unavailable, override: tc.override}
			identity, err := NewStrictVerifier(authority).Verify(t.Context(), tc.token)
			if tc.expected != nil {
				require.ErrorIs(t, err, tc.expected)
				require.Nil(t, identity)
			} else {
				require.NoError(t, err)
				require.Equal(t, testAccount, identity.UID)
			}
			require.Equal(t, 1, authority.calls)
		})
	}
}

// whoAmITestTransport is an in-memory HTTP transport for the legacy authority core.
type whoAmITestTransport func(*http.Request) (*http.Response, error)

// RoundTrip returns only the explicitly supplied synthetic response.
func (fn whoAmITestTransport) RoundTrip(req *http.Request) (*http.Response, error) { return fn(req) }

// TestWhoAmITransportRetainsContract verifies fixed authority targeting, original
// bearer forwarding, response bounds and redirect refusal in the shared core.
func TestWhoAmITransportRetainsContract(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		expected   error
	}{
		{"identity", `{"data":{"WhoAmI":{"id":"` + testAccount + `","username":"synthetic"}}}`, 200, nil},
		{"unauthorized", "{}", 401, ErrSessionInvalid}, {"forbidden", "{}", 403, ErrSessionInvalid},
		{"unavailable", "{}", 500, ErrUnavailable}, {"redirect", "{}", 307, ErrUnavailable},
		{"errors", `{"errors":[{"message":"never reflect SYNTHETIC_TOKEN"}],"data":null}`, 200, ErrSessionInvalid},
		{"missing_identity", `{"data":null}`, 200, ErrUnavailable},
		{"malformed", "not-json", 200, ErrUnavailable},
		{"oversized", strings.Repeat("x", MaxResponseBytes+1), 200, ErrUnavailable},
		{"malformed_uid", `{"data":{"WhoAmI":{"id":"not-uuid"}}}`, 200, ErrUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := NewVerifier(DefaultEndpoint, DefaultTimeout)
			calls := 0
			v.client.Transport = whoAmITestTransport(func(req *http.Request) (*http.Response, error) {
				calls++
				require.Equal(t, DefaultEndpoint, req.URL.String())
				require.Equal(t, "Bearer SYNTHETIC_TOKEN", req.Header.Get("Authorization"))
				raw, err := io.ReadAll(req.Body)
				require.NoError(t, err)
				require.Contains(t, string(raw), "WhoAmI")
				return &http.Response{StatusCode: tc.status, Header: http.Header{"Location": []string{"https://other.fixture/never"}}, Body: io.NopCloser(strings.NewReader(tc.body)), Request: req}, nil
			})
			identity, err := v.Verify(t.Context(), "SYNTHETIC_TOKEN")
			if tc.expected != nil {
				require.ErrorIs(t, err, tc.expected)
				require.Nil(t, identity)
				require.NotContains(t, err.Error(), "SYNTHETIC_TOKEN")
			} else {
				require.NoError(t, err)
				require.Equal(t, testAccount, identity.UID)
			}
			require.Equal(t, 1, calls)
		})
	}
}

// TestStrictPrivateSemanticsDoNotStrengthenLegacyCV preserves CV's existing
// parser, while private ownership separately rejects old unsupported claim shapes.
func TestStrictPrivateSemanticsDoNotStrengthenLegacyCV(t *testing.T) {
	_, private, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	claims := testClaims(time.Now().UTC())
	delete(claims, "iat")
	token := testSignedSession(t, private, claims, "HS256")
	uid, err := ClaimedUID(token)
	require.NoError(t, err)
	require.Equal(t, testAccount, uid)
	_, err = StrictClaimedUID(token, time.Now().UTC())
	require.ErrorIs(t, err, ErrSessionInvalid)
}
