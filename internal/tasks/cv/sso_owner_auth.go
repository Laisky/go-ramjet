package cv

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	gconfig "github.com/Laisky/go-config/v2"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/Laisky/go-ramjet/library/log"
)

const (
	// defaultSSOGraphQLEndpoint is the trusted SSO GraphQL endpoint.
	defaultSSOGraphQLEndpoint = "https://sso.laisky.com/query"
	// defaultSSOVerifyTimeout bounds one WhoAmI round trip.
	defaultSSOVerifyTimeout = 5 * time.Second
	// maxSSOTokenLength bounds the bearer token accepted before contacting SSO.
	maxSSOTokenLength = 8192
	// maxSSOResponseBytes bounds the WhoAmI response body CV will read.
	maxSSOResponseBytes = 64 << 10
	// ssoWhoAmIQuery asks SSO to validate the bearer token and resolve its account.
	ssoWhoAmIQuery = `query CVOwnerSession { WhoAmI { id username } }`
	// ctxKeyCVOwner stores the verified owner identity in the gin context.
	ctxKeyCVOwner = "cv_owner_identity"
)

var (
	// errSSOSessionInvalid means SSO rejected the session: missing, malformed,
	// expired, tampered, foreign or inactive. The user must sign in again.
	errSSOSessionInvalid = errors.New("sso session is invalid or expired")
	// errSSOUnavailable means SSO could not give a trustworthy answer. The
	// session may still be valid; nothing is authorized.
	errSSOUnavailable = errors.New("sso service is unavailable")
)

// ssoIdentity is the account SSO resolved for a bearer token.
type ssoIdentity struct {
	UID      string `json:"uid"`
	Username string `json:"username"`
}

// ssoSessionVerifier validates an SSO bearer token and resolves its account.
type ssoSessionVerifier interface {
	// Verify returns the account behind token, errSSOSessionInvalid when SSO
	// rejects the token, or errSSOUnavailable when SSO cannot answer reliably.
	Verify(ctx context.Context, token string) (*ssoIdentity, error)
}

// whoAmIVerifier validates sessions with the SSO GraphQL WhoAmI query, which
// checks the signature, expiry, issuer, subject/UID consistency and that the
// account is still active. CV never holds SSO signing material.
type whoAmIVerifier struct {
	endpoint string
	client   *http.Client
}

// newWhoAmIVerifier builds a verifier for a fixed, validated endpoint.
// It accepts the endpoint and timeout and returns the verifier.
func newWhoAmIVerifier(endpoint string, timeout time.Duration) *whoAmIVerifier {
	return &whoAmIVerifier{
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

// Verify implements ssoSessionVerifier.
func (v *whoAmIVerifier) Verify(ctx context.Context, token string) (*ssoIdentity, error) {
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
		return nil, errors.Wrapf(errSSOUnavailable, "request sso whoami: %v", redactTransportError(err))
	}
	defer func() { _ = resp.Body.Close() }()

	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return nil, errors.Wrapf(errSSOSessionInvalid, "sso whoami returned status %d", resp.StatusCode)
	case resp.StatusCode != http.StatusOK:
		return nil, errors.Wrapf(errSSOUnavailable, "sso whoami returned status %d", resp.StatusCode)
	}

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxSSOResponseBytes+1))
	if err != nil {
		return nil, errors.Wrapf(errSSOUnavailable, "read sso whoami response: %v", err)
	}
	if len(raw) > maxSSOResponseBytes {
		return nil, errors.Wrap(errSSOUnavailable, "sso whoami response is too large")
	}

	var payload whoAmIResponse
	if err = json.Unmarshal(raw, &payload); err != nil {
		return nil, errors.Wrap(errSSOUnavailable, "sso whoami response is not json")
	}
	if len(payload.Errors) > 0 {
		return nil, errors.Wrap(errSSOSessionInvalid, "sso rejected the session")
	}
	if payload.Data == nil || payload.Data.WhoAmI == nil {
		return nil, errors.Wrap(errSSOUnavailable, "sso whoami response has no identity")
	}
	uid, err := normalizeSSOUID(payload.Data.WhoAmI.ID)
	if err != nil {
		return nil, errors.Wrap(errSSOUnavailable, "sso whoami returned a malformed uid")
	}

	return &ssoIdentity{UID: uid, Username: payload.Data.WhoAmI.Username}, nil
}

