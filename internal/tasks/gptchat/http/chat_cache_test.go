package http

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strconv"
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

// cacheTestUser returns the user the cache policy tests send requests as.
func cacheTestUser() *config.UserConfig {
	return &config.UserConfig{Token: "sk-synthetic-cache-tenant", APIBase: "https://upstream.test"}
}

// cacheTestContext returns a request context whose caller key is apiKey.
func cacheTestContext(apiKey string) *gin.Context {
	gctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	gctx.Set(ctxKeyUserAuth, apiKey)
	return gctx
}

// requireNotCached fails the test when llmRespCache holds an answer for req, under
// the bare request key an unscoped writer would use or, when gctx is not nil,
// under the caller-scoped key of gctx and user.
func requireNotCached(t *testing.T, gctx *gin.Context, user *config.UserConfig, req *FrontendReq) {
	t.Helper()
	bareKey, err := req2CacheKey(req)
	require.NoError(t, err)
	keys := []string{bareKey}
	if gctx != nil {
		scopedKey, err := requestResponseCacheKey(gctx, user, req)
		require.NoError(t, err)
		keys = append(keys, scopedKey)
	}
	for _, key := range keys {
		_, ok := llmRespCache.Load(key)
		require.False(t, ok, "answer cached although caching is disallowed for this request")
	}
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
		gctx := cacheTestContext(cacheTestUser().Token)
		require.NoError(t, storeCachedResponse(gctx, cacheTestUser(), req, "cached answer"))

		answer, ok, err := loadCachedResponse(gctx, cacheTestUser(), req)
		require.NoError(t, err)
		require.True(t, ok)
		require.Equal(t, "cached answer", answer)
	})

	t.Run("unknown caller key", func(t *testing.T) {
		isolateResponseCache(t, false)
		req := cacheTestRequest("")
		gctx := cacheTestContext("")
		require.Error(t, storeCachedResponse(gctx, cacheTestUser(), req, "answer for nobody"))
		requireNotCached(t, nil, nil, req)
		_, ok, err := loadCachedResponse(gctx, cacheTestUser(), req)
		require.Error(t, err)
		require.False(t, ok)
	})

	t.Run("disallowed", func(t *testing.T) {
		isolateResponseCache(t, false)
		req := cacheTestRequest("sk-synthetic-mcp-secret")
		gctx := cacheTestContext(cacheTestUser().Token)
		require.NoError(t, storeCachedResponse(gctx, cacheTestUser(), req, "answer derived from private MCP data"))
		requireNotCached(t, gctx, cacheTestUser(), req)

		// Even an entry planted under the same key must not be served.
		key, err := requestResponseCacheKey(gctx, cacheTestUser(), req)
		require.NoError(t, err)
		llmRespCache.Store(key, "planted")
		_, ok, err := loadCachedResponse(gctx, cacheTestUser(), req)
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
	requireNotCached(t, nil, nil, conservationTestRequest())
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

// cacheTenantUpstream starts a Responses API upstream that always answers with
// answer and counts its calls.
func cacheTenantUpstream(t *testing.T, answer string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("content-type", "application/json")
		if _, err := w.Write([]byte(`{"id":"resp-1","output_text":"` + answer + `","output":[]}`)); err != nil {
			t.Errorf("write upstream answer: %v", err)
		}
	}))
	t.Cleanup(upstream.Close)
	return upstream, &calls
}

// TestChatResponseCacheIsolatesTenants verifies that a cached chat answer is only
// served to the API key and upstream that produced it. A BYOK caller chooses its
// own upstream through X-Laisky-Api-Base, so an answer cached for one tenant may
// have been written by that tenant; serving it to another tenant sending the same
// body would let one tenant answer another tenant's question.
func TestChatResponseCacheIsolatesTenants(t *testing.T) {
	isolateResponsesChatGlobals(t)
	gin.SetMode(gin.TestMode)

	chosen, chosenCalls := cacheTenantUpstream(t, "answer from a caller-chosen upstream")
	genuine, genuineCalls := cacheTenantUpstream(t, "genuine answer")

	originalCli := httpcli
	httpcli = &http.Client{Timeout: 10 * time.Second}
	t.Cleanup(func() { httpcli = originalCli })

	originalConfig := config.Config
	config.Config = &config.OpenAI{
		Token:                                   "srv-token",
		API:                                     genuine.URL,
		RateLimitExpensiveModelsIntervalSeconds: 600,
	}
	t.Cleanup(func() { config.Config = originalConfig })

	newUser := func(token, apiBase string) *config.UserConfig {
		user := &config.UserConfig{
			Token: token, UserName: "tenant", APIBase: apiBase, OpenaiToken: token,
			AllowedModels: []string{"*"}, NoLimitExpensiveModels: true, BYOK: true,
		}
		require.NoError(t, user.Valid())
		return user
	}
	ask := func(user *config.UserConfig) string {
		t.Helper()
		rec := httptest.NewRecorder()
		gctx, _ := gin.CreateTestContext(rec)
		gctx.Set(ctxKeyUser, user)
		gctx.Request = httptest.NewRequest(http.MethodPost, "/gptchat/api",
			strings.NewReader(`{"stream":false,`+conservationTestPrompt+`}`))
		gctx.Request.Header.Set("content-type", "application/json")
		gctx.Request.Header.Set("authorization", "Bearer "+user.Token)
		require.NoError(t, sendChatWithResponsesToolLoop(gctx))
		return rec.Body.String()
	}

	other := newUser("sk-synthetic-tenant-a-0001", chosen.URL)
	victim := newUser("sk-synthetic-tenant-b-0002", genuine.URL)

	require.Contains(t, ask(other), "answer from a caller-chosen upstream")
	require.Equal(t, int32(1), chosenCalls.Load())

	require.Contains(t, ask(victim), "genuine answer", "another tenant's cached answer was served")
	require.Equal(t, int32(1), genuineCalls.Load())

	// The same tenant and upstream still benefit from the cache.
	require.Contains(t, ask(victim), "genuine answer")
	require.Equal(t, int32(1), genuineCalls.Load(), "a repeated request of the same tenant must be served from the cache")

	// The same API key pointed at another upstream gets that upstream's answer.
	victimElsewhere := newUser(victim.Token, chosen.URL)
	require.Contains(t, ask(victimElsewhere), "answer from a caller-chosen upstream")
	require.Equal(t, int32(2), chosenCalls.Load(), "an answer cached for another upstream was served")
}

