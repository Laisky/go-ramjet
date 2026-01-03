package gptchat

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestRegisterFaviconRoutes verifies that the embedded favicon is served correctly.
func TestRegisterFaviconRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	grp := r.Group("/gptchat")
	registerFaviconRoutes(grp)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/gptchat/favicon.ico", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.NotEmpty(t, w.Body.Bytes())
	require.Equal(t, "image/x-icon", w.Header().Get("Content-Type"))
}
