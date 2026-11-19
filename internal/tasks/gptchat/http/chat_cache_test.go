package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	gutils "github.com/Laisky/go-utils/v6"
	glog "github.com/Laisky/go-utils/v6/log"
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

// conservationTestRequest is the request every conservation test records.
func conservationTestRequest() *FrontendReq {
	return &FrontendReq{
		Model:     "gpt-4.1",
		MaxTokens: 50,
		Messages: []FrontendReqMessage{{
			Role:    OpenaiMessageRoleUser,
			Content: FrontendReqMessageContent{StringContent: "hello"},
		}},
	}
}

// conservationTestPrompt is conservationTestRequest as JSON object members.
const conservationTestPrompt = `"model":"gpt-4.1","max_tokens":50,"messages":[{"role":"user","content":"hello"}]`

// TestSaveLlmConservationHandlerOnlyRecordsAudit verifies that the conservation
// audit endpoint, which accepts any caller-supplied answer without authentication,
// records the audit entry but never writes llmRespCache.
func TestSaveLlmConservationHandlerOnlyRecordsAudit(t *testing.T) {
	isolateResponseCache(t, false)
	gin.SetMode(gin.TestMode)

	type record struct {
		req      *FrontendReq
		response string
	}
	recorded := make(chan record, 1)
	origPersist := persistConservation
	t.Cleanup(func() { persistConservation = origPersist })
	persistConservation = func(_ glog.Logger, req *FrontendReq, response, _ string) {
		recorded <- record{req: req, response: response}
	}

	rec := httptest.NewRecorder()
	gctx, _ := gin.CreateTestContext(rec)
	gctx.Request = httptest.NewRequest(http.MethodPost, "/gptchat/audit/conservation",
		strings.NewReader(`{`+conservationTestPrompt+`,"response":"planted answer"}`))
	gctx.Request.Header.Set("content-type", "application/json")
	SaveLlmConservationHandler(gctx)
	require.Less(t, rec.Code, http.StatusBadRequest)

	select {
	case got := <-recorded:
		require.Equal(t, "planted answer", got.response)
		require.Equal(t, conservationTestRequest().Messages, got.req.Messages)
	case <-time.After(5 * time.Second):
		require.FailNow(t, "the audit entry was never persisted")
	}
	requireNotCached(t, conservationTestRequest())
}

// TestChatCacheKeyNeverMatchesConservationRecord pins why a conservation-shaped
// cache entry was never served even when the audit endpoint still wrote one: the
// chat endpoint keys the cache on a request whose temperature, top_p and n
// defaults bodyChecker always fills, which a conservation record cannot carry. An
// entry planted under the conservation key must not answer the same chat question.
func TestChatCacheKeyNeverMatchesConservationRecord(t *testing.T) {
	isolateResponsesChatGlobals(t)
	gin.SetMode(gin.TestMode)

	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		upstreamCalls.Add(1)
		w.Header().Set("content-type", "application/json")
		if _, err := w.Write([]byte(`{"id":"resp-1","output_text":"genuine answer","output":[]}`)); err != nil {
			t.Errorf("write upstream answer: %v", err)
		}
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

	plantedKey, err := req2CacheKey(conservationTestRequest())
	require.NoError(t, err)
	llmRespCache.Store(plantedKey, "planted answer")

	user := &config.UserConfig{
		Token:         "laisky-abcdefghijklmno",
		UserName:      "victim",
		APIBase:       strings.TrimRight(upstream.URL, "/"),
		OpenaiToken:   "sk-user",
		AllowedModels: []string{"*"},
	}
	require.NoError(t, user.Valid())

	rec := httptest.NewRecorder()
	gctx, _ := gin.CreateTestContext(rec)
	gctx.Set(ctxKeyUser, user)
	gctx.Request = httptest.NewRequest(http.MethodPost, "/gptchat/api",
		strings.NewReader(`{"stream":false,`+conservationTestPrompt+`}`))
	gctx.Request.Header.Set("content-type", "application/json")
	gctx.Request.Header.Set("authorization", "Bearer "+user.Token)

	require.NoError(t, sendChatWithResponsesToolLoop(gctx))
	require.NotContains(t, rec.Body.String(), "planted answer", "chat served a conservation-shaped cache entry")
	require.Contains(t, rec.Body.String(), "genuine answer")
	require.Equal(t, int32(1), upstreamCalls.Load())
}
