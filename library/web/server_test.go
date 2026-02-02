package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestNormalizeHandlerDuplicatePrefix verifies duplicated task prefixes are normalized.
func TestNormalizeHandlerDuplicatePrefix(t *testing.T) {
	var gotPath string
	h := &normalizeHandler{handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	})}

	req := httptest.NewRequest(http.MethodGet, "http://example.com/gptchat/gptchat/favicon.ico", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "/gptchat/favicon.ico", gotPath)
}

func TestSecurityMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(securityMiddleware)
	r.GET("/test", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
	require.Equal(t, "SAMEORIGIN", w.Header().Get("X-Frame-Options"))
	require.Equal(t, "strict-origin-when-cross-origin", w.Header().Get("Referrer-Policy"))
}
