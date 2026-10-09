package http

import (
	"crypto/subtle"
	"strings"
	"unicode"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
)

// resolveRamjetUser resolves original caller-owned credentials without accepting free-tier or server-key substitution.
func resolveRamjetUser(ctx *gin.Context) (*config.UserConfig, error) {
	if ctx == nil || ctx.Request == nil {
		return nil, errors.New("Ramjet requires a caller-provided API key")
	}
	key := GetRawUserToken(ctx)
	headerKey := strings.TrimPrefix(ctx.Request.Header.Get("Authorization"), "Bearer ")
	if subtle.ConstantTimeCompare([]byte(headerKey), []byte(key)) != 1 || key == "" || key == config.FreetierUserToken || strings.HasPrefix(key, "FREETIER-") ||
		strings.IndexFunc(key, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
		return nil, errors.New("Ramjet requires a caller-provided API key")
	}
	user, err := getUserByAuthHeader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "resolve Ramjet caller credentials")
	}
	if !user.BYOK || subtle.ConstantTimeCompare([]byte(user.Token), []byte(key)) != 1 ||
		subtle.ConstantTimeCompare([]byte(user.OpenaiToken), []byte(key)) != 1 {
		return nil, errors.New("Ramjet requires matching caller-owned credentials")
	}
	return user, nil
}
