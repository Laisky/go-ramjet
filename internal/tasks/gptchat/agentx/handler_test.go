package agentx

import (
	"context"
	stdjson "encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/hook"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/model"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/session"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/tool"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/tools"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
	httppkg "github.com/Laisky/go-ramjet/internal/tasks/gptchat/http"
)

// -----------------------------------------------------------------------------
// Shared test helpers
// -----------------------------------------------------------------------------

// fakeModelClient drives the loop with a scripted sequence of one batch
// per Stream() call. Mirrors the loop package's fake (kept local here so
// the test file does not depend on internal test packages).
type fakeModelClient struct {
	mu        sync.Mutex
	scripts   [][]model.StreamChunk
	calls     int
	caps      model.Capabilities
}

func newFakeModelClient(scripts [][]model.StreamChunk) *fakeModelClient {
	return &fakeModelClient{
		scripts: scripts,
		caps:    model.Capabilities{SupportsParallelToolCalls: true},
	}
}

func (f *fakeModelClient) Stream(ctx context.Context, _ model.Request) (<-chan model.StreamChunk, error) {
	f.mu.Lock()
	idx := f.calls
	f.calls++
	var batch []model.StreamChunk
	if idx < len(f.scripts) {
		batch = f.scripts[idx]
	} else {
		batch = []model.StreamChunk{{Kind: model.ChunkText, Text: ""}, {Kind: model.ChunkDone}}
	}
	f.mu.Unlock()

	ch := make(chan model.StreamChunk, len(batch)+1)
	go func() {
		defer close(ch)
		for _, c := range batch {
			select {
			case <-ctx.Done():
				return
			case ch <- c:
			}
		}
	}()
	return ch, nil
}

func (f *fakeModelClient) Capabilities() model.Capabilities { return f.caps }

func (f *fakeModelClient) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// fakeTool is a minimal tool.Tool used to satisfy the registry contract
// without requiring an actual MCP server.
type fakeTool struct {
	name   string
	output string
}

func (t *fakeTool) Name() string               { return t.name }
func (t *fakeTool) Description() string        { return "fake " + t.name }
func (t *fakeTool) Schema() stdjson.RawMessage { return stdjson.RawMessage(`{"type":"object"}`) }
func (t *fakeTool) Execute(_ context.Context, _ tool.Call, _ session.EventSink) (tool.Result, error) {
	return tool.Result{Content: t.output}, nil
}

// -----------------------------------------------------------------------------
// Test scaffolding
// -----------------------------------------------------------------------------

// setupTestConfig installs a minimal global config; t.Cleanup restores.
func setupTestConfig(t *testing.T, agent *config.AgentLoopConfig) {
	t.Helper()
	original := config.Config
	config.Config = &config.OpenAI{
		Token:                                   "srv-token",
		API:                                     "https://api.test",
		ExternalBillingAPI:                      "https://billing.test",
		RamjetURL:                               "https://ramjet.test",
		MemoryProject:                           "test-memory",
		MemoryStorageMCPURL:                     "https://mcp.test",
		MemoryModel:                             "openai/gpt-oss-120b",
		MemoryLLMTimeoutSeconds:                 15,
		MemoryLLMMaxOutputTokens:                512,
		RateLimitExpensiveModelsIntervalSeconds: 600,
		DefaultImageUrl:                         "https://api.test/v1/images/generations",
		DefaultImageToken:                       "srv-token",
		AgentLoop:                               agent,
	}
	t.Cleanup(func() { config.Config = original })
}

func defaultAgentCfg() *config.AgentLoopConfig {
	return &config.AgentLoopConfig{
		Enabled:               true,
		MaxIterations:         5,
		MaxToolCalls:          10,
		MaxParallelToolCalls:  4,
		WallClockSeconds:      30,
		CircuitBreakerRepeats: 3,
		ErrorBudget:           4,
		WriteGate:             "ask",
		WebFetchMaxTokens:     25000,
		DefaultFileProject:    "go-ramjet",
	}
}

