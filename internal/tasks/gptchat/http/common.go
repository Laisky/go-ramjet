package http

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	gutils "github.com/Laisky/go-utils/v4"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

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

func getUserFromToken(ctx *gin.Context) (*config.UserConfig, error) {
	userToken := strings.TrimPrefix(ctx.Request.Header.Get("Authorization"), "Bearer ")

	switch {
	case strings.HasPrefix(userToken, "FREETIER-"): // free user
		hasher := sha256.New()
		hasher.Write([]byte(userToken))
		username := hex.EncodeToString(hasher.Sum(nil))[:12]
		log.Logger.Debug("use server's freetier openai token",
			zap.String("token", userToken),
			zap.String("user", username))
		for _, u := range config.Config.UserTokens {
			if u.Token == config.FREETIER_USER_TOKEN {
				return &config.UserConfig{
					UserName:    "FREETIER-" + username,
					Token:       userToken,
					OpenaiToken: config.Config.Token,
					ImageToken:  config.Config.DefaultImageToken,
					ImageTokenType: gutils.OptionalVal(&u.ImageTokenType,
						config.ImageTokenType(config.Config.DefaultImageToken)),
					AllowedModels: u.AllowedModels,
					APIBase:       strings.TrimRight(config.Config.API, "/"),
				}, nil
			}
		}

		return nil, errors.Errorf("can not find freetier user %q in settings",
			config.FREETIER_USER_TOKEN)
	default: // use server's token in settings
		for _, u := range config.Config.UserTokens {
			if u.Token == userToken {
				log.Logger.Debug("paid user", zap.String("user", u.UserName))
				u.IsPaid = true

				// set default value
				u.OpenaiToken = gutils.OptionalVal(&u.OpenaiToken, config.Config.Token)
				u.ImageToken = gutils.OptionalVal(&u.ImageToken, config.Config.DefaultImageToken)
				u.ImageTokenType = gutils.OptionalVal(&u.ImageTokenType, config.Config.DefaultImageTokenType)
				u.APIBase = strings.TrimRight(gutils.OptionalVal(&u.APIBase, config.Config.API), "/")

				return &u, nil
			}
		}

		// use user's own openai/azure or whatever token
		hashed := sha256.Sum256([]byte(userToken))
		username := hex.EncodeToString(hashed[:])[:16]
		log.Logger.Debug("use user's own token", zap.String("user", username))
		u := &config.UserConfig{
			UserName:               username,
			Token:                  userToken,
			OpenaiToken:            userToken,
			ImageToken:             userToken,
			ImageTokenType:         config.Config.DefaultImageTokenType,
			AllowedModels:          []string{"*"},
			IsPaid:                 true,
			NoLimitExpensiveModels: true,
			NoLimitAllModels:       true,
			NoLimitImageModels:     true,
			APIBase:                "https://api.openai.com",
		}
		if strings.HasPrefix(userToken, "sk-") {
			u.ImageTokenType = config.ImageTokenOpenai
		}

		return u, nil
	}
}
