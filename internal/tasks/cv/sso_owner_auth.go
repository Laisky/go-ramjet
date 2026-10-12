package cv

import (
	"crypto/subtle"
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

	"github.com/Laisky/go-ramjet/library/log"
	"github.com/Laisky/go-ramjet/library/ssosession"
)

const (
	defaultSSOGraphQLEndpoint = ssosession.DefaultEndpoint
	defaultSSOVerifyTimeout   = ssosession.DefaultTimeout
	maxSSOTokenLength         = ssosession.MaxTokenLength
	ctxKeyCVOwner             = "cv_owner_identity"
)

var (
	errSSOSessionInvalid = ssosession.ErrSessionInvalid
	errSSOUnavailable    = ssosession.ErrUnavailable
)

type ssoIdentity = ssosession.Identity
type ssoSessionVerifier = ssosession.SessionVerifier
type whoAmIVerifier = ssosession.Verifier

// newWhoAmIVerifier preserves CV's server-configured authority and timeout.
func newWhoAmIVerifier(endpoint string, timeout time.Duration) *whoAmIVerifier {
	return ssosession.NewVerifier(endpoint, timeout)
}

// normalizeSSOUID preserves CV's existing UUID normalization.
func normalizeSSOUID(raw string) (string, error) {
	return ssosession.NormalizeUID(raw)
}

// bearerTokenFromHeader preserves CV's existing bounded bearer parsing.
func bearerTokenFromHeader(header string) (string, bool) {
	return ssosession.BearerToken(header)
}

// claimedSSOUID preserves CV's existing untrusted subject/UID parsing.
func claimedSSOUID(token string) (string, error) {
	return ssosession.ClaimedUID(token)
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
