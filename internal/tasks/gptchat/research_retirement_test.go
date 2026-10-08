package gptchat

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
)

// TestResearchRetirementRoutes verifies production registration bypasses rate limiting for tombstones while keeping active API routes protected.
func TestResearchRetirementRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previous := config.Config
	config.Config = &config.OpenAI{}
	t.Cleanup(func() { config.Config = previous })
	router := gin.New()
	limited := 0
	registerGPTChatRoutes(router.Group("/gptchat"), func(c *gin.Context) {
		limited++
		c.AbortWithStatus(http.StatusTeapot)
	})
	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, "/gptchat/deepresearch"},
		{http.MethodGet, "/gptchat/deepresearch/stored-task"},
	} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{"prompt":"synthetic"}`)))
		require.Equal(t, http.StatusGone, recorder.Code, tc.path)
		require.Contains(t, recorder.Body.String(), "deep_research_retired")
	}
	require.Zero(t, limited, "retirement responses must not depend on Redis rate limiting")
	for _, path := range []string{"/gptchat/api", "/gptchat/images/generations", "/gptchat/files/chat", "/gptchat/chat/oneshot", "/gptchat/ramjet/test", "/gptchat/user/config"} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, path, nil))
		require.Equal(t, http.StatusTeapot, recorder.Code, path)
	}
	require.Equal(t, 6, limited, "active API routes must retain rate limiting")
}