// redactTransportError drops the request URL from a transport error so a
// misconfigured endpoint containing credentials can never reach logs.
func redactTransportError(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return urlErr.Err
	}
	return err
}

// normalizeSSOUID validates a canonical SSO UID and returns it in lowercase.
func normalizeSSOUID(raw string) (string, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", errors.Wrap(err, "parse sso uid")
	}
	return parsed.String(), nil
}

// cvOwnerAuth authorizes CV editing for the configured owner accounts only.
type cvOwnerAuth struct {
	verifier  ssoSessionVerifier
	owners    []string
	configErr error
}

// newCVOwnerAuthFromConfig reads tasks.cv.sso.* and builds the owner guard.
// Configuration problems never open the routes: the guard then rejects every
// protected request with cv_owner_not_configured.
func newCVOwnerAuthFromConfig() *cvOwnerAuth {
	endpoint := strings.TrimSpace(gconfig.Shared.GetString("tasks.cv.sso.graphql_endpoint"))
	if endpoint == "" {
		endpoint = defaultSSOGraphQLEndpoint
	}
	timeout := gconfig.Shared.GetDuration("tasks.cv.sso.timeout")
	if timeout <= 0 {
		timeout = defaultSSOVerifyTimeout
	}

	guard := &cvOwnerAuth{}
	if err := validateSSOEndpoint(endpoint); err != nil {
		guard.configErr = errors.Wrap(err, "validate tasks.cv.sso.graphql_endpoint")
		return guard
	}
	guard.verifier = newWhoAmIVerifier(endpoint, timeout)

	for _, raw := range gconfig.Shared.GetStringSlice("tasks.cv.sso.owner_uids") {
		uid, err := normalizeSSOUID(raw)
		if err != nil {
			guard.configErr = errors.Wrap(err, "validate tasks.cv.sso.owner_uids")
			guard.owners = nil
			return guard
		}
		guard.owners = append(guard.owners, uid)
	}
	if len(guard.owners) == 0 {
		guard.configErr = errors.New("tasks.cv.sso.owner_uids is empty")
	}
	return guard
}

// validateSSOEndpoint requires HTTPS, except for loopback development hosts.
func validateSSOEndpoint(endpoint string) error {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return errors.Wrap(err, "parse endpoint")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("endpoint must not carry credentials, a query or a fragment")
	}
	switch parsed.Scheme {
	case "https":
		if parsed.Hostname() == "" {
			return errors.New("endpoint host is empty")
		}
		return nil
	case "http":
		if ip := net.ParseIP(parsed.Hostname()); ip != nil && ip.IsLoopback() {
			return nil
		}
		if parsed.Hostname() == "localhost" {
			return nil
		}
		return errors.New("endpoint must use https")
	default:
		return errors.New("endpoint must use https")
	}
}

// isOwner reports whether uid is a configured owner, in constant time per entry.
func (a *cvOwnerAuth) isOwner(uid string) bool {
	matched := 0
	for _, owner := range a.owners {
		matched |= subtle.ConstantTimeCompare([]byte(owner), []byte(uid))
	}
	return matched == 1
}

// abortCV writes a machine-readable rejection that never contains the token.
func abortCV(c *gin.Context, status int, code string, message string) {
	c.AbortWithStatusJSON(status, gin.H{"code": code, "error": message})
}

