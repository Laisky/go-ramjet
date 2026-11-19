package gptchat

import (
	"crypto/sha1" //nolint:gosec // Mirrors the legacy asset naming scheme; not a security use.
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
	ijs "github.com/Laisky/go-ramjet/internal/tasks/gptchat/templates/js"
)

// legacyAssetPath returns the URL path under which ihttp.RegisterStatic would
// serve an embedded legacy asset, using the same name-hash-ext scheme as
// prepareStaticFiles.
func legacyAssetPath(name, ext string, content []byte) string {
	hashed := sha1.Sum(content) //nolint:gosec // Mirrors the legacy asset naming scheme.
	return fmt.Sprintf("/gptchat/static/%s-%s%s", name, fmt.Sprintf("%x", hashed)[:7], ext)
}

// TestLegacyChatUINotServed verifies the gptchat routes serve neither the
// legacy HTML page nor its embedded scripts. The legacy chat.js still renders
// model output and other values through HTML sinks, which is acceptable only
// while the React SPA has replaced it and it stays unreachable.
func TestLegacyChatUINotServed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previous := config.Config
	config.Config = &config.OpenAI{}
	t.Cleanup(func() { config.Config = previous })

	router := gin.New()
	registerGPTChatRoutes(router.Group("/gptchat"), func(c *gin.Context) { c.Next() })

	for _, route := range router.Routes() {
		require.NotEqual(t, "/gptchat/", route.Path, "the legacy chat page must stay retired")
		require.False(t, strings.HasPrefix(route.Path, "/gptchat/static/"),
			"legacy static assets must stay retired: %s %s", route.Method, route.Path)
		require.False(t, strings.HasPrefix(route.Path, "/gptchat/payment/"),
			"legacy payment assets must stay retired: %s %s", route.Method, route.Path)
	}

	for _, path := range []string{
		"/gptchat/",
		legacyAssetPath("sites", ".js", ijs.Chat),
		legacyAssetPath("libs", ".js", ijs.Libs),
		legacyAssetPath("data", ".js", ijs.ChatPrompts),
		"/gptchat/payment/js",
	} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusNotFound, recorder.Code, path)
		require.NotContains(t, recorder.Body.String(), "function applyUrlConfigOverrides(", path)
	}
}