// newTestGinCtx builds a recorder-backed gin.Context with a user
// stashed at ctxKeyUser. The user is a BYOK laisky user so the
// downstream flatten / convert helpers do not need an external billing
// system.
func newTestGinCtx(t *testing.T, body string) (*gin.Context, *httptest.ResponseRecorder, *config.UserConfig) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	user := &config.UserConfig{
		Token:                  "laisky-test-token-12345",
		UserName:               "tester",
		APIBase:                "https://api.test",
		OpenaiToken:            "sk-tester-12345",
		ImageToken:             "sk-tester-12345",
		ImageUrl:               "https://api.test/v1/images/generations",
		AllowedModels:          []string{"*"},
		NoLimitExpensiveModels: true,
	}
	require.NoError(t, user.Valid())

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	// Stash the resolved user so getUserByAuthHeader skips the lookup.
	ctx.Set("ctx_user", user)
	ctx.Set("ctx_user_auth", user.Token)
	// Install the gin-middlewares lock so GinStreamSink's CtxLock works
	// without LockableMw running first.
	ctx.Set("@laisky-gmw:lock", &sync.RWMutex{})
	ctx.Request = httptest.NewRequest(http.MethodPost, "/gptchat/api", strings.NewReader(body))
	ctx.Request.Header.Set("content-type", "application/json")
	ctx.Request.Header.Set("authorization", "Bearer "+user.Token)
	return ctx, rec, user
}

// silence the gmw "unused" lint by referencing it once.
var _ = gmw.GetGinCtxFromStdCtx

// buildRegistry creates a tool.Registry with send_to_user + the
// supplied fakes. Used by I2/I6 to bypass MCP discovery.
func buildRegistry(t *testing.T, extras ...tool.Tool) tool.Registry {
	t.Helper()
	l, err := glog.NewConsoleWithName("test_agentx", glog.LevelError)
	require.NoError(t, err)
	reg := tool.NewRegistry(l)
	require.NoError(t, reg.Register(tools.NewSendToUserTool(), tool.SourceLocal))
	for _, e := range extras {
		require.NoError(t, reg.Register(e, tool.SourceCuratedMCP))
	}
	return reg
}

// rawArgs marshals a Go value into a json.RawMessage.
func rawArgs(t *testing.T, v any) stdjson.RawMessage {
	t.Helper()
	b, err := stdjson.Marshal(v)
	require.NoError(t, err)
	return b
}

// frontendReqAgent builds a FrontendReq with AgentMode set to the
// supplied pointer value.
func frontendReqAgent(agentMode *bool, prompt string, enableMCP *bool) *httppkg.FrontendReq {
	req := &httppkg.FrontendReq{
		Model: "gpt-test",
		Messages: []httppkg.FrontendReqMessage{{
			Role:    httppkg.OpenaiMessageRoleUser,
			Content: httppkg.FrontendReqMessageContent{StringContent: prompt},
		}},
		EnableMCP: enableMCP,
	}
	if agentMode != nil {
		req.LaiskyExtra = &struct {
			ChatSwitch struct {
				DisableHttpsCrawler bool  `json:"disable_https_crawler"`
				EnableGoogleSearch  bool  `json:"enable_google_search"`
				EnableMemory        *bool `json:"enable_memory,omitempty"`
				AgentMode           *bool `json:"agent_mode,omitempty"`
			} `json:"chat_switch"`
		}{}
		req.LaiskyExtra.ChatSwitch.AgentMode = agentMode
	}
	return req
}

// -----------------------------------------------------------------------------
// I3 — config-disabled returns ErrAgentLoopDisabled
// -----------------------------------------------------------------------------

