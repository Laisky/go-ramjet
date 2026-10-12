package ssosession

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"math"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
)

// StrictVerifier requires the original current Laisky EdDSA session and an
// independently resolved matching account. It does not apply CV owner policy.
type StrictVerifier struct {
	authority SessionVerifier
}

// NewStrictVerifier wraps a server-controlled WhoAmI authority without changing
// the legacy verifier's parsing semantics.
func NewStrictVerifier(authority SessionVerifier) *StrictVerifier {
	if base, ok := authority.(*Verifier); ok {
		if base == nil {
			return &StrictVerifier{}
		}
		isolated := *base
		isolated.strict = true
		authority = &isolated
	}
	return &StrictVerifier{authority: authority}
}

// NewDefaultStrictVerifier uses only the established fixed HTTPS authority.
func NewDefaultStrictVerifier() *StrictVerifier {
	return NewStrictVerifier(NewVerifier(DefaultEndpoint, DefaultTimeout))
}

// StrictClaimedUID returns the canonical identity claimed by a bounded, current
// EdDSA Laisky session. It does not verify a signature or authorize an account.
func StrictClaimedUID(token string, now time.Time) (string, error) {
	if len(token) == 0 || len(token) > MaxTokenLength {
		return "", errors.Wrap(ErrSessionInvalid, "invalid original session size")
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return "", errors.Wrap(ErrSessionInvalid, "original session is not a compact JWT")
	}
	for _, part := range parts {
		if strings.IndexFunc(part, func(r rune) bool {
			return !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' || r == '-')
		}) >= 0 {
			return "", errors.Wrap(ErrSessionInvalid, "invalid compact session component")
		}
	}
	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", errors.Wrap(ErrSessionInvalid, "invalid session header")
	}
	var header struct {
		Algorithm string          `json:"alg"`
		Type      json.RawMessage `json:"typ"`
		Critical  json.RawMessage `json:"crit"`
		B64       json.RawMessage `json:"b64"`
	}
	if requireUnambiguousJSON(headerJSON) != nil {
		return "", errors.Wrap(ErrSessionInvalid, "ambiguous session header")
	}
	if err = decodeExactFields(headerJSON, &header, "alg", "typ", "crit", "b64"); err != nil || header.Algorithm != "EdDSA" {
		return "", errors.Wrap(ErrSessionInvalid, "session must use EdDSA")
	}
	if len(header.Type) > 0 {
		var tokenType string
		if json.Unmarshal(header.Type, &tokenType) != nil || tokenType != "JWT" {
			return "", errors.Wrap(ErrSessionInvalid, "unsupported session type")
		}
	}
	if len(header.Critical) > 0 {
		return "", errors.Wrap(ErrSessionInvalid, "unsupported critical session header")
	}
	if len(header.B64) > 0 {
		var encoded bool
		if json.Unmarshal(header.B64, &encoded) != nil || !encoded {
			return "", errors.Wrap(ErrSessionInvalid, "unsupported session payload encoding")
		}
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(signature) != 64 {
		return "", errors.Wrap(ErrSessionInvalid, "invalid EdDSA session signature encoding")
	}
	payloadJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", errors.Wrap(ErrSessionInvalid, "invalid session payload")
	}
	var claims struct {
		Issuer      string   `json:"iss"`
		Subject     string   `json:"sub"`
		UID         string   `json:"uid"`
		Expires     *float64 `json:"exp"`
		IssuedAt    *float64 `json:"iat"`
		JWTID       *string  `json:"jti"`
		Username    *string  `json:"username"`
		DisplayName *string  `json:"display_name"`
	}
	if requireUnambiguousJSON(payloadJSON) != nil {
		return "", errors.Wrap(ErrSessionInvalid, "ambiguous session claims")
	}
	if err = decodeExactFields(payloadJSON, &claims, "iss", "sub", "uid", "exp", "iat", "jti", "username", "display_name"); err != nil {
		return "", errors.Wrap(ErrSessionInvalid, "invalid original session claims")
	}
	current := float64(now.Unix()) + float64(now.Nanosecond())/1e9
	if claims.Issuer != "laisky-sso" || claims.Expires == nil || claims.IssuedAt == nil || math.IsNaN(*claims.Expires) || math.IsInf(*claims.Expires, 0) || math.IsNaN(*claims.IssuedAt) || math.IsInf(*claims.IssuedAt, 0) || *claims.Expires <= current || *claims.IssuedAt > current+60 || *claims.IssuedAt >= *claims.Expires {
		return "", errors.Wrap(ErrSessionInvalid, "original session issuer or expiry is invalid")
	}
	if claims.JWTID == nil || *claims.JWTID == "" || claims.Username == nil || *claims.Username == "" || claims.DisplayName == nil {
		return "", errors.Wrap(ErrSessionInvalid, "required original session claims are missing")
	}
	uid, err := strictNormalizeUID(claims.UID)
	if err != nil || strings.ToLower(claims.UID) != uid || strings.ToLower(claims.Subject) != uid {
		return "", errors.Wrap(ErrSessionInvalid, "original session subject and uid must be the same canonical UUID")
	}
	return uid, nil
}

// Verify checks the strict claim shape before contacting the authority, then
// requires the online active account identity to match the original session.
func (v *StrictVerifier) Verify(ctx context.Context, token string) (*Identity, error) {
	uid, err := StrictClaimedUID(token, time.Now().UTC())
	if err != nil {
		return nil, errors.Wrap(err, "validate original session")
	}
	if v == nil || v.authority == nil {
		return nil, errors.Wrap(ErrUnavailable, "original session authority is not configured")
	}
	identity, err := v.authority.Verify(ctx, token)
	if err != nil {
		return nil, errors.Wrap(err, "verify original session with SSO")
	}
	if identity == nil || subtle.ConstantTimeCompare([]byte(identity.UID), []byte(uid)) != 1 {
		return nil, errors.Wrap(ErrSessionInvalid, "SSO account does not match the original session")
	}
	if _, err = StrictClaimedUID(token, time.Now().UTC()); err != nil {
		return nil, errors.Wrap(err, "original session expired during verification")
	}
	return identity, nil
}

// HasLaiskyIssuer recognizes session-shaped model credentials for rejection.
// It grants no identity and never contacts the authority.
func HasLaiskyIssuer(token string) bool {
	if len(token) > MaxTokenLength {
		return false
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return false
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}
	var claims struct {
		Issuer string `json:"iss"`
	}
	if err = json.Unmarshal(raw, &claims); err != nil {
		return false
	}
	return claims.Issuer == "laisky-sso"
}

// strictNormalizeUID accepts canonical UUID text case-insensitively and rejects
// nil, compact, braced or whitespace variants for private account identity.
func strictNormalizeUID(raw string) (string, error) {
	uid, err := NormalizeUID(raw)
	if err != nil || uid == "00000000-0000-0000-0000-000000000000" || strings.ToLower(raw) != uid {
		return "", errors.Wrap(ErrSessionInvalid, "invalid canonical account UUID")
	}
	return uid, nil
}
