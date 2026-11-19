package gptchat

import (
	"crypto/sha1" //nolint:gosec // Mirrors the legacy asset naming scheme; not a security use.
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v87"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
	ihttp "github.com/Laisky/go-ramjet/internal/tasks/gptchat/http"
	ijs "github.com/Laisky/go-ramjet/internal/tasks/gptchat/templates/js"
	"github.com/Laisky/go-ramjet/library/web"
)

// legacyAssetPath returns the URL path under which ihttp.RegisterStatic would
// serve an embedded legacy asset below /gptchat/static, using the same
// name-hash-ext scheme as prepareStaticFiles.
func legacyAssetPath(name, ext string, content []byte) string {
	hashed := sha1.Sum(content) //nolint:gosec // Mirrors the legacy asset naming scheme.
	return fmt.Sprintf("/gptchat/static/%s-%s%s", name, fmt.Sprintf("%x", hashed)[:7], ext)
}

// funcName returns the fully qualified runtime name of fn, which is how gin
// reports a route's handler in RouteInfo.Handler.
func funcName(fn any) string {
	return runtime.FuncForPC(reflect.ValueOf(fn).Pointer()).Name()
}

// bindProductionRoutes runs bindHTTP, exactly as the gptchat task does, against
// a fresh engine swapped in for web.Server, and returns that engine.
func bindProductionRoutes(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	t.Setenv("HTTP_PROXY", "")

	previousConfig, previousServer, previousStripeKey := config.Config, web.Server, stripe.Key
	config.Config = &config.OpenAI{}
	web.Server = gin.New()
	t.Cleanup(func() {
		config.Config, web.Server, stripe.Key = previousConfig, previousServer, previousStripeKey
	})

	bindHTTP()
	return web.Server
}

// TestLegacyChatUINotServed verifies the production gptchat routes serve
// neither the legacy HTML page nor its embedded scripts. The legacy chat.js
// still renders model output and other values through HTML sinks, which is
// acceptable only while the React SPA has replaced it and it stays unreachable.
func TestLegacyChatUINotServed(t *testing.T) {
	router := bindProductionRoutes(t)

	legacyPage := funcName(ihttp.Chat)
	legacyAssets := funcName(ihttp.RegisterStatic) + "."
	routes := router.Routes()
	require.NotEmpty(t, routes, "bindHTTP must register the gptchat API")
	for _, route := range routes {
		require.NotEqual(t, legacyPage, route.Handler,
			"the legacy chat page must stay retired: %s %s", route.Method, route.Path)
		require.False(t, strings.HasPrefix(route.Handler, legacyAssets),
			"legacy static assets must stay retired: %s %s", route.Method, route.Path)
		require.NotEqual(t, "/gptchat/", route.Path, "the legacy chat page must stay retired")
		require.False(t, strings.HasPrefix(route.Path, "/gptchat/static/"),
			"legacy static assets must stay retired: %s %s", route.Method, route.Path)
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