func TestI3_HandleAgent_ConfigDisabled(t *testing.T) {
	setupTestConfig(t, nil) // AgentLoop nil

	ctx, _, user := newTestGinCtx(t, "{}")
	on := true
	req := frontendReqAgent(&on, "hello", nil)
	err := HandleAgent(ctx, req, user, &httppkg.OpenAIResponsesReq{Model: "x"}, http.Header{})
	require.ErrorIs(t, err, ErrAgentLoopDisabled)
}

func TestI3_HandleAgent_EnabledFalse(t *testing.T) {
	cfg := defaultAgentCfg()
	cfg.Enabled = false
	setupTestConfig(t, cfg)

	ctx, _, user := newTestGinCtx(t, "{}")
	on := true
	req := frontendReqAgent(&on, "hello", nil)
	err := HandleAgent(ctx, req, user, &httppkg.OpenAIResponsesReq{Model: "x"}, http.Header{})
	require.ErrorIs(t, err, ErrAgentLoopDisabled)
}

// -----------------------------------------------------------------------------
// I2 — end-to-end agent run via stubbed model client
// -----------------------------------------------------------------------------

func TestI2_EndToEnd_WebFetchThenSendToUser(t *testing.T) {
	setupTestConfig(t, defaultAgentCfg())

	ctx, rec, user := newTestGinCtx(t, "{}")
	on := true
	req := frontendReqAgent(&on, "fetch X and tell me its title", nil)

	// Build a scripted model: round 1 -> web_fetch, round 2 -> send_to_user.
	scripts := [][]model.StreamChunk{
		{
			{Kind: model.ChunkFunction, FunctionCall: &model.FunctionCall{
				CallID:    "fc-fetch-1",
				Name:      "web_fetch",
				Arguments: rawArgs(t, map[string]any{"url": "https://example.com"}),
			}},
			{Kind: model.ChunkDone},
		},
		{
			{Kind: model.ChunkFunction, FunctionCall: &model.FunctionCall{
				CallID:    "fc-send-1",
				Name:      "send_to_user",
				Arguments: rawArgs(t, map[string]any{"final_answer": "The title is X."}),
			}},
			{Kind: model.ChunkDone},
		},
	}
	fake := newFakeModelClient(scripts)

	// Build a registry with send_to_user + a fake web_fetch tool.
	registry := buildRegistry(t, &fakeTool{name: "web_fetch", output: "<html><title>X</title></html>"})

	err := handleAgentWithDeps(ctx, agentRunInputs{
		FrontendReq:    req,
		User:           user,
		ResponsesReq:   &httppkg.OpenAIResponsesReq{Model: "gpt-test"},
		UpstreamHeader: http.Header{},
		AgentCfg:       defaultAgentCfg(),
	}, busOverride{
		ModelClient: fake,
		Registry:    registry,
	})
	require.NoError(t, err)

	body := rec.Body.String()
	// Trace line for the web_fetch tool call should land in the SSE.
	require.Contains(t, body, "tool_call: web_fetch")
	// Final answer is streamed via delta.content.
	require.Contains(t, body, "The title is X.")
	// Finish chunk arrives.
	require.Contains(t, body, `"finish_reason":"stop"`)
	// The model client was called twice (one per round).
	require.Equal(t, 2, fake.callCount())
}

// -----------------------------------------------------------------------------
// U13 — request's EnableMCP is never mutated even though the loop forces MCP on
// -----------------------------------------------------------------------------

