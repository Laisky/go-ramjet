// Package ssosession verifies original sessions with the server-owned Laisky SSO authority.
package ssosession

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/google/uuid"
)

const (
	// DefaultEndpoint is the established fixed Laisky SSO GraphQL authority.
	DefaultEndpoint = "https://sso.laisky.com/query"
	// DefaultTimeout bounds a session verification round trip.
	DefaultTimeout = 5 * time.Second
	// MaxTokenLength bounds the original session accepted before authority calls.
	MaxTokenLength = 8192
	// MaxResponseBytes bounds the GraphQL response body.
	MaxResponseBytes = 64 << 10
	ssoWhoAmIQuery   = `query CVOwnerSession { WhoAmI { id username } }`
)

var (
	// ErrSessionInvalid means the authority rejected the original session.
	ErrSessionInvalid = errors.New("sso session is invalid or expired")
	// ErrUnavailable means the authority cannot give a trustworthy answer.
	ErrUnavailable = errors.New("sso service is unavailable")
)

// Identity is the account SSO resolved for a bearer token.
type Identity struct {
	UID      string `json:"uid"`
	Username string `json:"username"`
}

// SessionVerifier validates an SSO bearer token and resolves its account.
type SessionVerifier interface {
	// Verify returns the account behind token, ErrSessionInvalid when SSO
	// rejects the token, or ErrUnavailable when SSO cannot answer reliably.
	Verify(ctx context.Context, token string) (*Identity, error)
}

// Verifier validates sessions with the SSO GraphQL WhoAmI query, which
// checks the signature, expiry, issuer, subject/UID consistency and that the
// account is still active. CV never holds SSO signing material.
type Verifier struct {
	endpoint string
	client   *http.Client
	strict   bool
}

// NewVerifier builds a verifier for a fixed, validated endpoint.
// It accepts the endpoint and timeout and returns the verifier.
func NewVerifier(endpoint string, timeout time.Duration) *Verifier {
	return &Verifier{
		endpoint: endpoint,
		client: &http.Client{
			Timeout: timeout,
			// Never follow redirects: a redirect would forward the bearer token to
			// a location CV did not choose.
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// whoAmIResponse is the GraphQL envelope returned by SSO.
type whoAmIResponse struct {
	Data *struct {
		WhoAmI *struct {
			ID       string `json:"id"`
			Username string `json:"username"`
		} `json:"WhoAmI"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// Verify implements SessionVerifier.
func (v *Verifier) Verify(ctx context.Context, token string) (*Identity, error) {
	body, err := json.Marshal(map[string]string{"query": ssoWhoAmIQuery})
	if err != nil {
		return nil, errors.Wrap(err, "marshal whoami query")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, v.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, errors.Wrap(err, "build whoami request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := v.client.Do(req)
	if err != nil {
		return nil, errors.Wrapf(ErrUnavailable, "request sso whoami: %v", RedactTransportError(err))
	}
	defer func() { _ = resp.Body.Close() }()

	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return nil, errors.Wrapf(ErrSessionInvalid, "sso whoami returned status %d", resp.StatusCode)
	case resp.StatusCode != http.StatusOK:
		return nil, errors.Wrapf(ErrUnavailable, "sso whoami returned status %d", resp.StatusCode)
	}

	raw, err := io.ReadAll(io.LimitReader(resp.Body, MaxResponseBytes+1))
	if err != nil {
		return nil, errors.Wrapf(ErrUnavailable, "read sso whoami response: %v", err)
	}
	if len(raw) > MaxResponseBytes {
		return nil, errors.Wrap(ErrUnavailable, "sso whoami response is too large")
	}

	if v.strict {
		exact, err := exactWhoAmIEnvelope(raw)
		if err != nil {
			return nil, errors.Wrap(ErrUnavailable, "sso whoami response is ambiguous")
		}
		raw = exact
	}
	var payload whoAmIResponse
	if err = json.Unmarshal(raw, &payload); err != nil {
		return nil, errors.Wrap(ErrUnavailable, "sso whoami response is not json")
	}
	if len(payload.Errors) > 0 {
		return nil, errors.Wrap(ErrSessionInvalid, "sso rejected the session")
	}
	if payload.Data == nil || payload.Data.WhoAmI == nil {
		return nil, errors.Wrap(ErrUnavailable, "sso whoami response has no identity")
	}
	normalizer := NormalizeUID
	if v.strict {
		normalizer = strictNormalizeUID
	}
	uid, err := normalizer(payload.Data.WhoAmI.ID)
	if err != nil {
		return nil, errors.Wrap(ErrUnavailable, "sso whoami returned a malformed uid")
	}

	return &Identity{UID: uid, Username: payload.Data.WhoAmI.Username}, nil
}

// RedactTransportError drops the request URL from a transport error so a
// misconfigured endpoint containing credentials can never reach logs.
func RedactTransportError(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return urlErr.Err
	}
	return err
}

// NormalizeUID validates a canonical SSO UID and returns it in lowercase.
func NormalizeUID(raw string) (string, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", errors.Wrap(err, "parse sso uid")
	}
	return parsed.String(), nil
}

// BearerToken extracts a bounded bearer token from an Authorization header.
func BearerToken(header string) (string, bool) {
	scheme, token, found := strings.Cut(strings.TrimSpace(header), " ")
	if !found || !strings.EqualFold(scheme, "Bearer") {
		return "", false
	}
	token = strings.TrimSpace(token)
	return token, token != ""
}

// ClaimedUID parses, without trusting, the subject an SSO JWT claims. It
// rejects anything that is not a bounded three-part JWT whose sub and uid are
// the same canonical UUID, so garbage never reaches SSO and the identity SSO
// later resolves can be cross-checked against the token's own claim.
func ClaimedUID(token string) (string, error) {
	if len(token) > MaxTokenLength {
		return "", errors.New("token is too long")
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return "", errors.New("token is not a compact jwt")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", errors.Wrap(err, "decode token payload")
	}
	var claims struct {
		Subject string `json:"sub"`
		UID     string `json:"uid"`
	}
	if err = json.Unmarshal(payload, &claims); err != nil {
		return "", errors.Wrap(err, "decode token claims")
	}
	subject, err := NormalizeUID(claims.Subject)
	if err != nil {
		return "", errors.Wrap(err, "token subject")
	}
	uid, err := NormalizeUID(claims.UID)
	if err != nil {
		return "", errors.Wrap(err, "token uid")
	}
	if subject != uid {
		return "", errors.New("token subject and uid differ")
	}
	return uid, nil
}