// Middleware authorizes one request: the bearer token must be a well-formed
// JWT, SSO must accept it through WhoAmI, its claimed subject must match the
// identity SSO resolved, and that identity must be a configured owner.
func (a *cvOwnerAuth) Middleware(c *gin.Context) {
	logger := gmw.GetLogger(c).Named("cv_owner_auth")

	token, ok := bearerTokenFromHeader(c.GetHeader("Authorization"))
	if !ok {
		abortCV(c, http.StatusUnauthorized, "sso_session_missing", "sign in with SSO to edit this CV")
		return
	}
	claimedUID, err := claimedSSOUID(token)
	if err != nil {
		logger.Debug("reject malformed sso token before contacting sso", zap.Error(err))
		abortCV(c, http.StatusUnauthorized, "sso_session_invalid", errSSOSessionInvalid.Error())
		return
	}
	if a.configErr != nil || a.verifier == nil {
		logger.Error("cv owner authorization is not configured", zap.Error(a.configErr))
		abortCV(c, http.StatusServiceUnavailable, "cv_owner_not_configured", "CV editing is not configured")
		return
	}

	start := time.Now()
	identity, err := a.verifier.Verify(gmw.Ctx(c), token)
	if err != nil {
		if errors.Is(err, errSSOSessionInvalid) {
			logger.Info("sso rejected cv session", zap.Error(err), zap.Duration("cost", time.Since(start)))
			abortCV(c, http.StatusUnauthorized, "sso_session_invalid", errSSOSessionInvalid.Error())
			return
		}
		logger.Warn("sso unavailable while verifying cv session", zap.Error(err), zap.Duration("cost", time.Since(start)))
		c.Header("Retry-After", "30")
		abortCV(c, http.StatusServiceUnavailable, "sso_unavailable", errSSOUnavailable.Error())
		return
	}
	if subtle.ConstantTimeCompare([]byte(identity.UID), []byte(claimedUID)) != 1 {
		logger.Warn("sso identity does not match the token subject",
			zap.String("sso_uid", identity.UID), zap.String("claimed_uid", claimedUID))
		abortCV(c, http.StatusUnauthorized, "sso_session_invalid", errSSOSessionInvalid.Error())
		return
	}
	if !a.isOwner(identity.UID) {
		logger.Info("non-owner cv session rejected", zap.String("uid", identity.UID))
		abortCV(c, http.StatusForbidden, "cv_owner_required", "this account is not allowed to edit the CV")
		return
	}

	logger.Debug("cv owner session verified", zap.String("uid", identity.UID), zap.Duration("cost", time.Since(start)))
	c.Set(ctxKeyCVOwner, identity)
	c.Next()
}

// bearerTokenFromHeader extracts a bounded bearer token from an Authorization header.
func bearerTokenFromHeader(header string) (string, bool) {
	scheme, token, found := strings.Cut(strings.TrimSpace(header), " ")
	if !found || !strings.EqualFold(scheme, "Bearer") {
		return "", false
	}
	token = strings.TrimSpace(token)
	return token, token != ""
}

// claimedSSOUID parses, without trusting, the subject an SSO JWT claims. It
// rejects anything that is not a bounded three-part JWT whose sub and uid are
// the same canonical UUID, so garbage never reaches SSO and the identity SSO
// later resolves can be cross-checked against the token's own claim.
func claimedSSOUID(token string) (string, error) {
	if len(token) > maxSSOTokenLength {
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
	subject, err := normalizeSSOUID(claims.Subject)
	if err != nil {
		return "", errors.Wrap(err, "token subject")
	}
	uid, err := normalizeSSOUID(claims.UID)
	if err != nil {
		return "", errors.Wrap(err, "token uid")
	}
	if subject != uid {
		return "", errors.New("token subject and uid differ")
	}
	return uid, nil
}

// getAuthSession reports the verified owner session so the editor can enable
// itself only after the server has authorized the account.
func (h *handler) getAuthSession(c *gin.Context) {
	identity, _ := c.Get(ctxKeyCVOwner)
	owner, ok := identity.(*ssoIdentity)
	if !ok || owner == nil {
		abortCV(c, http.StatusUnauthorized, "sso_session_invalid", errSSOSessionInvalid.Error())
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"uid": owner.UID, "username": owner.Username, "owner": true})
}

// logCVOwnerAuthConfig reports, once at startup, whether CV editing can be authorized.
func logCVOwnerAuthConfig(guard *cvOwnerAuth) {
	if guard.configErr != nil {
		log.Logger.Error("cv editing is disabled until tasks.cv.sso is configured", zap.Error(guard.configErr))
		return
	}
	log.Logger.Info("cv editing is restricted to configured sso owners", zap.Int("owners", len(guard.owners)))
}
