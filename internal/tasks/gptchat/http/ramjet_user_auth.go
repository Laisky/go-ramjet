package http

import (
	"crypto/subtle"
	"net/url"
	"strconv"
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
	selectedBase := ctx.Request.Header.Get("X-Laisky-Api-Base")
	if selectedBase != "" {
		if err := validateRamjetAPIBase(selectedBase); err != nil {
			return nil, errors.Wrap(err, "resolve Ramjet provider")
		}
	}
	user, err := getUserByAuthHeader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "resolve Ramjet caller credentials")
	}
	if !user.BYOK || subtle.ConstantTimeCompare([]byte(user.Token), []byte(key)) != 1 ||
		subtle.ConstantTimeCompare([]byte(user.OpenaiToken), []byte(key)) != 1 {
		return nil, errors.New("Ramjet requires matching caller-owned credentials")
	}
	resolved := *user
	if selectedBase != "" {
		resolved.APIBase = selectedBase
	}
	return &resolved, nil
}

// validateRamjetAPIBase accepts explicit HTTP provider syntax without destination restrictions or credential-bearing errors.
func validateRamjetAPIBase(raw string) error {
	invalid := func() error { return errors.New("A valid model provider URL is required") }
	if strings.IndexFunc(raw, func(r rune) bool { return r < 33 }) >= 0 {
		return invalid()
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return invalid()
	}
	if (!strings.EqualFold(parsed.Scheme, "http") && !strings.EqualFold(parsed.Scheme, "https")) || parsed.Hostname() == "" {
		return invalid()
	}
	if value := parsed.Port(); value != "" {
		port, err := strconv.Atoi(value)
		if err != nil || port < 0 || port > 65535 {
			return invalid()
		}
	}
	return nil
}
