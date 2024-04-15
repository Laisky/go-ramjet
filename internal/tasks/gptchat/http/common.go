package http

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v5"
	gutils "github.com/Laisky/go-utils/v4"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
	"github.com/jinzhu/copier"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
	"github.com/Laisky/go-ramjet/library/log"
)

// AbortErr abort with error
func AbortErr(ctx *gin.Context, err error) bool {
	if err == nil {
		return false
	}

	log.Logger.Error("openai chat abort", zap.Error(err))
	ctx.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
		"err": err.Error(),
	})
	return true
}

func getUserByAuthHeader(gctx *gin.Context) (user *config.UserConfig, err error) {
	userToken := strings.TrimPrefix(gctx.Request.Header.Get("Authorization"), "Bearer ")
	if userToken == "" {
		log.Logger.Debug("user token not found in header, use freetier token instead")
		userToken = config.FreetierUserToken
	}

	return getUserByToken(gctx, userToken)
}

type oneapiUserResponse struct {
	TokenID  int    `json:"token_id"`
	UID      int    `json:"uid"`
	Username string `json:"username"`
}

var (
	cacheGetOneapiUserIDByToken sync.Map
)

func getOneapiUserIDByToken(ctx context.Context, token string) (uid string, err error) {
	// load from cache
	if v, ok := cacheGetOneapiUserIDByToken.Load(token); ok {
		return v.(string), nil //nolint: forcetypeassert
	}

	url := config.Config.ExternalBillingAPI + "/api/user/get-by-token"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", errors.Wrap(err, "new request")
	}

	req.Header.Add("Authorization", token)
	resp, err := httpcli.Do(req) //nolint: bodyclose
	if err != nil {
		return "", errors.Wrap(err, "do request")
	}
	defer gutils.LogErr(resp.Body.Close, log.Logger)
	if resp.StatusCode != http.StatusOK {
		return "", errors.Errorf("bad status code %d", resp.StatusCode)
	}

	var respData oneapiUserResponse
	if err = json.NewDecoder(resp.Body).Decode(&respData); err != nil {
		return "", errors.Wrap(err, "decode response")
	}

	uid = strconv.Itoa(respData.UID)
	cacheGetOneapiUserIDByToken.Store(token, uid)
	return uid, nil
}

func getUserByToken(gctx *gin.Context, userToken string) (user *config.UserConfig, err error) {
	logger := gmw.GetLogger(gctx).Named("get_user_by_token")
	ctx, cancel := context.WithTimeout(gmw.Ctx(gctx), time.Second*10)
	defer cancel()

	userToken = strings.TrimSpace(strings.TrimPrefix(userToken, "Bearer "))
	if userToken == "" {
		return nil, errors.New("empty token")
	}

SWITCH_FOR_USER:
	switch {
	case strings.HasPrefix(userToken, "FREETIER-"),
		userToken == config.FreetierUserToken: // free user
		if len(userToken) < 15 {
			return nil, errors.Errorf("invalid freetier token %q", userToken)
		}

		username := userToken[:15]
		logger.Debug("use server's freetier openai token",
			zap.String("token", userToken),
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
	case strings.HasPrefix(userToken, "laisky-"):
		if len(userToken) < 15 {
			return nil, errors.Errorf("invalid laisky's oneapi token %q", userToken)
		}

		username := userToken[:15]
		logger.Debug("use laisky's oneapi token", zap.String("user", username))
		user = &config.UserConfig{ // default to openai user
			UserName:    username,
			Token:       userToken,
			OpenaiToken: userToken,
			// ImageToken:             config.Config.DefaultImageToken,
			// ImageTokenType:         config.Config.DefaultImageTokenType,
			// ImageUrl:               config.Config.DefaultImageUrl,
			AllowedModels:          []string{"*"},
			NoLimitExpensiveModels: true,
			APIBase:                "https://oneapi.laisky.com",
			// APIBase:  "http://100.97.108.34:3000",
			ImageUrl: "https://oneapi.laisky.com/v1/images/generations",
		}

		if strings.Contains(user.ImageUrl, "https://oneapi.laisky.com") {
			// billing by oneapi, no need to enable external billing
			user.EnableExternalImageBilling = false
			user.NoLimitImageModels = true
		} else {
			if oneapiUid, err := getOneapiUserIDByToken(ctx, userToken); err != nil {
				logger.Error("get oneapi uid", zap.Error(err))
				user.EnableExternalImageBilling = false
				user.NoLimitImageModels = false
			} else {
				logger.Debug("get oneapi uid", zap.String("uid", oneapiUid))
				user.EnableExternalImageBilling = true
				user.ExternalImageBillingUID = oneapiUid
				user.NoLimitImageModels = true
			}
		}
	default: // use server's token in settings
		for _, u := range config.Config.UserTokens {
			if u.Token == userToken {
				logger.Debug("paid user", zap.String("user", u.UserName))
				if err = u.Valid(); err != nil {
					return nil, errors.Wrap(err, "valid paid user")
				}

				user = u
				break SWITCH_FOR_USER
			}
		}

		// use user's own openai/azure or whatever token
		hashed := sha256.Sum256([]byte(userToken))
		username := hex.EncodeToString(hashed[:])[:16]
		logger.Debug("use user's own token", zap.String("user", username))
		user = &config.UserConfig{ // default to openai user
			UserName:               username,
			Token:                  userToken,
			OpenaiToken:            userToken,
			ImageToken:             userToken,
			ImageUrl:               "https://api.openai.com/v1/images/generations",
			AllowedModels:          []string{"*"},
			NoLimitExpensiveModels: true,
			NoLimitOpenaiModels:    true,
			NoLimitImageModels:     true,
			BYOK:                   true,
			APIBase:                config.Config.API,
		}

		// only BYOK user can set api base
		userApiBase := strings.TrimRight(gctx.Request.Header.Get("X-Laisky-Api-Base"), "/")
		if userApiBase != "" {
			user.APIBase = userApiBase

			// set image url
			switch {
			case strings.Contains(userApiBase, "openai.azure.com"):
				user.ImageUrl = userApiBase
			case strings.Contains(userApiBase, "api.openai.com"):
				user.ImageUrl = "https://api.openai.com/v1/images/generations"
			default:
				user.ImageUrl = fmt.Sprintf("%s/v1/images/generations", userApiBase)
			}

			logger.Debug("use user's own api base", zap.String("api_base", user.APIBase))
		}
	}

	if err = user.Valid(); err != nil {
		return nil, errors.Wrap(err, "valid user")
	}

	return user, nil
}
