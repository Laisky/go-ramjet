package cv

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestServeCVRootAgentSurfaceKeepsBrowserSPA verifies normal browser root requests still reach the SPA.
// It takes a testing.T and returns no values.
func TestServeCVRootAgentSurfaceKeepsBrowserSPA(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(serveCVRootAgentSurface)
	router.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, "spa cv page")
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "cv.laisky.com"
	req.Header.Set("Accept", "text/html")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	resp := recorder.Result()
	t.Cleanup(func() {
		_ = resp.Body.Close()
	})

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "spa cv page", string(body))
}

// TestServeCVRootAgentSurfaceMarkdown verifies markdown agents can fetch a root fallback.
// It takes a testing.T and returns no values.
func TestServeCVRootAgentSurfaceMarkdown(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(serveCVRootAgentSurface)
	router.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, "spa cv page")
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "cv.laisky.com"
	req.Header.Set("Accept", "text/markdown")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	resp := recorder.Result()
	t.Cleanup(func() {
		_ = resp.Body.Close()
	})

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, resp.Header.Get("Content-Type"), "text/markdown")
	require.Contains(t, string(body), "# Zhonghua (Laisky) Cai")
}

// TestServeCVRootAgentSurfaceExplicitAgentMode verifies explicit agent mode returns crawlable HTML.
// It takes a testing.T and returns no values.
func TestServeCVRootAgentSurfaceExplicitAgentMode(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(serveCVRootAgentSurface)
	router.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, "spa cv page")
	})

	req := httptest.NewRequest(http.MethodGet, "/?mode=agent", nil)
	req.Host = "cv.laisky.com"
	req.Header.Set("Accept", "text/html")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	resp := recorder.Result()
	t.Cleanup(func() {
		_ = resp.Body.Close()
	})

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, resp.Header.Get("Content-Type"), "text/html")
	require.Contains(t, string(body), "Agent Mode Active")
	require.NotContains(t, string(body), "spa cv page")
}

// TestServeCVRobots verifies robots.txt advertises crawl access and discovery files.
// It takes a testing.T and returns no values.
func TestServeCVRobots(t *testing.T) {
	t.Parallel()

	ctx, recorder := newCVTestContext(http.MethodGet, "/robots.txt")

	serveCVRobots(ctx)

	resp := recorder.Result()
	t.Cleanup(func() {
		_ = resp.Body.Close()
	})

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, resp.Header.Get("Content-Type"), "text/plain")
	require.Contains(t, string(body), "Allow: /")
	require.Contains(t, string(body), "https://cv.laisky.com/llms.txt")
	require.Contains(t, string(body), "https://cv.laisky.com/sitemap.xml")
}

// TestServeCVLLMs verifies llms.txt gives agents the public CV and API surface.
// It takes a testing.T and returns no values.
func TestServeCVLLMs(t *testing.T) {
	t.Parallel()

	ctx, recorder := newCVTestContext(http.MethodGet, "/llms.txt")

	serveCVLLMs(ctx)

	resp := recorder.Result()
	t.Cleanup(func() {
		_ = resp.Body.Close()
	})

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, string(body), "Zhonghua (Laisky) Cai CV")
	require.Contains(t, string(body), "https://cv.laisky.com/cv/content")
	require.Contains(t, string(body), "https://cv.laisky.com/openapi.json")
	require.Contains(t, string(body), "https://mcp.laisky.com")
}

// TestServeCVSitemap verifies the sitemap exposes machine-readable CV targets.
// It takes a testing.T and returns no values.
func TestServeCVSitemap(t *testing.T) {
	t.Parallel()

	ctx, recorder := newCVTestContext(http.MethodGet, "/sitemap.xml")

	serveCVSitemap(ctx)

	resp := recorder.Result()
	t.Cleanup(func() {
		_ = resp.Body.Close()
	})

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, resp.Header.Get("Content-Type"), "application/xml")
	require.Contains(t, string(body), "<loc>https://cv.laisky.com/</loc>")
	require.Contains(t, string(body), "<loc>https://cv.laisky.com/openapi.json</loc>")
}

