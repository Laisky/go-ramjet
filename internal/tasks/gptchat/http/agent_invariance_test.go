package http

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
)

// TestI1_ProxyInvariance_NoAgentModeSkipsDispatcher exercises the
// proxy path with `agent_mode` absent from the inbound request and
// asserts the registered agent dispatcher is never invoked. The
// existing http-package tests that exercise sendChatWithResponsesToolLoop
// already verify the *bytes* of the proxy path are unchanged; this
// test pins the dispatch gate so a future refactor cannot accidentally
// route a non-agent request through the agent loop.
//
// See docs/proposals/2026-05-26-gptchat-react-agent-mode.md §6.2 I1.
func TestI1_ProxyInvariance_NoAgentModeSkipsDispatcher(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Stub upstream — returns a tiny non-streaming response so the
	// proxy path runs to completion. The test only cares about the
	// gate; the proxy bytes are covered elsewhere.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		_, _ = io.ReadAll(r.Body)
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp-1","output_text":"ok","output":[]}`))
	}))
	t.Cleanup(upstream.Close)

	originalCli := httpcli
	httpcli = upstream.Client()
	t.Cleanup(func() { httpcli = originalCli })

	originalConfig := config.Config
	config.Config = &config.OpenAI{
		Token:                                   "srv-token",
		DefaultImageToken:                       "srv-image-token",
		DefaultImageUrl:                         upstream.URL + "/v1/images/generations",
		API:                                     strings.TrimRight(upstream.URL, "/"),
		RateLimitExpensiveModelsIntervalSeconds: 600,
		RamjetURL:                               "",
		MemoryProject:                           "gptchat",
		MemoryStorageMCPURL:                     "",
		MemoryLLMTimeoutSeconds:                 15,
		MemoryLLMMaxOutputTokens:                512,
	}
	t.Cleanup(func() { config.Config = originalConfig })

	user := &config.UserConfig{
		Token:         "laisky-abcdefghijklmno",
		UserName:      "tester",
		APIBase:       strings.TrimRight(upstream.URL, "/"),
		OpenaiToken:   "sk-user",
		AllowedModels: []string{"*"},
	}
	require.NoError(t, user.Valid())

	// Install a tripwire dispatcher; restore the original on cleanup.
	originalDispatcher := registeredAgentDispatcher
	t.Cleanup(func() { registeredAgentDispatcher = originalDispatcher })

	var tripped atomic.Int32
	RegisterAgentDispatcher(func(
		ctx *gin.Context,
		fr *FrontendReq,
		u *config.UserConfig,
		rr *OpenAIResponsesReq,
		hdr http.Header,
	) error {
		tripped.Add(1)
		return nil
	})

	// Request without agent_mode flag — proxy path should run.
	// Use a unique prompt so the response cache does not bleed into the
	// upstream-error test that uses "hi".
	const body = `{
		"model": "gpt-4.1",
		"stream": false,
		"max_tokens": 50,
		"messages": [{"role": "user", "content": "agent-mode-invariance-probe-A"}]
	}`

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Set(ctxKeyUser, user)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/gptchat/api", strings.NewReader(body))
	ctx.Request.Header.Set("content-type", "application/json")
	ctx.Request.Header.Set("authorization", "Bearer "+user.Token)

	err := sendChatWithResponsesToolLoop(ctx)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code)

	require.Equal(t, int32(0), tripped.Load(),
		"agent dispatcher must not fire for non-agent requests; trips=%d",
		tripped.Load())
	require.False(t, ctx.GetBool(ctxKeyAgentMode),
		"ctxKeyAgentMode must remain false for a request without agent_mode")
}

// TestI1b_AgentModeFlagRoutesToDispatcher asserts the inverse: when
// `agent_mode: true` is set, the dispatcher IS invoked. This pins the
// other half of the dispatch contract so a regression on one side does
// not silently mask the other.
func TestI1b_AgentModeFlagRoutesToDispatcher(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"id":"r-1","output_text":"ok","output":[]}`))
	}))
	t.Cleanup(upstream.Close)
	originalCli := httpcli
	httpcli = upstream.Client()
	t.Cleanup(func() { httpcli = originalCli })

	originalConfig := config.Config
	config.Config = &config.OpenAI{
		Token:                                   "srv-token",
		API:                                     strings.TrimRight(upstream.URL, "/"),
		DefaultImageToken:                       "srv-image-token",
		DefaultImageUrl:                         upstream.URL + "/v1/images/generations",
		RateLimitExpensiveModelsIntervalSeconds: 600,
		RamjetURL:                               "",
		MemoryProject:                           "gptchat",
		MemoryStorageMCPURL:                     "",
		MemoryLLMTimeoutSeconds:                 15,
		MemoryLLMMaxOutputTokens:                512,
	}
	t.Cleanup(func() { config.Config = originalConfig })

	user := &config.UserConfig{
		Token:         "laisky-abcdefghijklmno",
		UserName:      "tester",
		APIBase:       strings.TrimRight(upstream.URL, "/"),
		OpenaiToken:   "sk-user",
		AllowedModels: []string{"*"},
	}
	require.NoError(t, user.Valid())

	originalDispatcher := registeredAgentDispatcher
	t.Cleanup(func() { registeredAgentDispatcher = originalDispatcher })

	var tripped atomic.Int32
	RegisterAgentDispatcher(func(
		ctx *gin.Context,
		fr *FrontendReq,
		u *config.UserConfig,
		rr *OpenAIResponsesReq,
		hdr http.Header,
	) error {
		tripped.Add(1)
		return nil
	})

	const body = `{
		"model": "gpt-4.1",
		"stream": false,
		"max_tokens": 50,
		"messages": [{"role": "user", "content": "agent-mode-invariance-probe-B"}],
		"laisky_extra": {"chat_switch": {"agent_mode": true}}
	}`

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Set(ctxKeyUser, user)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/gptchat/api", strings.NewReader(body))
	ctx.Request.Header.Set("content-type", "application/json")
	ctx.Request.Header.Set("authorization", "Bearer "+user.Token)

	_ = sendChatWithResponsesToolLoop(ctx)

	require.True(t, ctx.GetBool(ctxKeyAgentMode),
		"ctxKeyAgentMode must be set when the request flips agent_mode=true")
	require.Equal(t, int32(1), tripped.Load(),
		"agent dispatcher must fire exactly once for an agent request")
}
