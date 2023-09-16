package http

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
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
	case strings.HasPrefix(userToken, "FREETIER-"): // use server's default openai token
		hasher := sha256.New()
		hasher.Write([]byte(userToken))
		username := hex.EncodeToString(hasher.Sum(nil))[:16]
		log.Logger.Debug("use server's freetier openai token",
			zap.String("token", userToken),
			zap.String("user", username))
		for _, u := range config.Config.UserTokens {
			if u.Token == config.FREETIER_USER_TOKEN {
				return &config.UserConfig{
					UserName:             username,
					Token:                userToken,
					OpenaiToken:          config.Config.Token,
					AllowedModels:        u.AllowedModels,
					LimitExpensiveModels: u.LimitExpensiveModels,
				}, nil
			}
		}

		return nil, errors.Errorf("can not find freetier user %q in settings",
			config.FREETIER_USER_TOKEN)
	default: // use server's token in settings
		for _, u := range config.Config.UserTokens {
			if u.Token == userToken {
				log.Logger.Debug("use server's default openai token",
					zap.String("user", u.UserName))
				u.IsPaid = true
				u.OpenaiToken = config.Config.Token // use server's default openai token
				return &u, nil
			}
		}

		// use user's own openai/azure or whatever token
		hashed := sha256.Sum256([]byte(userToken))
		username := hex.EncodeToString(hashed[:])[:16]
		log.Logger.Debug("use user's own token", zap.String("user", username))
		return &config.UserConfig{
			UserName:      username,
			Token:         userToken,
			OpenaiToken:   userToken,
			AllowedModels: []string{"*"},
			IsPaid:        true,
		}, nil
	}
}