// TestServeCVOpenAPI verifies the OpenAPI document exposes public CV endpoints.
// It takes a testing.T and returns no values.
func TestServeCVOpenAPI(t *testing.T) {
	t.Parallel()

	ctx, recorder := newCVTestContext(http.MethodGet, "/openapi.json")

	serveCVOpenAPI(ctx)

	resp := recorder.Result()
	t.Cleanup(func() {
		_ = resp.Body.Close()
	})

	var payload map[string]any
	err := json.NewDecoder(resp.Body).Decode(&payload)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "3.1.0", payload["openapi"])

	paths, ok := payload["paths"].(map[string]any)
	require.True(t, ok)
	require.Contains(t, paths, "/cv/content")
	require.Contains(t, paths, "/cv/pdf")
	require.Contains(t, paths, "/cv/meta")
	require.Contains(t, paths, "/api/v1/batch")
	require.Contains(t, paths, "/api/v1/sandbox/cv")
	require.Contains(t, paths, "/api/v1/webhooks")
	require.Contains(t, paths, "/ask")

	components, ok := payload["components"].(map[string]any)
	require.True(t, ok)
	schemas, ok := components["schemas"].(map[string]any)
	require.True(t, ok)
	require.Contains(t, schemas, "NLWebAnswer")
	require.Contains(t, schemas, "WebhookMetadata")
}

// TestServeCVAICatalog verifies agent catalog metadata links docs, APIs, and MCP.
// It takes a testing.T and returns no values.
func TestServeCVAICatalog(t *testing.T) {
	t.Parallel()

	ctx, recorder := newCVTestContext(http.MethodGet, "/.well-known/ai-catalog.json")

	serveCVAICatalog(ctx)

	resp := recorder.Result()
	t.Cleanup(func() {
		_ = resp.Body.Close()
	})

	var payload map[string]any
	err := json.NewDecoder(resp.Body).Decode(&payload)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "Zhonghua (Laisky) Cai CV", payload["name"])
	require.Equal(t, "0.1", payload["specVersion"])
	entries, ok := payload["entries"].([]any)
	require.True(t, ok)
	require.Len(t, entries, 3)
}

// TestServeCVAPICatalog verifies the API catalog uses linkset JSON for RFC 9727 discovery.
// It takes a testing.T and returns no values.
func TestServeCVAPICatalog(t *testing.T) {
	t.Parallel()

	ctx, recorder := newCVTestContext(http.MethodGet, "/.well-known/api-catalog")

	serveCVAPICatalog(ctx)

	resp := recorder.Result()
	t.Cleanup(func() {
		_ = resp.Body.Close()
	})

	var payload map[string]any
	err := json.NewDecoder(resp.Body).Decode(&payload)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, resp.Header.Get("Content-Type"), "application/linkset+json")
	require.Contains(t, payload, "linkset")
}

// TestServeCVProtocolDocs verifies CLI, webhook, and sandbox docs are crawlable.
// It takes a testing.T and returns no values.
func TestServeCVProtocolDocs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		path     string
		handler  gin.HandlerFunc
		contains string
	}{
		{name: "cli", path: "/cli.md", handler: serveCVCLIDocs, contains: "curl -fsSL https://cv.laisky.com/api/v1/cv"},
		{name: "webhooks", path: "/webhooks.md", handler: serveCVWebhookMarkdown, contains: "X-CV-Signature"},
		{name: "sandbox", path: "/sandbox", handler: serveCVSandboxDocs, contains: "api/v1/sandbox/cv"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx, recorder := newCVTestContext(http.MethodGet, tt.path)

			tt.handler(ctx)

			resp := recorder.Result()
			t.Cleanup(func() {
				_ = resp.Body.Close()
			})

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, resp.StatusCode)
			require.Contains(t, string(body), tt.contains)
		})
	}
}

// TestServeCVHTTPSignatureDirectory verifies Web Bot Auth key lifetimes are numeric.
// It takes a testing.T and returns no values.
func TestServeCVHTTPSignatureDirectory(t *testing.T) {
	t.Parallel()

	ctx, recorder := newCVTestContext(http.MethodGet, "/.well-known/http-message-signatures-directory")

	serveCVHTTPSignatureDirectory(ctx)

	resp := recorder.Result()
	t.Cleanup(func() {
		_ = resp.Body.Close()
	})

	var payload map[string]any
	err := json.NewDecoder(resp.Body).Decode(&payload)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	keys, ok := payload["keys"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, keys)
	key, ok := keys[0].(map[string]any)
	require.True(t, ok)
	require.IsType(t, float64(0), key["nbf"])
	require.IsType(t, float64(0), key["exp"])
}

// TestServeCVMCPMetadata verifies MCP discovery metadata includes the public server.
// It takes a testing.T and returns no values.
func TestServeCVMCPMetadata(t *testing.T) {
	t.Parallel()

	ctx, recorder := newCVTestContext(http.MethodGet, "/.well-known/mcp.json")

	serveCVMCPMetadata(ctx)

	resp := recorder.Result()
	t.Cleanup(func() {
		_ = resp.Body.Close()
	})

	var payload map[string]any
	err := json.NewDecoder(resp.Body).Decode(&payload)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, cvPublicMCPServer, payload["url"])
	require.Equal(t, "streamable-http", payload["transport"])
}
