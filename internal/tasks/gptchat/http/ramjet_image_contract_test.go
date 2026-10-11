package http

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gconfig "github.com/Laisky/go-config/v2"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
)

// TestRamjetBYOKImageCapabilityPassthrough keeps image selection opaque to the gateway and preserves the public Python response.
func TestRamjetBYOKImageCapabilityPassthrough(t *testing.T) {
	originalBackend := gconfig.Shared.GetString("openai.rate_limiter_backend")
	gconfig.Shared.Set("openai.rate_limiter_backend", "legacy")
	t.Cleanup(func() { gconfig.Shared.Set("openai.rate_limiter_backend", originalBackend) })
	const key = "sk-SYNTHETIC-IMAGE-PROFILE-CALLER-KEY"
	const successBody = `{"task_id":"synthetic-image-task","image_url":["https://storage.fixture/images/task.png"]}`
	const failureBody = `{"error":"A supported image model and profile are required"}`
	for _, tc := range []struct {
		name, base, payload, response string
		status                        int
	}{
		{"official_default", "https://api.openai.com/v1", `{"prompt":"synthetic image"}`, successBody, http.StatusOK},
		{"official_pinned_gpt", "https://api.openai.com/v1", `{"prompt":"synthetic image","model":"gpt-image-2-2026-04-21","image_profile":"gpt-image"}`, successBody, http.StatusOK},
		{"custom_known_legacy", "http://100.64.0.10:3000/provider/v1", `{"prompt":"synthetic image","model":"dall-e-2"}`, successBody, http.StatusOK},
		{"custom_declared_gpt", "http://100.64.0.10:3000/provider/v1?tenant=synthetic&flag=", `{"prompt":"synthetic image","model":"custom-image-model","image_profile":"gpt-image"}`, successBody, http.StatusOK},
		{"custom_declared_legacy", "http://100.64.0.10:3000/provider/v1", `{"prompt":"synthetic image","model":"custom-image-model","image_profile":"legacy"}`, successBody, http.StatusOK},
		{"custom_missing_profile", "http://100.64.0.10:3000/provider/v1", `{"prompt":"synthetic image","model":"custom-image-model"}`, failureBody, http.StatusBadRequest},
		{"official_retired_model", "https://api.openai.com/v1", `{"prompt":"synthetic image","model":"dall-e-3","image_profile":"legacy"}`, failureBody, http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			byokAuditSetup(t)
			config.Config.RamjetURL = "http://ramjet.test"
			config.Config.RateLimitExpensiveModelsIntervalSeconds = 60
			original := newAuthContext(key)
			original.Request.Method = http.MethodPost
			original.Request.URL.Path = "/gptchat/ramjet/gptchat/image/dalle"
			original.Request.Header.Set("Content-Type", "application/json")
			original.Request.Header.Set("X-Laisky-Api-Base", tc.base)
			original.Request.Body = io.NopCloser(strings.NewReader(tc.payload))
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = original.Request
			ctx.Set(ctxKeyUser, &config.UserConfig{BYOK: true, Token: key, OpenaiToken: key, ImageToken: key, UserName: key[:15], APIBase: tc.base, AllowedModels: []string{"*"}})
			calls := 0
			httpcli = &http.Client{Transport: byokAuditTransport(func(req *http.Request) (*http.Response, error) {
				calls++
				require.Equal(t, "/gptchat/image/dalle", req.URL.Path)
				require.Equal(t, key, req.Header.Get("Authorization"))
				require.Equal(t, tc.base, req.Header.Get("X-Laisky-Openai-Api-Base"))
				require.Equal(t, key[:15], req.Header.Get("X-Laisky-User-Id"))
				payload, err := io.ReadAll(req.Body)
				require.NoError(t, err)
				require.Equal(t, tc.payload, string(payload))
				response := byokAuditResponse(req, tc.response)
				response.StatusCode = tc.status
				return response, nil
			})}
			RamjetProxyHandler(ctx)
			require.Equal(t, 1, calls)
			require.Equal(t, tc.status, recorder.Code)
			require.Equal(t, tc.response, recorder.Body.String())
			require.Equal(t, "application/json", recorder.Header().Get("Content-Type"))
		})
	}
}
