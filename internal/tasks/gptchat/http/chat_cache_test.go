package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	gutils "github.com/Laisky/go-utils/v6"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
)

// isolateResponseCache installs a private llmRespCache and configuration for one
// test and restores both on cleanup. enableMemory sets config.Config.EnableMemory.
func isolateResponseCache(t *testing.T, enableMemory bool) {
	t.Helper()
	originalCache, originalConfig := llmRespCache, config.Config
	t.Cleanup(func() { llmRespCache, config.Config = originalCache, originalConfig })
	llmRespCache = gutils.NewExpCache[string](t.Context(), 3*time.Second)
	config.Config = &config.OpenAI{EnableMemory: enableMemory}
}

// cacheTestRequest returns a chat request with one user message; mcpAPIKey, when
// set, attaches an MCP server carrying that credential.
func cacheTestRequest(mcpAPIKey string) *FrontendReq {
	req := &FrontendReq{
		Model: "gpt-4.1",
		Messages: []FrontendReqMessage{{
			Role:    OpenaiMessageRoleUser,
			Content: FrontendReqMessageContent{StringContent: "hello"},
		}},
	}
	if mcpAPIKey != "" {
		req.MCPServers = []MCPServerConfig{{Name: "private", URL: "https://mcp.test", APIKey: mcpAPIKey, Enabled: true}}
	}
	return req
}

// requireNotCached fails the test when llmRespCache holds an answer for req.
func requireNotCached(t *testing.T, req *FrontendReq) {
	t.Helper()
	key, err := req2CacheKey(req)
	require.NoError(t, err)
	_, ok := llmRespCache.Load(key)
	require.False(t, ok, "answer cached although caching is disallowed for this request")
}

// TestResponseCacheAllowed pins the single cache policy shared by every cache read
// and write.
func TestResponseCacheAllowed(t *testing.T) {
	for _, tc := range []struct {
		name         string
		req          *FrontendReq
		enableMemory bool
		want         bool
	}{
		{name: "plain chat", req: cacheTestRequest(""), want: true},
		{name: "nil request", req: nil, want: false},
		{name: "no messages", req: &FrontendReq{Model: "gpt-4.1"}, want: false},
		{name: "memory enabled", req: cacheTestRequest(""), enableMemory: true, want: false},
		{name: "mcp api key", req: cacheTestRequest("sk-synthetic-mcp-secret"), want: false},
		{name: "blank mcp api key", req: cacheTestRequest("   "), want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			isolateResponseCache(t, tc.enableMemory)
			require.Equal(t, tc.want, responseCacheAllowed(tc.req))
		})
	}
}

// TestStoreAndLoadCachedResponseShareThePolicy verifies that allowed answers round
// trip through the cache while disallowed ones are neither stored nor served.
func TestStoreAndLoadCachedResponseShareThePolicy(t *testing.T) {
	t.Run("allowed", func(t *testing.T) {
		isolateResponseCache(t, false)
		req := cacheTestRequest("")
		require.NoError(t, storeCachedResponse(req, "cached answer"))

		answer, ok, err := loadCachedResponse(req)
		require.NoError(t, err)
		require.True(t, ok)
		require.Equal(t, "cached answer", answer)
	})

	t.Run("disallowed", func(t *testing.T) {
		isolateResponseCache(t, false)
		req := cacheTestRequest("sk-synthetic-mcp-secret")
		require.NoError(t, storeCachedResponse(req, "answer derived from private MCP data"))
		requireNotCached(t, req)

		// Even an entry planted under the same key must not be served.
		key, err := req2CacheKey(req)
		require.NoError(t, err)
		llmRespCache.Store(key, "planted")
		_, ok, err := loadCachedResponse(req)
		require.NoError(t, err)
		require.False(t, ok)
	})
}

// TestSaveLlmConservationHandlerCannotSeedChatAnswers verifies that the conservation
// audit endpoint, which accepts any caller-supplied answer without authentication,
// never writes llmRespCache, so it cannot plant an answer that the chat endpoint
// serves to another caller instead of asking the upstream.
func TestSaveLlmConservationHandlerCannotSeedChatAnswers(t *testing.T) {
	isolateResponsesChatGlobals(t)
	gin.SetMode(gin.TestMode)

	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		upstreamCalls.Add(1)
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp-1","output_text":"genuine answer","output":[]}`))
	}))
	t.Cleanup(upstream.Close)

	originalCli := httpcli
	httpcli = upstream.Client()
	t.Cleanup(func() { httpcli = originalCli })

	originalConfig := config.Config
	config.Config = &config.OpenAI{
		Token:                                   "srv-token",
		API:                                     strings.TrimRight(upstream.URL, "/"),
		RateLimitExpensiveModelsIntervalSeconds: 600,
	}
	t.Cleanup(func() { config.Config = originalConfig })

	const prompt = `"model":"gpt-4.1","max_tokens":50,"messages":[{"role":"user","content":"hello"}]`

	// An anonymous caller records a conservation carrying an answer of its choice.
	auditRec := httptest.NewRecorder()
	auditCtx, _ := gin.CreateTestContext(auditRec)
	auditCtx.Request = httptest.NewRequest(http.MethodPost, "/gptchat/audit/conservation",
		strings.NewReader(`{`+prompt+`,"response":"planted answer"}`))
	auditCtx.Request.Header.Set("content-type", "application/json")
	SaveLlmConservationHandler(auditCtx)
	require.Less(t, auditRec.Code, http.StatusBadRequest)
	requireNotCached(t, &FrontendReq{
		Model:     "gpt-4.1",
		MaxTokens: 50,
		Messages: []FrontendReqMessage{{
			Role:    OpenaiMessageRoleUser,
			Content: FrontendReqMessageContent{StringContent: "hello"},
		}},
	})

	// Another user then asks the same question through the chat endpoint.
	user := &config.UserConfig{
		Token:         "laisky-abcdefghijklmno",
		UserName:      "victim",
		APIBase:       strings.TrimRight(upstream.URL, "/"),
		OpenaiToken:   "sk-user",
		AllowedModels: []string{"*"},
	}
	require.NoError(t, user.Valid())

	chatRec := httptest.NewRecorder()
	chatCtx, _ := gin.CreateTestContext(chatRec)
	chatCtx.Set(ctxKeyUser, user)
	chatCtx.Request = httptest.NewRequest(http.MethodPost, "/gptchat/api",
		strings.NewReader(`{"stream":false,`+prompt+`}`))
	chatCtx.Request.Header.Set("content-type", "application/json")
	chatCtx.Request.Header.Set("authorization", "Bearer "+user.Token)

	require.NoError(t, sendChatWithResponsesToolLoop(chatCtx))
	require.NotContains(t, chatRec.Body.String(), "planted answer", "chat served an answer planted through the audit endpoint")
	require.Contains(t, chatRec.Body.String(), "genuine answer")
	require.Equal(t, int32(1), upstreamCalls.Load())
}
