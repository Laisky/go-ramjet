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
	"github.com/Laisky/go-ramjet/library/ssosession"
)

// resolveRamjetUser resolves original caller-owned credentials without accepting free-tier or server-key substitution.
func resolveRamjetUser(ctx *gin.Context) (*config.UserConfig, error) {
	if ctx == nil || ctx.Request == nil {
		return nil, errors.New("Ramjet requires a caller-provided API key")
	}
	key := GetRawUserToken(ctx)
	if ssosession.HasLaiskyIssuer(key) {
		return nil, errors.New("An SSO session cannot be used as a model API key")
	}
	if session, ok := ssosession.BearerToken(ctx.Request.Header.Get(ramjetSessionHeader)); ok && subtle.ConstantTimeCompare([]byte(key), []byte(session)) == 1 {
		return nil, errors.New("Account sessions and model API keys must be separate")
	}
	headerKey := strings.TrimPrefix(ctx.Request.Header.Get("Authorization"), "Bearer ")
	if subtle.ConstantTimeCompare([]byte(headerKey), []byte(key)) != 1 || key == "" || key == config.FreetierUserToken || strings.HasPrefix(key, "FREETIER-") ||
		strings.IndexFunc(key, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) || r > 126 }) >= 0 {
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
	// BYOK supplies models only; it confers no account or storage ownership.
	resolved.UserName = "public"
	if selectedBase != "" {
		resolved.APIBase = selectedBase
	}
	if resolved.APIBase != "" {
		if err := validateRamjetAPIBase(resolved.APIBase); err != nil {
			return nil, errors.Wrap(err, "resolve Ramjet provider")
		}
		parsed, err := url.Parse(resolved.APIBase)
		if err != nil {
			return nil, errors.New("A valid model provider URL is required")
		}
		if original, ok := ssosession.BearerToken(ctx.Request.Header.Get(ramjetSessionHeader)); ok && modelURLContainsSession(parsed, original) {
			return nil, errors.New("Account sessions cannot be supplied in model provider URLs")
		}
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
	if modelURLContainsSession(parsed, "") {
		return errors.New("Account sessions cannot be supplied in model provider URLs")
	}
	if value := parsed.Port(); value != "" {
		port, err := strconv.Atoi(value)
		if err != nil || port < 0 || port > 65535 {
			return invalid()
		}
	}
	return nil
}

// modelURLContainsSession inspects only URL routing components for identifiable
// Laisky sessions or the separately supplied original session. It does not
// restrict destinations, unrelated provider JWTs, prompts or request content.
func modelURLContainsSession(parsed *url.URL, original string) bool {
	components := []string{parsed.Fragment}
	components = append(components, strings.Split(parsed.Path, "/")...)
	if parsed.User != nil {
		components = append(components, parsed.User.Username())
		if password, ok := parsed.User.Password(); ok {
			components = append(components, password)
		}
	}
	for name, values := range parsed.Query() {
		components = append(components, name)
		components = append(components, values...)
	}
	for _, component := range components {
		if original != "" && strings.Contains(component, original) {
			return true
		}
		if ssosession.HasLaiskyIssuer(component) {
			return true
		}
		for _, field := range strings.Fields(component) {
			if ssosession.HasLaiskyIssuer(field) {
				return true
			}
		}
	}
	return false
}
