package http

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	gconfig "github.com/Laisky/go-config/v2"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
)

// TestRamjetBYOKRequiredBeforeProxyDispatch rejects requests that would previously spend server-owned credentials.
func TestRamjetBYOKRequiredBeforeProxyDispatch(t *testing.T) {
	for _, key := range []string{"", "FREETIER-SYNTHETIC-CLIENT-0123456789", config.FreetierUserToken, "sk-SHORT", "sk-SYNTHETIC WITH SPACE", " sk-SYNTHETIC-LEADING-SPACE", "sk-SYNTHETIC-TRAILING-SPACE "} {
		t.Run(key, func(t *testing.T) {
			byokAuditSetup(t)
			original := newAuthContext(key)
			if key == "" {
				original.Request.Header.Del("Authorization")
			}
			original.Request.URL.Path = "/gptchat/ramjet/gptchat/query/chunks"
			original.Request.Body = io.NopCloser(strings.NewReader("{}"))
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = original.Request
			calls := 0
			httpcli = &http.Client{Transport: byokAuditTransport(func(req *http.Request) (*http.Response, error) {
				calls++
				return byokAuditResponse(req, "{}"), nil
			})}
			RamjetProxyHandler(ctx)
			require.GreaterOrEqual(t, recorder.Code, http.StatusBadRequest)
			require.Zero(t, calls)
		})
	}
}

// TestRamjetBYOKRequiredBeforeChunkDispatch prevents caller-triggered chunk queries from using a configured server key.
func TestRamjetBYOKRequiredBeforeChunkDispatch(t *testing.T) {
	byokAuditSetup(t)
	ctx := newAuthContext("")
	calls := 0
	httpcli = &http.Client{Transport: byokAuditTransport(func(req *http.Request) (*http.Response, error) {
		calls++
		return byokAuditResponse(req, "{\"results\":\"not-allowed\"}"), nil
	})}
	_, err := queryChunks(ctx, queryChunksArgs{user: &config.UserConfig{OpenaiToken: "SERVER_OPENAI_TOKEN"}, query: "synthetic", ext: ".txt", model: "test-model", content: []byte("local")})
	require.Error(t, err)
	require.Zero(t, calls)
}

// TestRamjetBYOKCachedServerCredentialsRejected prevents cached account state from replacing the original caller key.
func TestRamjetBYOKCachedServerCredentialsRejected(t *testing.T) {
	for _, supplied := range []string{"", "sk-SYNTHETIC-ORIGINAL-CALLER-KEY"} {
		t.Run(supplied, func(t *testing.T) {
			byokAuditSetup(t)
			ctx := newAuthContext(supplied)
			ctx.Set(ctxKeyUser, &config.UserConfig{BYOK: true, Token: "sk-SYNTHETIC-ORIGINAL-CALLER-KEY", OpenaiToken: "SERVER_SYNTHETIC_SUBSTITUTE", UserName: "persisted-user-id"})
			_, err := resolveRamjetUser(ctx)
			require.Error(t, err)
		})
	}
}

// TestRamjetBYOKSelectedProviderPreserved keeps explicit HTTP provider URLs instead of silently choosing a default.
func TestRamjetBYOKSelectedProviderPreserved(t *testing.T) {
	for _, base := range []string{
		"http://100.64.0.10:3000/provider?api-version=synthetic",
		"http://SYNTHETIC_URL_USER:SYNTHETIC_URL_PASS@100.64.0.10:3000/provider",
		"http://100.64.0.10:3000/provider#synthetic-fragment",
	} {
		t.Run(base, func(t *testing.T) {
			byokAuditSetup(t)
			key := "sk-SYNTHETIC-ONLY-PROVIDER-KEY"
			ctx, entries := byokAuthAuditContext(t, key)
			ctx.Request.Method = http.MethodPost
			ctx.Request.URL.Path = "/gptchat/ramjet/gptchat/query/chunks"
			ctx.Request.Body = io.NopCloser(strings.NewReader("{}"))
			ctx.Request.Header.Set("X-Laisky-Api-Base", base)
			calls := 0
			httpcli = &http.Client{Transport: byokAuditTransport(func(req *http.Request) (*http.Response, error) {
				calls++
				require.Equal(t, base, req.Header.Get("X-Laisky-Openai-Api-Base"))
				require.Equal(t, key, req.Header.Get("Authorization"))
				return byokAuditResponse(req, "{}"), nil
			})}
			RamjetProxyHandler(ctx)
			require.Equal(t, http.StatusOK, ctx.Writer.Status())
			require.Equal(t, 1, calls)
			require.NotContains(t, fmt.Sprint(entries.All()), base)
		})
	}
}