func TestU13_EnableMCPIsolation(t *testing.T) {
	setupTestConfig(t, defaultAgentCfg())

	ctx, _, user := newTestGinCtx(t, "{}")

	// Caller passed EnableMCP=false; the agent loop should NOT mutate it.
	off := false
	on := true
	req := frontendReqAgent(&on, "hello", &off)
	// Caller also supplied one MCP server up-front (e.g. they had the
	// "MCP" UI toggle on). The agent path must not mutate this slice
	// even though it later injects the curated server into its own
	// dispatch-path copy.
	req.MCPServers = []httppkg.MCPServerConfig{
		{URL: "https://user-mcp.test", Enabled: true},
	}

	// Single-round model: immediate send_to_user.
	scripts := [][]model.StreamChunk{{
		{Kind: model.ChunkFunction, FunctionCall: &model.FunctionCall{
			CallID:    "fc-send-1",
			Name:      "send_to_user",
			Arguments: rawArgs(t, map[string]any{"final_answer": "done"}),
		}},
		{Kind: model.ChunkDone},
	}}
	fake := newFakeModelClient(scripts)
	registry := buildRegistry(t)

	err := handleAgentWithDeps(ctx, agentRunInputs{
		FrontendReq:    req,
		User:           user,
		ResponsesReq:   &httppkg.OpenAIResponsesReq{Model: "gpt-test"},
		UpstreamHeader: http.Header{},
		AgentCfg:       defaultAgentCfg(),
	}, busOverride{
		ModelClient: fake,
		Registry:    registry,
	})
	require.NoError(t, err)

	// Verify the caller's request is still EnableMCP=&false (pointer
	// preserved, value unchanged).
	require.NotNil(t, req.EnableMCP, "caller's EnableMCP pointer must survive")
	require.False(t, *req.EnableMCP,
		"caller's EnableMCP value must not be flipped to true")

	// Caller's MCPServers slice must be untouched even though the
	// agent dispatch-path copy gets the curated server appended.
	require.Len(t, req.MCPServers, 1,
		"caller's MCPServers slice must not be mutated")
	require.Equal(t, "https://user-mcp.test", req.MCPServers[0].URL,
		"caller's MCPServers entry must be unchanged")

	// Verify the LegacyDepsProvider closure (which the belt builder
	// would have called) sees an EnableMCP=true copy. We re-derive the
	// closure directly to test the contract in isolation, since the
	// scripted run skipped the belt builder by overriding Registry.
	cp := forceMCPEnabled(req)
	require.NotNil(t, cp.EnableMCP)
	require.True(t, *cp.EnableMCP, "forced copy must have EnableMCP=true")
	require.NotSame(t, req, cp, "forceMCPEnabled must not return the original")
}

// -----------------------------------------------------------------------------
// I5 — client cancellation propagates and the SSE consumer goroutine exits
// -----------------------------------------------------------------------------

// stallingModelClient blocks until the context is cancelled.
type stallingModelClient struct{}

func (stallingModelClient) Stream(ctx context.Context, _ model.Request) (<-chan model.StreamChunk, error) {
	ch := make(chan model.StreamChunk)
	go func() {
		defer close(ch)
		<-ctx.Done()
		// Emit a single error chunk before close so the loop sees a
		// terminal kind. Without this the loop hangs on the channel
		// drain; the model contract demands an explicit terminal chunk.
		select {
		case ch <- model.StreamChunk{Kind: model.ChunkError, Err: ctx.Err()}:
		default:
		}
	}()
	return ch, nil
}

func (stallingModelClient) Capabilities() model.Capabilities {
	return model.Capabilities{SupportsParallelToolCalls: true}
}

