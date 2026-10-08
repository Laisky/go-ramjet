package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestDeepResearchRetired verifies both legacy endpoints reject all requests without accessing authentication, Redis, or llm-storm.
func TestDeepResearchRetired(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/gptchat/deepresearch", CreateDeepResearchHandler)
	router.GET("/gptchat/deepresearch/:task_id", GetDeepResearchStatusHandler)
	for _, tc := range []struct{ name, method, path, body, token string }{
		{"create", http.MethodPost, "/gptchat/deepresearch", `{"prompt":"retirement regression"}`, ""},
		{"invalid JSON", http.MethodPost, "/gptchat/deepresearch", "{", ""},
		{"cached authenticated client", http.MethodPost, "/gptchat/deepresearch", `{"prompt":"retirement regression"}`, "Bearer synthetic-retired-token"},
		{"status", http.MethodGet, "/gptchat/deepresearch/historical-task", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Authorization", tc.token)
			require.NotPanics(t, func() { router.ServeHTTP(recorder, request) })
			require.Equal(t, http.StatusGone, recorder.Code)
			var result map[string]string
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &result))
			require.Equal(t, "deep_research_retired", result["code"])
			require.Contains(t, result["error"], "retired")
			require.NotContains(t, recorder.Body.String(), "synthetic-retired-token")
			require.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
		})
	}
}
