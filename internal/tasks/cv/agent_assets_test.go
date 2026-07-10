package cv

import (
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
}