func TestI5_Cancellation(t *testing.T) {
	setupTestConfig(t, defaultAgentCfg())

	ctx, _, user := newTestGinCtx(t, "{}")
	on := true
	req := frontendReqAgent(&on, "hang please", nil)

	// Replace ctx.Request with a request whose context can be cancelled.
	reqCtx, cancel := context.WithCancel(ctx.Request.Context())
	ctx.Request = ctx.Request.WithContext(reqCtx)

	// Spawn a goroutine that cancels after a short delay so the loop
	// observes a context error mid-Stream.
	go func() {
		time.Sleep(25 * time.Millisecond)
		cancel()
	}()

	beforeG := runtime.NumGoroutine()

	registry := buildRegistry(t)
	cfg := defaultAgentCfg()
	cfg.WallClockSeconds = 5 // shorter than the test's default but generous
	err := handleAgentWithDeps(ctx, agentRunInputs{
		FrontendReq:    req,
		User:           user,
		ResponsesReq:   &httppkg.OpenAIResponsesReq{Model: "gpt-test"},
		UpstreamHeader: http.Header{},
		AgentCfg:       cfg,
	}, busOverride{
		ModelClient: stallingModelClient{},
		Registry:    registry,
	})
	// The loop returns nil for any clean termination (including
	// cancellation — the cancellation Error event was emitted). The
	// public surface treats cancellation as a clean exit. We just want
	// to make sure the function does not hang.
	_ = err

	// Give goroutines a beat to wind down, then check we did not leak.
	time.Sleep(50 * time.Millisecond)
	afterG := runtime.NumGoroutine()
	require.LessOrEqual(t, afterG-beforeG, 2,
		"goroutine leak after cancellation; before=%d after=%d", beforeG, afterG)
}

// -----------------------------------------------------------------------------
// I6 — hook composition: a custom OnAfterToolCall redactor runs alongside
// the standard chain (verified by intercepting the result before the model
// sees it on the next round)
// -----------------------------------------------------------------------------

func TestI6_HookComposition_Redaction(t *testing.T) {
	setupTestConfig(t, defaultAgentCfg())

	ctx, rec, user := newTestGinCtx(t, "{}")
	on := true
	req := frontendReqAgent(&on, "fetch secret", nil)

	// Round 1: model calls web_fetch (tool returns secret text).
	// Round 2: model calls send_to_user with the (redacted) content.
	scripts := [][]model.StreamChunk{
		{
			{Kind: model.ChunkFunction, FunctionCall: &model.FunctionCall{
				CallID:    "fc-fetch-1",
				Name:      "web_fetch",
				Arguments: rawArgs(t, map[string]any{"url": "https://example.com"}),
			}},
			{Kind: model.ChunkDone},
		},
		{
			{Kind: model.ChunkFunction, FunctionCall: &model.FunctionCall{
				CallID:    "fc-send-1",
				Name:      "send_to_user",
				Arguments: rawArgs(t, map[string]any{"final_answer": "redacted-output"}),
			}},
			{Kind: model.ChunkDone},
		},
	}
	fake := newFakeModelClient(scripts)

	leaked := &fakeTool{name: "web_fetch", output: "SECRET-API-KEY: abc123"}
	registry := buildRegistry(t, leaked)

	var sawRedaction atomic.Int32
	redactor := func(_ context.Context, ev hook.ToolCallEvent) (hook.ToolCallEvent, error) {
		if ev.Result == nil {
			return ev, nil
		}
		if strings.Contains(ev.Result.Content, "SECRET-API-KEY") {
			next := *ev.Result
			next.Content = strings.ReplaceAll(next.Content, "SECRET-API-KEY: abc123", "[REDACTED]")
			ev.Result = &next
			sawRedaction.Add(1)
		}
		return ev, nil
	}

	err := handleAgentWithDeps(ctx, agentRunInputs{
		FrontendReq:    req,
		User:           user,
		ResponsesReq:   &httppkg.OpenAIResponsesReq{Model: "gpt-test"},
		UpstreamHeader: http.Header{},
		AgentCfg:       defaultAgentCfg(),
	}, busOverride{
		ModelClient: fake,
		Registry:    registry,
		PreRegister: func(b *hook.Bus) {
			b.OnAfterToolCall(redactor)
		},
	})
	require.NoError(t, err)

	require.GreaterOrEqual(t, int(sawRedaction.Load()), 1,
		"redactor should fire on the web_fetch result")

	// The final answer is whatever the model said; we just confirm the
	// trace shows the tool call ran and the loop exited.
	body := rec.Body.String()
	require.Contains(t, body, "tool_call: web_fetch")
}

