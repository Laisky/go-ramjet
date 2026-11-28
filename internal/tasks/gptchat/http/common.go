package http

import (
	"net/url"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
	"github.com/jinzhu/copier"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
	"github.com/Laisky/go-ramjet/library/log"
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
	return strings.TrimSpace(strings.TrimPrefix(gctx.Request.Header.Get("authorization"), "Bearer "))
}

func getUserByAuthHeader(gctx *gin.Context) (user *config.UserConfig, err error) {
	if useri, ok := gctx.Get(ctxKeyUser); ok {
		return useri.(*config.UserConfig), nil
	}

	userToken := strings.TrimPrefix(gctx.Request.Header.Get("authorization"), "Bearer ")
	if userToken == "" {
		log.Logger.Debug("user token not found in header, use freetier token instead")
		userToken = config.FreetierUserToken
	}
	gctx.Set(ctxKeyUserAuth, strings.TrimSpace(userToken))

	user, err = getUserByToken(gctx, userToken)
	if err != nil {
		return nil, errors.Wrap(err, "get user by token")
	}

	gctx.Set(ctxKeyUser, user)
	return user, nil
}

// var OpenaiModelList = []string{
// 	"gpt-3.5-turbo", "gpt-3.5-turbo-0301", "gpt-3.5-turbo-0613", "gpt-3.5-turbo-1106", "gpt-3.5-turbo-0125",
// 	"gpt-3.5-turbo-16k", "gpt-3.5-turbo-16k-0613",
// 	"gpt-3.5-turbo-instruct",
// 	"gpt-4", "gpt-4-0314", "gpt-4-0613", "gpt-4-1106-preview", "gpt-4-0125-preview",
// 	"gpt-4-32k", "gpt-4-32k-0314", "gpt-4-32k-0613",
// 	"gpt-4-turbo-preview", "gpt-4-turbo", "gpt-4-turbo-2024-04-09",
// 	"gpt-4o", "gpt-4o-2024-05-13", "gpt-4o-2024-08-06", "gpt-4o-2024-11-20", "chatgpt-4o-latest",
// 	"gpt-4o-mini", "gpt-4o-mini-2024-07-18",
// 	"gpt-4o-search-preview",
// 	"gpt-4o-mini-search-preview",
// 	"gpt-4-vision-preview",
// 	"text-embedding-ada-002", "text-embedding-3-small", "text-embedding-3-large",
// 	"text-curie-001", "text-babbage-001", "text-ada-001", "text-davinci-002", "text-davinci-003",
// 	"text-moderation-latest", "text-moderation-stable",
// 	"text-davinci-edit-001",
// 	"davinci-002", "babbage-002",
// 	"dall-e-2", "dall-e-3",
// 	"whisper-1",
// 	"tts-1", "tts-1-1106", "tts-1-hd", "tts-1-hd-1106",
// 	"o1", "o1-2024-12-17",
// 	"o1-preview", "o1-preview-2024-09-12",
// 	"o1-mini", "o1-mini-2024-09-12",
// }

func getUserByToken(gctx *gin.Context, userToken string) (user *config.UserConfig, err error) {
	if useri, ok := gctx.Get(ctxKeyUser); ok {
		return useri.(*config.UserConfig), nil
	}

	logger := gmw.GetLogger(gctx).Named("get_user_by_token")
	// ctx, cancel := context.WithTimeout(gmw.Ctx(gctx), time.Second*10)
	// defer cancel()

	userToken = strings.TrimSpace(strings.TrimPrefix(userToken, "Bearer "))
	if userToken == "" {
		return nil, errors.New("empty token")
	}

SWITCH_FOR_USER:
	switch {
	case strings.HasPrefix(userToken, "FREETIER-"),
		userToken == config.FreetierUserToken: // free userch
		if len(userToken) < 15 {
			return nil, errors.New("invalid freetier token")
		}

		username := userToken[:15]
		logger.Debug("use server's freetier openai token",
			zap.String("user", username))

		for _, commFreeUser := range config.Config.UserTokens {
			if commFreeUser.Token == config.FreetierUserToken {
				user = &config.UserConfig{}
				if err = copier.Copy(user, commFreeUser); err != nil {
					return nil, errors.Wrap(err, "copy free user")
				}

				user.IsFree = true
				user.UserName = username
				break SWITCH_FOR_USER
			}
		}

		return nil, errors.Errorf("can not find freetier user %q in settings",
			config.FreetierUserToken)
	case strings.HasPrefix(userToken, "laisky-"),
		strings.HasPrefix(userToken, "sk-"):
		if len(userToken) < 15 {
			return nil, errors.New("invalid laisky's oneapi token")
		}

		username := userToken[:15]
		logger.Debug("use laisky's oneapi token")
		user = &config.UserConfig{ // default to openai user
			UserName:    username,
			Token:       userToken,
			OpenaiToken: userToken,
			// Keep the caller's key for the separate legacy Ramjet media path.
			ImageToken: userToken,
			BYOK:       true, // mark as bring-your-own-key for rate limiting & auditing logic
			// ImageTokenType:         config.Config.DefaultImageTokenType,
			// ImageUrl:               config.Config.DefaultImageUrl,
			AllowedModels:          []string{"*"},
			NoLimitExpensiveModels: true,
			APIBase:                "https://oneapi.laisky.com",
			// APIBase:  "http://100.97.108.34:3000",
			ImageUrl: "https://oneapi.laisky.com/v1/images/generations",
		}

		applyUserAPIBaseOverride(gctx, user, logger)
	default: // use server's token in settings
		return nil, errors.New("invalid token")
	}

	if err = user.Valid(); err != nil {
		return nil, errors.Wrap(err, "valid user")
	}

	return user, nil
}

// applyUserAPIBaseOverride applies strict user-provided API base for BYOK users.
//
// Parameters:
//   - gctx: Current gin request context.
//   - user: Target user config to modify in place.
//   - logger: Request logger used for diagnostics.
func applyUserAPIBaseOverride(gctx *gin.Context, user *config.UserConfig, logger *glog.LoggerT) {
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
	switch {
	case strings.Contains(override, "openai.azure.com"):
		user.ImageUrl = override
	case strings.Contains(override, "api.openai.com"):
		user.ImageUrl = "https://api.openai.com/v1/images/generations"
	default:
		user.ImageUrl = override + "/v1/images/generations"
	}

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
