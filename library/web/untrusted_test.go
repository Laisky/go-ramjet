package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestMarkUntrustedContentOverridesDefaultPolicy keeps relayed content sandboxed
// even behind the default security middleware, whose framing policy it retains.
func TestMarkUntrustedContentOverridesDefaultPolicy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(securityMiddleware)
	r.GET("/relay", func(c *gin.Context) {
		MarkUntrustedContent(c)
		c.String(http.StatusOK, "<script>alert(1)</script>")
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/relay", nil))

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, []string{UntrustedContentPolicy}, w.Header().Values("Content-Security-Policy"))
	require.Contains(t, UntrustedContentPolicy, "sandbox")
	require.NotContains(t, UntrustedContentPolicy, "allow-same-origin")
	require.NotContains(t, UntrustedContentPolicy, "allow-scripts")
	require.Contains(t, UntrustedContentPolicy, "frame-ancestors 'self'")
	require.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
}