// (I1 — proxy invariance — lives in the http package's
// agent_invariance_test.go because it needs access to unexported
// sendChatWithResponsesToolLoop and the ctxKeyAgentMode constant.
// Keeping it here would require leaking that surface into agentx.)

// -----------------------------------------------------------------------------
// Coercion end-to-end — feed responsesReq.Input with a mixed-shape slice
// (some typed OpenAIResponsesInputMessage, some bare map[string]any items
// — the exact symptom from the live e2e repro after the memory hook
// overwrote Input with PreparedInput at responses_chat_handler.go:789)
// and assert the agent loop reaches Stream with every item already
// coerced into one of the three concrete typed structs the OneAPI
// adapter accepts.
// -----------------------------------------------------------------------------

// recordingModelClient captures the Request.Input it observes on every
// Stream call so the test can assert the wrapper coerced maps to typed
// structs before they reached the model boundary.
type recordingModelClient struct {
	mu       sync.Mutex
	inputs   [][]model.InputItem
	scripts  [][]model.StreamChunk
	calls    int
	caps     model.Capabilities
}

func newRecordingModelClient(scripts [][]model.StreamChunk) *recordingModelClient {
	return &recordingModelClient{
		scripts: scripts,
		caps:    model.Capabilities{SupportsParallelToolCalls: true},
	}
}

func (r *recordingModelClient) Stream(ctx context.Context, req model.Request) (<-chan model.StreamChunk, error) {
	r.mu.Lock()
	idx := r.calls
	r.calls++
	snap := make([]model.InputItem, len(req.Input))
	copy(snap, req.Input)
	r.inputs = append(r.inputs, snap)
	var batch []model.StreamChunk
	if idx < len(r.scripts) {
		batch = r.scripts[idx]
	} else {
		batch = []model.StreamChunk{{Kind: model.ChunkText, Text: ""}, {Kind: model.ChunkDone}}
	}
	r.mu.Unlock()

	ch := make(chan model.StreamChunk, len(batch)+1)
	go func() {
		defer close(ch)
		for _, c := range batch {
			select {
			case <-ctx.Done():
				return
			case ch <- c:
			}
		}
	}()
	return ch, nil
}

func (r *recordingModelClient) Capabilities() model.Capabilities { return r.caps }

func (r *recordingModelClient) snapshotInputs() [][]model.InputItem {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([][]model.InputItem, len(r.inputs))
	for i, slot := range r.inputs {
		cp := make([]model.InputItem, len(slot))
		copy(cp, slot)
		out[i] = cp
	}
	return out
}