// TestChatResponseCacheIsolatesFreeTierCallers verifies that two callers with
// different FREETIER- keys never share cached answers. Every free-tier caller is
// resolved to a copy of the shared free-tier user, so user.Token is the same for
// all of them; the cache must be scoped by the caller's own key instead. The
// request enables an MCP server without its own key, which is called with the
// caller's key, so an answer may carry that caller's private MCP data.
func TestChatResponseCacheIsolatesFreeTierCallers(t *testing.T) {
	isolateResponsesChatGlobals(t)
	setupFreeUserTestQuota(t)
	gin.SetMode(gin.TestMode)

	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := calls.Add(1)
		w.Header().Set("content-type", "application/json")
		answer := `{"id":"resp-1","output_text":"answer ` + strconv.Itoa(int(n)) + `","output":[]}`
		if _, err := w.Write([]byte(answer)); err != nil {
			t.Errorf("write upstream answer: %v", err)
		}
	}))
	t.Cleanup(upstream.Close)

	originalCli := httpcli
	httpcli = &http.Client{Timeout: 10 * time.Second}
	t.Cleanup(func() { httpcli = originalCli })

	originalConfig := config.Config
	config.Config = &config.OpenAI{
		Token:                                   "srv-token",
		API:                                     upstream.URL,
		RateLimitExpensiveModelsIntervalSeconds: 600,
		UserTokens: []*config.UserConfig{{
			Token: config.FreetierUserToken, UserName: "public", APIBase: upstream.URL,
			OpenaiToken: "sk-synthetic-shared", AllowedModels: []string{"*"}, NoLimitExpensiveModels: true,
		}},
	}
	t.Cleanup(func() { config.Config = originalConfig })

	const body = `{"stream":false,"model":"gpt-4.1","max_tokens":50,` +
		`"mcp_servers":[{"name":"private","url":"https://mcp.test","enabled":true}],` +
		`"messages":[{"role":"user","content":"what is in my notes?"}]}`
	ask := func(key string) string {
		t.Helper()
		rec := httptest.NewRecorder()
		gctx, _ := gin.CreateTestContext(rec)
		gctx.Request = httptest.NewRequest(http.MethodPost, "/gptchat/api", strings.NewReader(body))
		gctx.Request.Header.Set("content-type", "application/json")
		gctx.Request.Header.Set("authorization", "Bearer "+key)
		require.NoError(t, sendChatWithResponsesToolLoop(gctx))
		return rec.Body.String()
	}

	callerA, callerB := "FREETIER-synthetic-caller-a", "FREETIER-synthetic-caller-b"
	require.Contains(t, ask(callerA), "answer 1")
	require.Contains(t, ask(callerB), "answer 2", "caller B was served caller A's cached answer")
	require.Equal(t, int32(2), calls.Load())

	// The same caller still benefits from the cache.
	require.Contains(t, ask(callerA), "answer 1")
	require.Equal(t, int32(2), calls.Load(), "a repeated request of the same caller must be served from the cache")
}

// TestResponseCacheKeyCannotBeRecomputedFromAPIKey verifies the cache key is a keyed
// digest: it is stable within the process, but nobody holding only an API key and
// a request can recompute it, because a plain SHA-256 of the key material is a
// fast hash of a secret.
func TestResponseCacheKeyCannotBeRecomputedFromAPIKey(t *testing.T) {
	const apiKey, apiBase = "sk-synthetic-cache-tenant", "https://upstream.test"
	req := cacheTestRequest("")
	key, err := responseCacheKey(apiKey, apiBase, req)
	require.NoError(t, err)
	again, err := responseCacheKey(apiKey, apiBase, req)
	require.NoError(t, err)
	require.Equal(t, key, again, "the cache key must be stable within the process")

	reqKey, err := req2CacheKey(req)
	require.NoError(t, err)
	var material []byte
	for _, part := range []string{apiKey, apiBase, reqKey} {
		material = strconv.AppendInt(material, int64(len(part)), 10)
		material = append(material, ':')
		material = append(material, part...)
	}
	plain := sha256.Sum256(material)
	require.NotEqual(t, hex.EncodeToString(plain[:]), key, "the cache key is an unkeyed hash of the API key")
	require.NotContains(t, key, apiKey)
}
