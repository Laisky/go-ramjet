package http

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/Laisky/errors/v2"
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
	ctx.AbortWithStatusJSON(http.StatusBadRequest, err.Error())
	return true
}

func getUserByAuthHeader(gctx *gin.Context) (user *config.UserConfig, err error) {
	userToken := strings.TrimPrefix(gctx.Request.Header.Get("Authorization"), "Bearer ")
	if userToken == "" {
		return nil, errors.New("authorization token is empty")
	}

	return getUserByToken(gctx.Request.Context(), userToken)
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

func getUserByToken(ctx context.Context, userToken string) (user *config.UserConfig, err error) {
	userToken = strings.TrimSpace(strings.TrimPrefix(userToken, "Bearer "))
	if userToken == "" {
		return nil, errors.New("empty token")
	}

SWITCH_FOR_USER:
	switch {
	case strings.HasPrefix(userToken, "FREETIER-"): // free user
		if len(userToken) < 15 {
			return nil, errors.Errorf("invalid freetier token %q", userToken)
		}

		username := userToken[:15]
		log.Logger.Debug("use server's freetier openai token",
			zap.String("token", userToken),
			zap.String("user", username))

		for _, commFreeUser := range config.Config.UserTokens {
			if commFreeUser.Token == config.FreetierUserToken {
				user = &config.UserConfig{
					IsFree: true,
				}
				if err = copier.Copy(user, commFreeUser); err != nil {
					return nil, errors.Wrap(err, "copy free user")
				}

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
		log.Logger.Debug("use laisky's oneapi token", zap.String("user", username))
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
		}

		if oneapiUid, err := getOneapiUserIDByToken(ctx, userToken); err != nil {
			log.Logger.Error("get oneapi uid", zap.Error(err))
			user.EnableExternalImageBilling = false
			user.NoLimitImageModels = false
		} else {
			log.Logger.Debug("get oneapi uid", zap.String("uid", oneapiUid))
			user.EnableExternalImageBilling = true
			user.ExternalImageBillingUID = oneapiUid
			user.NoLimitImageModels = true
		}
	default: // use server's token in settings
		for _, u := range config.Config.UserTokens {
			if u.Token == userToken {
				log.Logger.Debug("paid user", zap.String("user", u.UserName))
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
		log.Logger.Debug("use user's own token", zap.String("user", username))
		user = &config.UserConfig{ // default to openai user
			UserName:    username,
			Token:       userToken,
			OpenaiToken: userToken,
			ImageToken:  userToken,
			// ImageTokenType:         config.ImageTokenOpenai,
			AllowedModels:          []string{"*"},
			NoLimitExpensiveModels: true,
			NoLimitOpenaiModels:    true,
			NoLimitImageModels:     true,
			BYOK:                   true,
			APIBase:                config.Config.API,
		}
	}

	if err = user.Valid(); err != nil {
		return nil, errors.Wrap(err, "valid user")
	}
	return user, nil
}