func TestI_AgentLoop_CoercesMixedInputBeforeStream(t *testing.T) {
	setupTestConfig(t, defaultAgentCfg())

	ctx, _, user := newTestGinCtx(t, "{}")
	on := true
	req := frontendReqAgent(&on, "hi", nil)

	// Seed responsesReq.Input with the mixed shape that the live e2e
	// repro produces after the memory hook overwrites it: a typed input
	// message (left over from convert2UpstreamResponsesRequest) followed
	// by a map[string]any (the memory hook's PreparedInput shape).
	responsesReq := &httppkg.OpenAIResponsesReq{
		Model: "gpt-test",
		Input: []any{
			httppkg.OpenAIResponsesInputMessage{Role: "system", Content: "be brief"},
			map[string]any{"role": "user", "content": "previous turn"},
		},
	}

	// One-round model: immediately send_to_user. We only need to verify
	// the loop reaches Stream without a model_stream_error.
	scripts := [][]model.StreamChunk{{
		{Kind: model.ChunkFunction, FunctionCall: &model.FunctionCall{
			CallID:    "fc-send-1",
			Name:      "send_to_user",
			Arguments: rawArgs(t, map[string]any{"final_answer": "ok"}),
		}},
		{Kind: model.ChunkDone},
	}}
	rec := newRecordingModelClient(scripts)
	registry := buildRegistry(t)

	err := handleAgentWithDeps(ctx, agentRunInputs{
		FrontendReq:    req,
		User:           user,
		ResponsesReq:   responsesReq,
		UpstreamHeader: http.Header{},
		AgentCfg:       defaultAgentCfg(),
	}, busOverride{
		ModelClient: rec,
		Registry:    registry,
	})
	require.NoError(t, err)

	// At least one Stream call must have landed — i.e. the loop got past
	// step 0, which is the original repro symptom.
	snaps := rec.snapshotInputs()
	require.GreaterOrEqual(t, len(snaps), 1,
		"agent loop should have made at least one upstream call")

	// Every item across every recorded call must be one of the three
	// typed concrete structs the OneAPI validator accepts. The loop
	// itself emits map-shaped userMessage / systemMessage entries — the
	// coercingModelClient wrapper is what converts them.
	for round, items := range snaps {
		for i, it := range items {
			switch it.(type) {
			case httppkg.OpenAIResponsesInputMessage,
				*httppkg.OpenAIResponsesInputMessage,
				httppkg.OpenAIResponsesFunctionCall,
				*httppkg.OpenAIResponsesFunctionCall,
				httppkg.OpenAIResponsesFunctionCallOutput,
				*httppkg.OpenAIResponsesFunctionCallOutput:
				// OK — matches one of the validator's accepted shapes.
			default:
				t.Fatalf("round %d Input[%d] is %T; want one of the three "+
					"typed structs accepted by validateInputItem", round, i, it)
			}
		}
	}

	// Spot-check the first call: the seed's typed system message must
	// survive verbatim, the map "previous turn" user message must have
	// been converted to a typed struct, and the loop's userMessage("hi")
	// (a map[string]any inside the loop) must reach the model boundary
	// as a typed struct too. The exact slot order depends on the
	// OnContext hook chain (the React renderer prepends its own system
	// message) so we scan-by-content rather than asserting indices.
	first := snaps[0]
	require.GreaterOrEqual(t, len(first), 3,
		"first call should carry: react prompt + seed system + prior user + loop's userMessage")

	var foundSeed, foundPriorUser, foundLoopUser bool
	for _, it := range first {
		msg, ok := it.(httppkg.OpenAIResponsesInputMessage)
		if !ok {
			continue
		}
		content, _ := msg.Content.(string)
		switch {
		case msg.Role == "system" && content == "be brief":
			foundSeed = true
		case msg.Role == "user" && content == "previous turn":
			foundPriorUser = true
		case msg.Role == "user" && content == "hi":
			foundLoopUser = true
		}
	}
	require.True(t, foundSeed,
		"seed typed system message ('be brief') must survive the coercion")
	require.True(t, foundPriorUser,
		"map-shaped prior user turn must be coerced into a typed input message")
	require.True(t, foundLoopUser,
		"loop-emitted userMessage('hi') must reach the model as a typed input message")
}

// -----------------------------------------------------------------------------
// Bug 1 (curated belt MCP resolution) — the LegacyDeps the agent dispatch
// path hands to ExecuteToolCallCtx must carry the curated MCP server in
// FrontendReq.MCPServers; without it findMCPServerForToolName returns nil
// and the legacy dispatcher rejects every curated belt call with
// "tool X not found in enabled MCP servers".
// -----------------------------------------------------------------------------

// TestForceMCPEnabledWithCuratedServer_AddsCuratedServer asserts the
// helper appends the curated server to a fresh MCPServers slice when the
// caller's request carries none, leaving the caller's request untouched.
func TestForceMCPEnabledWithCuratedServer_AddsCuratedServer(t *testing.T) {
	off := false
	req := &httppkg.FrontendReq{EnableMCP: &off}
	curated := &httppkg.MCPServerConfig{
		URL:     "https://mcp.curated.test",
		Enabled: true,
	}

	cp := forceMCPEnabledWithCuratedServer(req, curated)
	require.NotNil(t, cp)
	require.NotSame(t, req, cp, "copy must not alias the caller's request")
	require.NotNil(t, cp.EnableMCP)
	require.True(t, *cp.EnableMCP, "copy must force EnableMCP=true")

	require.Len(t, cp.MCPServers, 1, "curated server must be appended")
	require.Equal(t, "https://mcp.curated.test", cp.MCPServers[0].URL)
	require.True(t, cp.MCPServers[0].Enabled)

	// Caller's request untouched.
	require.Nil(t, req.MCPServers, "caller's MCPServers must stay nil")
	require.NotNil(t, req.EnableMCP)
	require.False(t, *req.EnableMCP,
		"caller's EnableMCP must stay false")
}

