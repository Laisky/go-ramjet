package http

import (
	"crypto/subtle"
	"net/url"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
)

const (
	ctxKeyUser     string = "ctx_user"
	ctxKeyUserAuth string = "ctx_user_auth"
)

// GetRawUserToken returns the original user token from the Authorization header.
//
// It is cached into the gin context by getUserByAuthHeader.
func GetRawUserToken(gctx *gin.Context) string {
	if gctx == nil {
		return ""
	}
	if v, ok := gctx.Get(ctxKeyUserAuth); ok {
		if s, ok := v.(string); ok {
			return strings.TrimSpace(s)
		}
	}
	if gctx.Request == nil {
		return ""
	}
	token, ok := modelAuthorizationToken(gctx.Request.Header)
	if !ok {
		return ""
	}
	return token
}

func getUserByAuthHeader(gctx *gin.Context) (*config.UserConfig, error) {
	if cached, ok := gctx.Get(ctxKeyUser); ok {
		return cached.(*config.UserConfig), nil
	}

	header := gctx.Request.Header
	token, ok := modelAuthorizationToken(header)
	if !ok {
		return nil, errors.New("Invalid model authorization")
	}
	if token == "" {
		token = config.FreetierUserToken
	}
	user, err := getUserByToken(gctx, token)
	if err != nil {
		return nil, errors.Wrap(err, "authenticate model user")
	}
	gctx.Set(ctxKeyUserAuth, token)
	gctx.Set(ctxKeyUser, user)
	return user, nil
}

// getUserByToken resolves an explicit native session or caller-provided key.
// Only a recognized native free session may select an operator-owned key.
func getUserByToken(gctx *gin.Context, token string) (*config.UserConfig, error) {
	if !validModelToken(gctx.Request.Header, token) || config.Config == nil {
		return nil, errors.New("Invalid model credential")
	}

	if cached, ok := gctx.Get(ctxKeyUser); ok {
		user := cached.(*config.UserConfig)
		if subtle.ConstantTimeCompare([]byte(user.Token), []byte(token)) == 1 &&
			(!user.BYOK || subtle.ConstantTimeCompare([]byte(user.OpenaiToken), []byte(token)) == 1) {
			return user, nil
		}
		return nil, errors.New("Model credential does not match the authenticated request")
	}

	var user config.UserConfig
	switch {
	case token == config.FreetierUserToken || strings.HasPrefix(token, "FREETIER-"):
		if len(token) < 15 {
			return nil, errors.New("Invalid free-tier credential")
		}
		free, err := config.Config.FreeTierPolicy()
		if err != nil {
			return nil, errors.Wrap(err, "resolve free-tier policy")
		}
		if free == nil {
			return nil, errors.New("Free-tier inference is not configured")
		}
		user = *free
		user.IsFree = true
		user.UserName = token[:15]
		user.APIBaseUserSelected = false
		user.APIBase = config.Config.ManagedAPIBase()
	case isCallerModelToken(token):
		user = config.UserConfig{
			UserName: token[:15], Token: token, OpenaiToken: token, BYOK: true,
			AllowedModels: []string{"*"}, APIBase: config.DefaultOneAPIBase,
		}
		logger := gmw.GetLogger(gctx)
		logger.Debug("use laisky\x27s oneapi token")
		applyUserAPIBaseOverride(gctx, &user, logger)
	default:
		return nil, errors.New("Invalid model credential")
	}

	if err := user.Valid(); err != nil {
		return nil, errors.Wrap(err, "validate model user")
	}
	return &user, nil
}

// applyUserAPIBaseOverride applies strict user-provided API base for BYOK users.
//
// Parameters:
//   - gctx: Current gin request context.
//   - user: Target user config to modify in place.
//   - logger: Request logger used for diagnostics.
func applyUserAPIBaseOverride(gctx *gin.Context, user *config.UserConfig, logger glog.Logger) {
	if gctx == nil || user == nil || !user.BYOK {
		return
	}

	raw := strings.TrimSpace(gctx.Request.Header.Get("X-Laisky-Api-Base"))
	if raw == "" {
		return
	}

	override, err := validateAPIBase(raw)
	if err != nil {
		logger.Warn("ignore invalid X-Laisky-Api-Base")
		return
	}

	user.APIBase = override
	user.APIBaseUserSelected = true

	logger.Debug("use user's own api base")
}

// validateAPIBase validates and normalizes a user-provided API base URL.
//
// Parameters:
//   - raw: Raw API base header value.
//
// Returns:
//   - string: Normalized API base URL without trailing slash.
//   - error: Non-nil when URL is invalid or unsafe.
func validateAPIBase(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", errors.Wrap(err, "parse api base")
	}

	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return "", errors.Errorf("unsupported api base scheme %q", parsed.Scheme)
	}

	if parsed.Host == "" {
		return "", errors.New("api base host is empty")
	}

	if parsed.User != nil {
		return "", errors.New("api base must not contain user info")
	}

	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("api base must not contain query or fragment")
	}

	normalized := strings.TrimRight(parsed.String(), "/")
	if normalized == "" {
		return "", errors.New("api base is empty")
	}

	return normalized, nil
}
