package http

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
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

func getUserFromToken(ctx *gin.Context) (user *config.UserConfig, err error) {
	userToken := strings.TrimPrefix(ctx.Request.Header.Get("Authorization"), "Bearer ")

SWITCH_FOR_USER:
	switch {
	case strings.HasPrefix(userToken, "FREETIER-"): // free user
		hasher := sha256.New()
		hasher.Write([]byte(userToken))
		username := hex.EncodeToString(hasher.Sum(nil))[:12]
		log.Logger.Debug("use server's freetier openai token",
			zap.String("token", userToken),
			zap.String("user", username))

		for _, commFreeUser := range config.Config.UserTokens {
			if commFreeUser.Token == config.FreetierUserToken {
				user = &config.UserConfig{}
				if err = copier.Copy(user, commFreeUser); err != nil {
					return nil, errors.Wrap(err, "copy free user")
				}

				user.UserName = "FREETIER-" + username
				break SWITCH_FOR_USER
			}
		}

		return nil, errors.Errorf("can not find freetier user %q in settings",
			config.FreetierUserToken)
	default: // use server's token in settings
		for _, u := range config.Config.UserTokens {
			if u.Token == userToken {
				log.Logger.Debug("paid user", zap.String("user", u.UserName))
				u.IsPaid = true
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
			UserName:               username,
			Token:                  userToken,
			OpenaiToken:            userToken,
			ImageToken:             userToken,
			ImageTokenType:         config.ImageTokenOpenai,
			AllowedModels:          []string{"*"},
			IsPaid:                 true,
			NoLimitExpensiveModels: true,
			NoLimitAllModels:       true,
			NoLimitImageModels:     true,
			APIBase:                "https://api.openai.com",
		}
	}

	if err = user.Valid(); err != nil {
		return nil, errors.Wrap(err, "valid user")
	}
	return user, nil
}
