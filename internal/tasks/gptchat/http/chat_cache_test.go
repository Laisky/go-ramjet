package http

import (
	"testing"
	"time"

	gutils "github.com/Laisky/go-utils/v6"
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

// TestSaveLLMConservationRespectsCachePolicy verifies the conservation audit never
// caches answers of requests the chat read path refuses to cache: requests with MCP
// credentials (the answer may expose data fetched with them) and any request while
// transparent memory is enabled (answers are personalized).
func TestSaveLLMConservationRespectsCachePolicy(t *testing.T) {
	t.Run("mcp api key", func(t *testing.T) {
		isolateResponseCache(t, false)
		req := cacheTestRequest("sk-synthetic-mcp-secret")
		saveLLMConservation(conservationLogger(), req, "answer derived from private MCP data", "")
		requireNotCached(t, req)
	})

	t.Run("memory enabled", func(t *testing.T) {
		isolateResponseCache(t, true)
		req := cacheTestRequest("")
		saveLLMConservation(conservationLogger(), req, "personalized answer", "")
		requireNotCached(t, req)
	})
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
