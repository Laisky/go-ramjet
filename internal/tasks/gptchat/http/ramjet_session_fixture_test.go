package http

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/library/ssosession"
)

const (
	ramjetTestAccountA = "0199b2a4-0000-7000-8000-00000000c0de"
	ramjetTestAccountB = "0199b2a4-0000-7000-8000-0000000f0e1d"
	ramjetTestAccountC = "0199b2a4-0000-7000-8000-0000000dead0"
)

// ramjetTestAuthority verifies synthetic EdDSA signatures and records only local
// authority calls. It never contacts the real SSO service.
type ramjetTestAuthority struct {
	public      ed25519.PublicKey
	calls       int
	overrideUID string
	unavailable bool
	inactive    bool
}

// Verify models the authority's signature/active-account decision for local
// synthetic sessions; the actual strict wrapper still performs claim binding.
func (a *ramjetTestAuthority) Verify(_ context.Context, token string) (*ssosession.Identity, error) {
	a.calls++
	if a.unavailable {
		return nil, ssosession.ErrUnavailable
	}
	if a.inactive {
		return nil, ssosession.ErrSessionInvalid
	}
	parts := splitSessionParts(token)
	if len(parts) != 3 {
		return nil, ssosession.ErrSessionInvalid
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !ed25519.Verify(a.public, []byte(parts[0]+"."+parts[1]), signature) {
		return nil, ssosession.ErrSessionInvalid
	}
	uid, err := ssosession.StrictClaimedUID(token, time.Now().UTC())
	if err != nil {
		return nil, errors.Wrap(err, "synthetic authority claims")
	}
	if a.overrideUID != "" {
		uid = a.overrideUID
	}
	return &ssosession.Identity{UID: uid, Username: "synthetic@example.test"}, nil
}

// splitSessionParts separates a synthetic compact token without modifying it.
func splitSessionParts(token string) []string {
	parts := []string{}
	start := 0
	for i := range len(token) {
		if token[i] == '.' {
			parts = append(parts, token[start:i])
			start = i + 1
		}
	}
	return append(parts, token[start:])
}

// ramjetSessionTestSetup installs a strict verifier backed by a local signature
// authority and returns an original synthetic session for the requested account.
func ramjetSessionTestSetup(t *testing.T, uid string) (string, *ramjetTestAuthority) {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	authority := &ramjetTestAuthority{public: public}
	previous := ramjetSessionVerifier
	ramjetSessionVerifier = ssosession.NewStrictVerifier(authority)
	t.Cleanup(func() { ramjetSessionVerifier = previous })
	now := time.Now().UTC()
	claims := map[string]any{
		"iss": "laisky-sso", "sub": uid, "uid": uid,
		"iat": now.Add(-time.Minute).Unix(), "exp": now.Add(time.Hour).Unix(),
		"jti": "0199b2a4-0000-7000-8000-00000000a111", "username": "synthetic@example.test", "display_name": "Synthetic",
	}
	payload, err := json.Marshal(claims)
	require.NoError(t, err)
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"EdDSA","typ":"JWT"}`))
	unsigned := header + "." + base64.RawURLEncoding.EncodeToString(payload)
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(ed25519.Sign(private, []byte(unsigned))), authority
}

// requireRamjetSession attaches the synthetic session only to a local request.
func requireRamjetSession(t *testing.T, header http.Header, uid string) *ramjetTestAuthority {
	t.Helper()
	token, authority := ramjetSessionTestSetup(t, uid)
	header.Set(ramjetSessionHeader, "Bearer "+token)
	return authority
}
