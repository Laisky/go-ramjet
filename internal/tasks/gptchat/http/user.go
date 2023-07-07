package http

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
	"github.com/Laisky/go-ramjet/library/log"
)

func getUser(ctx *gin.Context) (*config.UserConfig, error) {
	userToken := strings.TrimPrefix(ctx.Request.Header.Get("Authorization"), "Bearer ")

	if strings.HasPrefix(userToken, "sk-") {
		hasher := sha256.New()
		hasher.Write([]byte(userToken))
		username := hex.EncodeToString(hasher.Sum(nil))[:16]
		log.Logger.Debug("use user's own openai token", zap.String("user", username))
		return &config.UserConfig{
			UserName:      username,
			Token:         userToken,
			OpenaiToken:   userToken,
			AllowedModels: []string{"*"},
		}, nil
	}

	for _, u := range config.Config.UserTokens {
		if u.Token == userToken {
			log.Logger.Debug("use server's default openai token", zap.String("user", u.UserName))
			u.OpenaiToken = config.Config.Token // use server's default openai token
			return &u, nil
		}
	}

	return nil, nil
}

func GetCurrentUser(ctx *gin.Context) {

}