// TestRamjetBYOKMalformedProviderRejected stops invalid selected destinations before any upstream dispatch.
func TestRamjetBYOKMalformedProviderRejected(t *testing.T) {
	for _, base := range []string{
		"file:///synthetic", "https://", "http://[invalid", "http://provider.test:70000", "http://provider.test/%SYNTHETIC",
	} {
		t.Run(base, func(t *testing.T) {
			byokAuditSetup(t)
			ctx, entries := byokAuthAuditContext(t, "sk-SYNTHETIC-ONLY-PROVIDER-KEY")
			ctx.Request.Method = http.MethodPost
			ctx.Request.URL.Path = "/gptchat/ramjet/gptchat/query/chunks"
			ctx.Request.Body = io.NopCloser(strings.NewReader("{}"))
			ctx.Request.Header.Set("X-Laisky-Api-Base", base)
			calls := 0
			httpcli = &http.Client{Transport: byokAuditTransport(func(req *http.Request) (*http.Response, error) {
				calls++
				return byokAuditResponse(req, "{}"), nil
			})}
			RamjetProxyHandler(ctx)
			require.GreaterOrEqual(t, ctx.Writer.Status(), http.StatusBadRequest)
			require.Zero(t, calls)
			require.NotContains(t, fmt.Sprint(entries.All()), base)
		})
	}
}

// TestRamjetBYOKProviderResolutionLeavesGenericStateUnchanged keeps non-Ramjet provider policy independent.
func TestRamjetBYOKProviderResolutionLeavesGenericStateUnchanged(t *testing.T) {
	byokAuditSetup(t)
	key := "sk-SYNTHETIC-ONLY-PROVIDER-KEY"
	ctx := newAuthContext(key)
	selected := "http://100.64.0.10:3000/provider?api-version=synthetic"
	ctx.Request.Header.Set("X-Laisky-Api-Base", selected)
	general, err := getUserByAuthHeader(ctx)
	require.NoError(t, err)
	require.Equal(t, "https://oneapi.laisky.com", general.APIBase)
	resolved, err := resolveRamjetUser(ctx)
	require.NoError(t, err)
	require.Equal(t, selected, resolved.APIBase)
	require.Equal(t, "https://oneapi.laisky.com", general.APIBase)
	require.Equal(t, key[:15], resolved.UserName)
}

// TestRamjetBYOKImageCredentialConsistency prevents image requests from using a cached replacement credential.
func TestRamjetBYOKImageCredentialConsistency(t *testing.T) {
	originalBackend := gconfig.Shared.GetString("openai.rate_limiter_backend")
	gconfig.Shared.Set("openai.rate_limiter_backend", "legacy")
	t.Cleanup(func() { gconfig.Shared.Set("openai.rate_limiter_backend", originalBackend) })
	const key = "sk-SYNTHETIC-IMAGE-CALLER-KEY"
	for _, tc := range []struct {
		name, imageToken string
		rejected         bool
	}{
		{"missing", "", true},
		{"substituted", "SERVER_SYNTHETIC_IMAGE_SUBSTITUTE", true},
		{"caller", key, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			byokAuditSetup(t)
			config.Config.RateLimitExpensiveModelsIntervalSeconds = 60
			original := newAuthContext(key)
			original.Request.URL.Path = "/gptchat/ramjet/gptchat/image/dalle"
			original.Request.Body = io.NopCloser(strings.NewReader("{}"))
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = original.Request
			ctx.Set(ctxKeyUser, &config.UserConfig{BYOK: true, Token: key, OpenaiToken: key, ImageToken: tc.imageToken, UserName: key[:15], APIBase: "http://100.64.0.10:3000", AllowedModels: []string{"*"}})
			calls := 0
			httpcli = &http.Client{Transport: byokAuditTransport(func(req *http.Request) (*http.Response, error) {
				calls++
				require.Equal(t, key, req.Header.Get("Authorization"))
				return byokAuditResponse(req, "{}"), nil
			})}
			RamjetProxyHandler(ctx)
			if tc.rejected {
				require.GreaterOrEqual(t, recorder.Code, http.StatusBadRequest)
				require.Zero(t, calls)
			} else {
				require.Equal(t, http.StatusOK, recorder.Code)
				require.Equal(t, 1, calls)
			}
		})
	}
}