// TestForceMCPEnabledWithCuratedServer_DedupesByURL asserts that when
// the caller already supplies a server with the same URL as the curated
// server, the helper does not append a duplicate.
func TestForceMCPEnabledWithCuratedServer_DedupesByURL(t *testing.T) {
	on := true
	req := &httppkg.FrontendReq{
		EnableMCP: &on,
		MCPServers: []httppkg.MCPServerConfig{
			{URL: "https://mcp.curated.test", Enabled: true, APIKey: "user-key"},
		},
	}
	curated := &httppkg.MCPServerConfig{
		URL:     "https://mcp.curated.test",
		Enabled: true,
	}

	cp := forceMCPEnabledWithCuratedServer(req, curated)
	require.Len(t, cp.MCPServers, 1,
		"duplicate URL must not be appended")
	require.Equal(t, "user-key", cp.MCPServers[0].APIKey,
		"caller's entry must be preserved (with its APIKey)")
}

// TestForceMCPEnabledWithCuratedServer_NilCurated falls back to plain
// forceMCPEnabled when no curated server is configured.
func TestForceMCPEnabledWithCuratedServer_NilCurated(t *testing.T) {
	off := false
	req := &httppkg.FrontendReq{EnableMCP: &off}
	cp := forceMCPEnabledWithCuratedServer(req, nil)
	require.NotNil(t, cp)
	require.NotNil(t, cp.EnableMCP)
	require.True(t, *cp.EnableMCP)
	require.Nil(t, cp.MCPServers,
		"no curated server => no MCPServers injection")
}

// TestPopulateCuratedServerTools_FiltersBySource asserts only curated_mcp
// tools are surfaced in the server's Tools field — local tools
// (send_to_user, spawn_agent) must NOT be claimed by the MCP server.
func TestPopulateCuratedServerTools_FiltersBySource(t *testing.T) {
	server := &httppkg.MCPServerConfig{URL: "https://mcp.test", Enabled: true}

	logger := newTestLogger(t)
	reg := tool.NewRegistry(logger)
	require.NoError(t, reg.Register(&fakeTool{name: "send_to_user"}, tool.SourceLocal))
	require.NoError(t, reg.Register(&fakeTool{name: "web_search"}, tool.SourceCuratedMCP))
	require.NoError(t, reg.Register(&fakeTool{name: "web_fetch"}, tool.SourceCuratedMCP))

	populateCuratedServerTools(server, reg)
	require.Len(t, server.Tools, 2,
		"only curated_mcp tools should be added; send_to_user is local")

	// Decode each raw tool definition and assert the names line up.
	names := make([]string, 0, len(server.Tools))
	for _, raw := range server.Tools {
		var m map[string]string
		require.NoError(t, stdjson.Unmarshal(raw, &m))
		names = append(names, m["name"])
	}
	require.ElementsMatch(t, []string{"web_search", "web_fetch"}, names)
}

// newTestLogger constructs a console logger gated to error-level so
// tests stay quiet. Returned as a single value rather than a tuple so
// future call sites do not have to discard a placeholder.
func newTestLogger(t *testing.T) glog.Logger {
	t.Helper()
	l, err := glog.NewConsoleWithName("agentx_test", glog.LevelError)
	require.NoError(t, err)
	return l
}
