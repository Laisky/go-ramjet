package ssosession

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestStrictSSOSessionSocketRetained exercises the production HTTP verifier and
// strict private-session wrapper over real TCP against a signature-validating
// authority fixture. It does not run GraphQL/Mongo or a second server process.
func TestStrictSSOSessionSocketRetained(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	_, foreignPrivate, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	valid := testSignedSession(t, private, testClaims(time.Now().UTC()), "EdDSA")
	foreign := testSignedSession(t, foreignPrivate, testClaims(time.Now().UTC()), "EdDSA")
	expiredClaims := testClaims(time.Now().UTC())
	expiredClaims["iat"] = time.Now().Add(-2 * time.Hour).Unix()
	expiredClaims["exp"] = time.Now().Add(-time.Hour).Unix()
	expired := testSignedSession(t, private, expiredClaims, "EdDSA")
	for _, tc := range []struct {
		name, token string
		expected    error
		calls       int32
	}{
		{name: "active", token: valid, calls: 1},
		{name: "foreign_signature", token: foreign, expected: ErrSessionInvalid, calls: 1},
		{name: "inactive", token: valid, expected: ErrSessionInvalid, calls: 1},
		{name: "wrong_account", token: valid, expected: ErrSessionInvalid, calls: 1},
		{name: "unavailable", token: valid, expected: ErrUnavailable, calls: 1},
		{name: "redirect", token: valid, expected: ErrUnavailable, calls: 1},
		{name: "ambiguous_identity", token: valid, expected: ErrUnavailable, calls: 1},
		{name: "expired_before_authority", token: expired, expected: ErrSessionInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls, redirected atomic.Int32
			seen := make(chan bool, 1)
			sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				redirected.Add(1)
				w.WriteHeader(http.StatusUnauthorized)
			}))
			t.Cleanup(sink.Close)
			authority := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				var operation map[string]string
				decodeErr := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&operation)
				if decodeErr != nil {
					seen <- false
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				requestValid := r.Method == http.MethodPost && r.URL.Path == "/query" &&
					subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+tc.token)) == 1 &&
					r.Header.Get("Content-Type") == "application/json" && strings.Contains(operation["query"], "WhoAmI")
				seen <- requestValid
				if !requestValid {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				if tc.name == "redirect" {
					http.Redirect(w, r, sink.URL+"/never", http.StatusTemporaryRedirect)
					return
				}
				fixture := &strictTestAuthority{public: public, inactive: tc.name == "inactive"}
				identity, verifyErr := fixture.Verify(r.Context(), tc.token)
				if verifyErr != nil {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				if tc.name == "unavailable" {
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				if tc.name == "wrong_account" {
					identity.UID = "0199b2a4-0000-7000-8000-0000000f0e1d"
				}
				body := `{"data":{"WhoAmI":{"id":"` + identity.UID + `","username":"synthetic@example.test"}}}`
				if tc.name == "ambiguous_identity" {
					body = `{"data":{"WhoAmI":{"id":"` + identity.UID + `","id":"` + identity.UID + `"}}}`
				}
				w.Header().Set("Content-Type", "application/json")
				if _, writeErr := w.Write([]byte(body)); writeErr != nil {
					t.Errorf("write synthetic authority response: %v", writeErr)
				}
			}))
			t.Cleanup(authority.Close)
			base := NewVerifier(authority.URL+"/query", 2*time.Second)
			base.client.Transport = &http.Transport{}
			t.Cleanup(base.client.CloseIdleConnections)
			identity, verifyErr := NewStrictVerifier(base).Verify(t.Context(), tc.token)
			if tc.expected != nil {
				require.ErrorIs(t, verifyErr, tc.expected)
				require.Nil(t, identity)
				require.NotContains(t, verifyErr.Error(), tc.token)
			} else {
				require.NoError(t, verifyErr)
				require.Equal(t, testAccount, identity.UID)
			}
			require.Equal(t, tc.calls, calls.Load())
			require.Zero(t, redirected.Load(), "SSO redirects must not replay original sessions")
			if tc.calls > 0 {
				require.True(t, <-seen, "the original session must reach only the intended GraphQL authority")
			}
		})
	}
}
