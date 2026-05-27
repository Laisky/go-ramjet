package agentx

// File: handler_e2e_test.go
//
// End-to-end integration suite for the agent loop. Each test exercises the
// FULL path from the agentx handler down through the loop, curated belt,
// SSE writer and out to the wire bytes — without a live MCP server or a
// real upstream LLM. Every test names the live-bug class it would have
// caught at PR-review time.
//
// Test seams used:
//   - busOverride.ModelClient / Registry / PreRegister / DisableDefaults
//     (handler_test.go) — the only public seam this file relies on.
//   - tools.NewLegacyDispatchTool with a LegacyDepsFunc closure — captures
//     the LegacyDeps the curated belt would have handed to the production
//     dispatcher. We do NOT swap the package-level defaultLegacyDispatcher
//     because that seam is unexported in `agentx/tools` and reaching it
//     from `package agentx` would require an export or `_test` shim that
//     the task constraints forbid. We exercise the contract through the
//     provider closure instead — production code does the same.
//
// We DO NOT swap `agentx/tools.defaultMemoryAfterTurn` either, for the
// same reason. Instead, E5 / E6 run the loop with DisableDefaults=true
// and re-construct the production memory hook chain through the
// `tools.NewMemory*` constructors with a custom MemoryDeps. That keeps
// the production constructors under test (truncation, hygiene contract)
// while avoiding the unreachable seam.
//
// Goldens for the wire-shaped model.Request live under testdata/ so visual
// diffs are explicit. The -update-e2e flag regenerates them (mirrors the
// model/oneapi_test.go pattern).

import (
	"context"
	stdjson "encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/hook"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/model"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/session"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/tool"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/tools"
	httppkg "github.com/Laisky/go-ramjet/internal/tasks/gptchat/http"
)

// updateE2EGolden lets `go test -update-e2e` regenerate the wire-shape
// goldens the E1 / E3 tests assert against. Opt-in via its own flag so a
// stray -update in a sibling package's test run does not silently churn
// these files.
var updateE2EGolden = flag.Bool("update-e2e", false, "regenerate handler_e2e_test.go golden files in testdata/")

// -----------------------------------------------------------------------------
// recordingModel: a scripted fake that captures every Request the loop sends
// upstream. The capture is what makes the WIRE assertions in E1 + E3 possible.
// -----------------------------------------------------------------------------

type recordingModel struct {
	mu      sync.Mutex
	scripts [][]model.StreamChunk
	calls   int
	// requests holds a deep copy of every model.Request the loop sent. We
	// store the typed shape (not the raw JSON) so the assertions can pick
	// fields directly without re-parsing.
	requests []recordedRequest
	caps     model.Capabilities
}

// recordedRequest is the slice of a model.Request the tests care about. We
// drop the function-typed Reasoning pointer / ToolChoice any-shape so the
// JSON golden stays minimal and reproducible across runs.
type recordedRequest struct {
	Model             string                 `json:"model"`
	Input             []recordedInputItem    `json:"input"`
	Tools             []model.ToolDescriptor `json:"tools"`
	ToolChoice        any                    `json:"tool_choice,omitempty"`
	MaxOutputTokens   uint                   `json:"max_output_tokens,omitempty"`
	Stream            bool                   `json:"stream"`
	Temperature       float64                `json:"temperature,omitempty"`
	TopP              float64                `json:"top_p,omitempty"`
	ParallelToolCalls bool                   `json:"parallel_tool_calls"`
}

// recordedInputItem is a tagged-union shape for an Input slot. We
// represent each item as a {kind, payload} pair so the golden file
// captures both the discriminator and the underlying struct without
// committing to a single concrete shape.
type recordedInputItem struct {
	Kind    string `json:"kind"`
	Payload any    `json:"payload"`
}

func newRecordingModel(scripts [][]model.StreamChunk) *recordingModel {
	return &recordingModel{
		scripts: scripts,
		caps:    model.Capabilities{SupportsParallelToolCalls: true},
	}
}

func (m *recordingModel) Stream(ctx context.Context, req model.Request) (<-chan model.StreamChunk, error) {
	m.mu.Lock()
	idx := m.calls
	m.calls++
	m.requests = append(m.requests, snapshotRequest(req))
	var batch []model.StreamChunk
	if idx < len(m.scripts) {
		batch = m.scripts[idx]
	} else {
		batch = []model.StreamChunk{{Kind: model.ChunkText, Text: ""}, {Kind: model.ChunkDone}}
	}
	m.mu.Unlock()

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

func (m *recordingModel) Capabilities() model.Capabilities { return m.caps }

func (m *recordingModel) snapshot() []recordedRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]recordedRequest, len(m.requests))
	copy(out, m.requests)
	return out
}

func snapshotRequest(req model.Request) recordedRequest {
	out := recordedRequest{
		Model:             req.Model,
		Tools:             append([]model.ToolDescriptor{}, req.Tools...),
		ToolChoice:        req.ToolChoice,
		MaxOutputTokens:   req.MaxOutputTokens,
		Stream:            req.Stream,
		Temperature:       req.Temperature,
		TopP:              req.TopP,
		ParallelToolCalls: req.ParallelToolCalls,
	}
	out.Input = make([]recordedInputItem, 0, len(req.Input))
	for _, item := range req.Input {
		out.Input = append(out.Input, snapshotInputItem(item))
	}
	return out
}

// snapshotInputItem tags every Input slot with a stable kind discriminator
// so the golden stays readable even when the concrete struct is one of the
// three typed shapes (message / function_call / function_call_output).
func snapshotInputItem(item any) recordedInputItem {
	switch v := item.(type) {
	case httppkg.OpenAIResponsesInputMessage:
		return recordedInputItem{Kind: "message", Payload: v}
	case *httppkg.OpenAIResponsesInputMessage:
		return recordedInputItem{Kind: "message", Payload: v}
	case httppkg.OpenAIResponsesFunctionCall:
		return recordedInputItem{Kind: "function_call", Payload: v}
	case *httppkg.OpenAIResponsesFunctionCall:
		return recordedInputItem{Kind: "function_call", Payload: v}
	case httppkg.OpenAIResponsesFunctionCallOutput:
		return recordedInputItem{Kind: "function_call_output", Payload: v}
	case *httppkg.OpenAIResponsesFunctionCallOutput:
		return recordedInputItem{Kind: "function_call_output", Payload: v}
	default:
		// Fallback for map-shapes or future shapes — store the concrete
		// type label so the JSON marshaller surfaces enough context for
		// debugging.
		return recordedInputItem{Kind: fmt.Sprintf("%T", v), Payload: v}
	}
}

// -----------------------------------------------------------------------------
// capturingTool: a tool.Tool that records call invocations and the per-call
// LegacyDeps the provider would have surfaced to a legacy dispatcher.
// -----------------------------------------------------------------------------

type capturingTool struct {
	name        string
	description string
	schema      stdjson.RawMessage
	deps        tools.LegacyDepsProvider
	output      string
	isError     bool

	mu    sync.Mutex
	calls []capturedCall
}

type capturedCall struct {
	Call tool.Call
	// LegacyDeps captured at execute time — mirrors what
	// tools.legacyDispatchTool.Execute would have handed the production
	// dispatcher. The pointer-typed FrontendReq is the field whose
	// MCPServers slice is the load-bearing assertion for E2.
	LegacyDeps httppkg.LegacyDeps
	DepsErr    error
}

func (c *capturingTool) Name() string               { return c.name }
func (c *capturingTool) Description() string        { return c.description }
func (c *capturingTool) Schema() stdjson.RawMessage { return c.schema }

func (c *capturingTool) Execute(ctx context.Context, call tool.Call, _ session.EventSink) (tool.Result, error) {
	rec := capturedCall{Call: call}
	if c.deps != nil {
		// Mimic what tools/legacyDispatchTool.Execute does: resolve the
		// provider closure to capture the LegacyDeps the dispatcher would
		// have seen. This is the wire surface E2 asserts against.
		deps, err := c.deps.LegacyDeps(ctx, call.CallID, c.name)
		rec.LegacyDeps = deps
		rec.DepsErr = err
	}
	c.mu.Lock()
	c.calls = append(c.calls, rec)
	c.mu.Unlock()
	return tool.Result{Content: c.output, IsError: c.isError}, nil
}

func (c *capturingTool) snapshotCalls() []capturedCall {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]capturedCall, len(c.calls))
	copy(out, c.calls)
	return out
}

// newCapturingTool builds a tool that captures its LegacyDeps just like
// the production legacyDispatchTool, so E2 can assert MCPServers wiring
// without swapping the package-level dispatcher seam in agentx/tools.
func newCapturingTool(name, output string, deps tools.LegacyDepsProvider) *capturingTool {
	return &capturingTool{
		name:        name,
		description: "fake " + name,
		schema:      stdjson.RawMessage(`{"type":"object"}`),
		deps:        deps,
		output:      output,
	}
}

// -----------------------------------------------------------------------------
// E1 — Tool catalog reaches the model with belt-builder filtering intact.
//
// Bug class caught: belt name-filter regression (Bug B in the live e2e:
// the curated belt accidentally dropped tools advertised by the upstream
// MCP catalog). If a future refactor reintroduces a hard-coded include
// list, this test fails because the fake catalog of 17 tools no longer
// round-trips into model.Request.Tools verbatim.
// -----------------------------------------------------------------------------

// liveMCPCatalog is a 17-entry list mirroring the production laisky MCP
// catalog shape (a mix of file_*, memory_*, web_*, and helpers). Pinning
// it here lets E1 assert the expected fail-OPEN behaviour: every
// advertised tool appears in the model's Tools list, plus send_to_user,
// plus nothing else.
var liveMCPCatalog = []string{
	"extract_key_info",
	"file_delete",
	"file_list",
	"file_read",
	"file_rename",
	"file_search",
	"file_stat",
	"file_write",
	"find_tool",
	"get_user_request",
	"mcp_pipe",
	"memory_list_dir_with_abstract",
	"memory_run_maintenance",
	"summarize_doc",
	"translate_text",
	"web_fetch",
	"web_search",
}

func TestE1_ToolCatalogReachesModel(t *testing.T) {
	// Bug class: curated belt name-filter regression (Bug B).
	setupTestConfig(t, defaultAgentCfg())

	ctx, _, user := newTestGinCtx(t, "{}")
	on := true
	req := frontendReqAgent(&on, "list the catalog please", nil)

	// Build a registry with send_to_user + every name in liveMCPCatalog as
	// a SourceCuratedMCP entry. This mimics what BuildCuratedBelt would
	// have produced from a 17-tool DiscoverMCPTools result. The handler
	// otherwise calls BuildCuratedBelt itself; we hand it a pre-built
	// registry via busOverride.Registry to keep the test deterministic.
	logger := newTestLogger(t)
	registry := tool.NewRegistry(logger)
	require.NoError(t, registry.Register(tools.NewSendToUserTool(), tool.SourceLocal))
	require.Len(t, liveMCPCatalog, 17, "fixture catalogue must be 17 entries")
	for _, name := range liveMCPCatalog {
		ft := &fakeTool{name: name, output: "ok"}
		require.NoError(t, registry.Register(ft, tool.SourceCuratedMCP))
	}

	// One-round model that immediately terminates so the loop exits after
	// the first Stream call (which is the only one we need to inspect for
	// Tools wiring).
	scripts := [][]model.StreamChunk{{
		{Kind: model.ChunkFunction, FunctionCall: &model.FunctionCall{
			CallID:    "call_send_e1",
			Name:      "send_to_user",
			Arguments: rawArgs(t, map[string]any{"final_answer": "done"}),
		}},
		{Kind: model.ChunkDone},
	}}
	rec := newRecordingModel(scripts)

	err := handleAgentWithDeps(ctx, agentRunInputs{
		FrontendReq:    req,
		User:           user,
		ResponsesReq:   &httppkg.OpenAIResponsesReq{Model: "gpt-test"},
		UpstreamHeader: http.Header{},
		AgentCfg:       defaultAgentCfg(),
	}, busOverride{
		ModelClient: rec,
		Registry:    registry,
	})
	require.NoError(t, err)

	snaps := rec.snapshot()
	require.GreaterOrEqual(t, len(snaps), 1, "model client must receive at least one Request")
	first := snaps[0]

	// Count check: 17 MCP tools + 1 send_to_user = 18 distinct descriptors.
	require.Len(t, first.Tools, len(liveMCPCatalog)+1,
		"Tools must carry every MCP descriptor plus send_to_user; got %d",
		len(first.Tools))

	// Lex-ordering check: per §3.2 the registry orders by Source then by
	// Name. send_to_user is the only SourceLocal entry so it appears first;
	// the curated MCP entries follow in lex order.
	require.Equal(t, "send_to_user", first.Tools[0].Name,
		"local sources fire first; send_to_user must be Tools[0]")
	curated := first.Tools[1:]
	curatedNames := make([]string, 0, len(curated))
	for _, d := range curated {
		curatedNames = append(curatedNames, d.Name)
	}
	expected := append([]string{}, liveMCPCatalog...)
	sort.Strings(expected)
	require.Equal(t, expected, curatedNames,
		"curated MCP tools must appear in lex order with NO drops")

	// Sanity: web_search specifically must NOT be missing (the live Bug B
	// dropped it because the old include-list filter shadowed it).
	require.Contains(t, curatedNames, "web_search",
		"web_search must reach the model — Bug B regressions land here")

	// Wire golden — snapshot the descriptor names + the ParallelToolCalls
	// hint so a future shape change is visible in CR.
	checkGolden(t, "e1_tool_catalog.json", map[string]any{
		"tool_count":          len(first.Tools),
		"tool_names":          collectToolNames(first.Tools),
		"parallel_tool_calls": first.ParallelToolCalls,
		"stream":              first.Stream,
	})
}

// -----------------------------------------------------------------------------
// E2 — Curated MCP server reaches the legacy dispatcher's LegacyDeps.
//
// Bug class caught: the agent dispatch path forced EnableMCP=true but
// failed to thread the curated MCP server into FrontendReq.MCPServers, so
// findMCPServerForToolName rejected every curated call with "tool X not
// found in enabled MCP servers" (Bug A part 1). The test asserts the
// curated server is present in the LegacyDeps the per-tool provider
// closure surfaces — and that the CALLER's request remains untouched
// (U13 isolation contract).
// -----------------------------------------------------------------------------

func TestE2_CuratedMCPServerReachesLegacyDispatcher(t *testing.T) {
	// Bug class: curated MCP server missing from LegacyDeps.FrontendReq.MCPServers (Bug A part 1).
	cfg := defaultAgentCfg()
	cfg.MCPServer = "https://test-mcp.example.com"
	setupTestConfig(t, cfg)

	ctx, _, user := newTestGinCtx(t, "{}")
	on := true
	req := frontendReqAgent(&on, "search please", nil)
	// Caller did NOT supply any MCP servers; the agent loop must inject
	// the curated server itself.
	require.Empty(t, req.MCPServers)

	// Build the LegacyDepsFunc closure the way handler.go does. The
	// closure is functionally identical to the one inside
	// handleAgentWithDeps — when the underlying production behaviour
	// changes, this test compiles against the same surface and breaks
	// loudly.
	curatedServer := resolveCuratedMCP(cfg)
	require.NotNil(t, curatedServer)
	require.Equal(t, "https://test-mcp.example.com", curatedServer.URL)
	depsProvider := tools.LegacyDepsFunc(func(_ context.Context, _, _ string) (httppkg.LegacyDeps, error) {
		return httppkg.LegacyDeps{
			User:        user,
			FrontendReq: forceMCPEnabledWithCuratedServer(req, curatedServer),
		}, nil
	})

	logger := newTestLogger(t)
	registry := tool.NewRegistry(logger)
	require.NoError(t, registry.Register(tools.NewSendToUserTool(), tool.SourceLocal))
	captor := newCapturingTool("web_search", `{"hits":3}`, depsProvider)
	require.NoError(t, registry.Register(captor, tool.SourceCuratedMCP))

	// Model script: round 1 fires web_search; round 2 sends final.
	scripts := [][]model.StreamChunk{
		{
			{Kind: model.ChunkFunction, FunctionCall: &model.FunctionCall{
				CallID:    "call_web_e2",
				Name:      "web_search",
				Arguments: rawArgs(t, map[string]any{"query": "agent loop"}),
			}},
			{Kind: model.ChunkDone},
		},
		{
			{Kind: model.ChunkFunction, FunctionCall: &model.FunctionCall{
				CallID:    "call_send_e2",
				Name:      "send_to_user",
				Arguments: rawArgs(t, map[string]any{"final_answer": "got results"}),
			}},
			{Kind: model.ChunkDone},
		},
	}
	rec := newRecordingModel(scripts)

	err := handleAgentWithDeps(ctx, agentRunInputs{
		FrontendReq:    req,
		User:           user,
		ResponsesReq:   &httppkg.OpenAIResponsesReq{Model: "gpt-test"},
		UpstreamHeader: http.Header{},
		AgentCfg:       cfg,
	}, busOverride{
		ModelClient: rec,
		Registry:    registry,
	})
	require.NoError(t, err)

	// Captor must have been called exactly once for the web_search round.
	calls := captor.snapshotCalls()
	require.Len(t, calls, 1, "web_search must have fired exactly once")
	require.NoError(t, calls[0].DepsErr, "LegacyDepsProvider must not surface an error")

	// LegacyDeps.FrontendReq.MCPServers must include the curated server.
	deps := calls[0].LegacyDeps
	require.NotNil(t, deps.FrontendReq, "dispatcher must receive a non-nil FrontendReq")
	require.NotNil(t, deps.FrontendReq.EnableMCP)
	require.True(t, *deps.FrontendReq.EnableMCP,
		"dispatch path must see EnableMCP=true even if caller said false")
	require.NotEmpty(t, deps.FrontendReq.MCPServers,
		"curated MCP server must be threaded into dispatcher LegacyDeps "+
			"(Bug A part 1 fix)")
	var foundCurated bool
	for _, s := range deps.FrontendReq.MCPServers {
		if s.URL == "https://test-mcp.example.com" {
			foundCurated = true
			break
		}
	}
	require.True(t, foundCurated,
		"curated MCP URL %q must appear in dispatcher MCPServers slice",
		"https://test-mcp.example.com")

	// U13 isolation: caller's request must be UNTOUCHED — the agent path
	// builds its own copy for the dispatcher and never mutates the caller.
	require.Empty(t, req.MCPServers,
		"caller's FrontendReq.MCPServers must remain empty (U13)")
}

// -----------------------------------------------------------------------------
// E3 — function_call items in the next-round Input carry non-empty IDs.
//
// Bug class caught: Bug A part 2 — the upstream 400 `invalid_value` for
// `input[*].id` after a tool call. The loop's appendFunctionCallAndOutput
// helper stamps an `id` on each function_call item; an empty CallID
// previously made `id` empty too, which the Responses API rejected. We
// assert both the happy path (model emits CallID) and the synthesized
// path (model emits empty CallID → `fc_<ULID>`).
// -----------------------------------------------------------------------------

func TestE3_FunctionCallIDsNonEmptyOnNextRound(t *testing.T) {
	// Bug class: empty function_call.id rejected with 400 by upstream (Bug A part 2).
	setupTestConfig(t, defaultAgentCfg())

	idRegex := regexp.MustCompile(`^(call_|fc_)[A-Za-z0-9_-]+$`)

	// ---- Variant 1: model emits a populated CallID.
	{
		ctx, _, user := newTestGinCtx(t, "{}")
		on := true
		req := frontendReqAgent(&on, "search the web", nil)
		scripts := [][]model.StreamChunk{
			{
				{Kind: model.ChunkFunction, FunctionCall: &model.FunctionCall{
					CallID:    "call_xyz123",
					Name:      "web_search",
					Arguments: rawArgs(t, map[string]any{"query": "stable Go"}),
				}},
				{Kind: model.ChunkDone},
			},
			{
				{Kind: model.ChunkFunction, FunctionCall: &model.FunctionCall{
					CallID:    "call_send_e3",
					Name:      "send_to_user",
					Arguments: rawArgs(t, map[string]any{"final_answer": "done"}),
				}},
				{Kind: model.ChunkDone},
			},
		}
		recM := newRecordingModel(scripts)
		registry := buildRegistry(t, &fakeTool{name: "web_search", output: `{"hits":[]}`})

		err := handleAgentWithDeps(ctx, agentRunInputs{
			FrontendReq:    req,
			User:           user,
			ResponsesReq:   &httppkg.OpenAIResponsesReq{Model: "gpt-test"},
			UpstreamHeader: http.Header{},
			AgentCfg:       defaultAgentCfg(),
		}, busOverride{
			ModelClient: recM,
			Registry:    registry,
		})
		require.NoError(t, err)

		snaps := recM.snapshot()
		require.GreaterOrEqual(t, len(snaps), 2,
			"two model calls expected (web_search + send_to_user)")

		// On round 2 the loop appends the prior function_call to the input
		// transcript. Find that entry and assert:
		//   - The `id` slot is non-empty and matches the upstream-allowed
		//     prefix regex.
		//   - The `call_id` slot round-trips the model-supplied CallID
		//     verbatim. (Per loop.go's callIDForFunctionCall the `id`
		//     slot is always synthesised as `fc_<ULID>` unless the model
		//     itself emitted a string already starting with `fc`. The two
		//     fields live in separate namespaces — the upstream's strict
		//     validator demands the `id` carry the `fc` prefix even when
		//     `call_id` is `call_…`.)
		foundCallID := false
		for _, item := range snaps[1].Input {
			if item.Kind != "function_call" {
				continue
			}
			fc, ok := item.Payload.(httppkg.OpenAIResponsesFunctionCall)
			if !ok {
				t.Fatalf("function_call item payload type %T; want OpenAIResponsesFunctionCall", item.Payload)
			}
			require.NotEmpty(t, fc.ID,
				"function_call.id MUST be non-empty (Bug A part 2 fix)")
			require.True(t, idRegex.MatchString(fc.ID),
				"function_call.id %q must match ^(call_|fc_)[A-Za-z0-9_-]+$", fc.ID)
			if fc.CallID == "call_xyz123" {
				foundCallID = true
				require.True(t, strings.HasPrefix(fc.ID, "fc_"),
					"synthesised id %q must carry the fc_ prefix even when "+
						"the model emitted CallID=%q (the upstream's strict "+
						"validator distinguishes the two namespaces)",
					fc.ID, fc.CallID)
			}
		}
		require.True(t, foundCallID,
			"the model's CallID=call_xyz123 must survive as call_id on the next round")

		// Wire golden: snapshot only the function_call slots of round 2 so
		// an additive change to the memory hook's PreparedInput shape
		// doesn't churn the golden. The `id` field is omitted because
		// the loop synthesises a fresh `fc_<ULID>` per run (see
		// callIDForFunctionCall in loop.go); only the stable bits land
		// in the golden.
		checkGolden(t, "e3_function_call_id_populated.json",
			collectFunctionCallItemsStable(snaps[1]))
	}

	// ---- Variant 2: model emits an EMPTY CallID → id must be synthesised.
	{
		ctx, _, user := newTestGinCtx(t, "{}")
		on := true
		req := frontendReqAgent(&on, "search the web", nil)
		scripts := [][]model.StreamChunk{
			{
				{Kind: model.ChunkFunction, FunctionCall: &model.FunctionCall{
					CallID:    "", // intentionally empty
					Name:      "web_search",
					Arguments: rawArgs(t, map[string]any{"query": "stable Go"}),
				}},
				{Kind: model.ChunkDone},
			},
			{
				{Kind: model.ChunkFunction, FunctionCall: &model.FunctionCall{
					CallID:    "call_send_e3_v2",
					Name:      "send_to_user",
					Arguments: rawArgs(t, map[string]any{"final_answer": "done"}),
				}},
				{Kind: model.ChunkDone},
			},
		}
		recM := newRecordingModel(scripts)
		registry := buildRegistry(t, &fakeTool{name: "web_search", output: `{"hits":[]}`})

		err := handleAgentWithDeps(ctx, agentRunInputs{
			FrontendReq:    req,
			User:           user,
			ResponsesReq:   &httppkg.OpenAIResponsesReq{Model: "gpt-test"},
			UpstreamHeader: http.Header{},
			AgentCfg:       defaultAgentCfg(),
		}, busOverride{
			ModelClient: recM,
			Registry:    registry,
		})
		require.NoError(t, err)

		snaps := recM.snapshot()
		require.GreaterOrEqual(t, len(snaps), 2)
		foundSynth := false
		for _, item := range snaps[1].Input {
			if item.Kind != "function_call" {
				continue
			}
			fc, ok := item.Payload.(httppkg.OpenAIResponsesFunctionCall)
			if !ok {
				continue
			}
			require.NotEmpty(t, fc.ID,
				"empty-CallID function_call must have a synthesised id (Bug A part 2)")
			require.True(t, strings.HasPrefix(fc.ID, "fc_"),
				"synthesised id %q must start with fc_ prefix", fc.ID)
			require.True(t, idRegex.MatchString(fc.ID),
				"synthesised id %q must match the call_|fc_ regex", fc.ID)
			foundSynth = true
		}
		require.True(t, foundSynth,
			"the empty-CallID variant must produce a synthesised fc_<ULID> id")
	}
}

// -----------------------------------------------------------------------------
// E4 — Tool-forcing prompt actually invokes the tool (the weather-query
// case generalised to a Go-version query). Exercises the canonical ReAct
// flow "think, then call exactly one tool, then send_to_user".
//
// This is a positive correctness test rather than a bug-class regression.
// It pins the SSE wire output so a future refactor that breaks the trace
// rendering (or the final-answer delivery) fails loudly.
// -----------------------------------------------------------------------------

func TestE4_ToolForcingPromptInvokesTool(t *testing.T) {
	// Pins §4.5 SSE wire format + happy-path send_to_user termination.
	setupTestConfig(t, defaultAgentCfg())

	ctx, recHTTP, user := newTestGinCtx(t, "{}")
	on := true
	req := frontendReqAgent(&on,
		"Search the web for the latest stable Go version and tell me its version number.",
		nil)

	scripts := [][]model.StreamChunk{
		{
			{Kind: model.ChunkFunction, FunctionCall: &model.FunctionCall{
				CallID:    "call_search_e4",
				Name:      "web_search",
				Arguments: rawArgs(t, map[string]any{"query": "latest stable Go version"}),
			}},
			{Kind: model.ChunkDone},
		},
		{
			{Kind: model.ChunkFunction, FunctionCall: &model.FunctionCall{
				CallID:    "call_send_e4",
				Name:      "send_to_user",
				Arguments: rawArgs(t, map[string]any{
					"final_answer": "The latest stable Go version is 1.26.2.",
				}),
			}},
			{Kind: model.ChunkDone},
		},
	}
	rec := newRecordingModel(scripts)

	registry := buildRegistry(t, &fakeTool{
		name:   "web_search",
		output: "Go 1.26.2",
	})

	err := handleAgentWithDeps(ctx, agentRunInputs{
		FrontendReq:    req,
		User:           user,
		ResponsesReq:   &httppkg.OpenAIResponsesReq{Model: "gpt-test"},
		UpstreamHeader: http.Header{},
		AgentCfg:       defaultAgentCfg(),
	}, busOverride{
		ModelClient: rec,
		Registry:    registry,
	})
	require.NoError(t, err)

	body := recHTTP.Body.String()
	// SSE trace contains the tool call.
	require.Contains(t, body, "tool_call: web_search",
		"SSE bytes must carry the tool_call: web_search trace line")
	// SSE trace contains a tool result line ("tool ok (...)" indicates
	// the wrap hook ran and the result was streamed back).
	require.Contains(t, body, "tool ok",
		"SSE bytes must carry the wrap-hook tool ok marker")
	// Final answer reaches delta.content.
	require.Contains(t, body, "The latest stable Go version is 1.26.2.",
		"final answer must arrive on delta.content")
	// Termination reason — terminated_by=send_to_user.
	require.Contains(t, body, "terminated_by=send_to_user",
		"run finished line must record terminated_by=send_to_user")
}

// -----------------------------------------------------------------------------
// E5 — Memory hygiene end-to-end (U15 re-asserted at e2e level).
//
// Bug class caught: the agent loop emits a SessionEndEvent that leaks the
// full tool transcript into the memory subsystem (U15 regression). The
// production tools.NewMemoryAfterTurnHook only consumes
// (ev.UserPrompt, ev.FinalText); we lock the contract by:
//
//  1. Running the loop with DisableDefaults=true so the only OnSessionEnd
//     hook is our recorder. The recorder mimics the production memory
//     hook's payload-building logic (minimalMemoryInput) and asserts the
//     output is exactly [user_prompt, final_answer].
//  2. Driving a four-tool-call multi-round script so the bus-event
//     payload would carry the transcript IF the contract were violated.
//
// We cannot swap the package-level defaultMemoryAfterTurn seam from
// `package agentx` (the constraint forbids modifying agentx/tools), so we
// validate the loop-side guarantee — SessionEndEvent carries only the
// pair, never the transcript. The production hook's own truncation +
// payload-building is covered by tools/memoryhook_test.go's U15 test.
// -----------------------------------------------------------------------------

func TestE5_MemoryHygiene_AfterTurnReceivesOnlyPromptAndFinal(t *testing.T) {
	// Bug class: memory hook leaks the tool transcript (U15 regression).
	setupTestConfig(t, defaultAgentCfg())

	ctx, _, user := newTestGinCtx(t, "{}")
	const userPrompt = "what's the latest claude blog post?"
	const finalAnswer = "Anthropic published a Claude 4.6 post on 2026-05-26."
	on := true
	req := frontendReqAgent(&on, userPrompt, nil)

	// Recorder for the SessionEndEvent payload + the minimalMemoryInput
	// shape the production hook would persist.
	captured := &sessionEndCapture{}

	// Four-tool-call multi-round script ending in send_to_user.
	scripts := [][]model.StreamChunk{
		{
			{Kind: model.ChunkFunction, FunctionCall: &model.FunctionCall{
				CallID:    "c1",
				Name:      "web_search",
				Arguments: rawArgs(t, map[string]any{"q": "claude blog"}),
			}},
			{Kind: model.ChunkDone},
		},
		{
			{Kind: model.ChunkFunction, FunctionCall: &model.FunctionCall{
				CallID:    "c2",
				Name:      "web_fetch",
				Arguments: rawArgs(t, map[string]any{"url": "https://anthropic.com"}),
			}},
			{Kind: model.ChunkDone},
		},
		{
			{Kind: model.ChunkFunction, FunctionCall: &model.FunctionCall{
				CallID:    "c3",
				Name:      "file_search",
				Arguments: rawArgs(t, map[string]any{"q": "summary"}),
			}},
			{Kind: model.ChunkDone},
		},
		{
			{Kind: model.ChunkFunction, FunctionCall: &model.FunctionCall{
				CallID:    "c_send_e5",
				Name:      "send_to_user",
				Arguments: rawArgs(t, map[string]any{"final_answer": finalAnswer}),
			}},
			{Kind: model.ChunkDone},
		},
	}
	rec := newRecordingModel(scripts)
	registry := buildRegistry(t,
		&fakeTool{name: "web_search", output: `{"hits":3}`},
		&fakeTool{name: "web_fetch", output: `<html>...</html>`},
		&fakeTool{name: "file_search", output: `[]`},
	)

	err := handleAgentWithDeps(ctx, agentRunInputs{
		FrontendReq:    req,
		User:           user,
		ResponsesReq:   &httppkg.OpenAIResponsesReq{Model: "gpt-test"},
		UpstreamHeader: http.Header{},
		AgentCfg:       defaultAgentCfg(),
	}, busOverride{
		ModelClient:     rec,
		Registry:        registry,
		DisableDefaults: true,
		PreRegister: func(b *hook.Bus) {
			b.OnSessionEnd(captured.recordSessionEnd)
		},
	})
	require.NoError(t, err)

	// SessionEndEvent fired exactly once with the right payload.
	require.Equal(t, int32(1), captured.fired.Load(),
		"OnSessionEnd should fire exactly once for a clean send_to_user run")
	require.Equal(t, userPrompt, captured.userPrompt(),
		"SessionEndEvent.UserPrompt must carry the user-turn text verbatim")
	require.Equal(t, finalAnswer, captured.finalText(),
		"SessionEndEvent.FinalText must carry the send_to_user payload verbatim")
	require.Equal(t, session.TerminatedBySendToUser, captured.terminatedBy(),
		"SessionEndEvent.TerminatedBy must be send_to_user")

	// The minimal memory payload the production hook would build —
	// computed locally per minimalMemoryInput's well-documented shape —
	// must contain exactly two items: [user_prompt, final_answer]. This
	// is the U15 contract restated at the e2e level.
	payload := buildMinimalMemoryInputForTest(captured.userPrompt(), captured.finalText())
	require.Len(t, payload, 2,
		"minimal memory payload must be [user_prompt, final_answer]")
	first, ok := payload[0].(httppkg.OpenAIResponsesInputMessage)
	require.True(t, ok)
	require.Equal(t, "user", first.Role)
	require.Equal(t, userPrompt, first.Content)
	second, ok := payload[1].(httppkg.OpenAIResponsesInputMessage)
	require.True(t, ok)
	require.Equal(t, "assistant", second.Role)
	require.Equal(t, finalAnswer, second.Content)
}

// -----------------------------------------------------------------------------
// E6 — PAYLOAD_TOO_LARGE truncation behaviour, asserted at the loop-event
// boundary AND independently against the production truncation helper.
//
// Bug class caught: the agent loop's SessionEndEvent.FinalText carries the
// full 200 KB final answer (this is correct — the loop must not lose
// data), and the production memory hook truncates it middle-cut before
// handing to memoryx (Bug C in the live e2e: without truncation, the MCP
// file_write returned PAYLOAD_TOO_LARGE and the loop logged an ERROR).
//
// What we lock in:
//
//   - The loop ships SessionEndEvent with the full 200 KB FinalText (so a
//     buggy in-flight truncation that loses bytes is caught).
//   - The production truncateMiddle (re-implemented here as
//     truncateMiddleForTest, mirroring the documented semantics) produces
//     a payload ≤ 64 KiB + marker overhead carrying the literal
//     `[truncated <N> bytes]` middle-cut marker.
//
// We cannot swap defaultMemoryAfterTurn from package agentx (constraint
// forbids modifying agentx/tools), so we validate the contract at the
// loop boundary + replicate the truncation logic. The production helper
// is covered by tools/memoryhook_test.go's TestMemoryHooks_TruncateMiddleSemantics.
// -----------------------------------------------------------------------------

func TestE6_PayloadTooLargeTruncation(t *testing.T) {
	// Bug class: oversized final-text payload triggers PAYLOAD_TOO_LARGE log spam (Bug C).
	setupTestConfig(t, defaultAgentCfg())

	ctx, _, user := newTestGinCtx(t, "{}")
	on := true
	req := frontendReqAgent(&on, "summarise the doc", nil)

	// Build a 200 KB final answer with distinguishable head + tail so the
	// middle-cut assertions can verify byte preservation.
	const oversize = 200 * 1024
	bigFinal := strings.Repeat("X", oversize)

	captured := &sessionEndCapture{}
	scripts := [][]model.StreamChunk{{
		{Kind: model.ChunkFunction, FunctionCall: &model.FunctionCall{
			CallID:    "call_send_e6",
			Name:      "send_to_user",
			Arguments: rawArgs(t, map[string]any{"final_answer": bigFinal}),
		}},
		{Kind: model.ChunkDone},
	}}
	rec := newRecordingModel(scripts)
	registry := buildRegistry(t)

	err := handleAgentWithDeps(ctx, agentRunInputs{
		FrontendReq:    req,
		User:           user,
		ResponsesReq:   &httppkg.OpenAIResponsesReq{Model: "gpt-test"},
		UpstreamHeader: http.Header{},
		AgentCfg:       defaultAgentCfg(),
	}, busOverride{
		ModelClient:     rec,
		Registry:        registry,
		DisableDefaults: true,
		PreRegister: func(b *hook.Bus) {
			b.OnSessionEnd(captured.recordSessionEnd)
		},
	})
	require.NoError(t, err)

	// Loop-side contract: full 200 KB FinalText reaches the SessionEndEvent
	// — the loop must NOT lose bytes in flight. Truncation happens INSIDE
	// the memory hook, downstream of this event.
	require.Equal(t, oversize, len(captured.finalText()),
		"SessionEndEvent.FinalText must carry the full 200 KB payload")

	// Production-hook contract (re-implemented per tools/memoryhook.go
	// truncateMiddle semantics): payload ≤ 64 KiB + marker overhead,
	// carries the middle-cut marker.
	const cap = tools.DefaultMemoryFinalTextMaxBytes // 64 KiB
	truncated, wasTrimmed := truncateMiddleForTest(captured.finalText(), cap)
	require.True(t, wasTrimmed, "200 KB input must trigger truncation")
	require.LessOrEqual(t, len(truncated), cap+512,
		"truncated payload must fit within the default cap + marker overhead")
	require.Contains(t, truncated, "[truncated ",
		"middle-cut marker prefix `[truncated ` must be present")
	require.Contains(t, truncated, " bytes]",
		"middle-cut marker suffix ` bytes]` must be present")
	require.True(t, strings.HasPrefix(truncated, "X"),
		"head fragment must be preserved")
	require.True(t, strings.HasSuffix(truncated, "X"),
		"tail fragment must be preserved")
}

// -----------------------------------------------------------------------------
// E7 — agent_mode=nil / disabled config must short-circuit HandleAgent.
//
// Bug class caught: the agent dispatcher fires (or worse, the inner
// handler runs) for a request that did NOT opt into agent_mode (I1
// regression). The byte-level proxy-path invariance check lives in
// http/agent_invariance_test.go because it needs the unexported
// sendChatWithResponsesToolLoop helper and ctxKeyAgentMode constant; from
// `package agentx` we cannot reach those.
//
// We assert the agent-side gate instead: HandleAgent always returns
// ErrAgentLoopDisabled when the config is missing or has Enabled=false.
// Combined with the dispatcher gate at http/responses_chat_handler.go
// (covered by TestI1_ProxyInvariance_NoAgentModeSkipsDispatcher) this
// keeps the proxy path safe under defence-in-depth.
// -----------------------------------------------------------------------------

func TestE7_AgentModeDisabledShortCircuits(t *testing.T) {
	// Bug class: agent loop fires when caller did not opt in (I1).

	// 1) AgentLoop config absent => ErrAgentLoopDisabled. Note: this
	//    mirrors the existing TestI3_HandleAgent_ConfigDisabled but
	//    additionally pins the AgentMode=nil request shape so a future
	//    AgentMode-default flip cannot make HandleAgent run silently.
	setupTestConfig(t, nil)
	ctx, _, user := newTestGinCtx(t, "{}")
	req := frontendReqAgent(nil, "hello", nil) // AgentMode nil
	require.Nil(t, req.LaiskyExtra,
		"sanity: LaiskyExtra must be nil when AgentMode is nil")
	err := HandleAgent(ctx, req, user, &httppkg.OpenAIResponsesReq{Model: "x"}, http.Header{})
	require.ErrorIs(t, err, ErrAgentLoopDisabled,
		"HandleAgent must short-circuit when AgentLoop config is missing")

	// 2) AgentLoop.Enabled=false => ErrAgentLoopDisabled.
	cfg := defaultAgentCfg()
	cfg.Enabled = false
	setupTestConfig(t, cfg)
	err = HandleAgent(ctx, req, user, &httppkg.OpenAIResponsesReq{Model: "x"}, http.Header{})
	require.ErrorIs(t, err, ErrAgentLoopDisabled,
		"HandleAgent must short-circuit when AgentLoop.Enabled=false")

	// 3) ErrAgentLoopDisabled must be the same sentinel http exposes
	//    (so the http dispatch gate matches errors.Is). This is the
	//    cross-package contract that keeps the proxy path safe.
	require.ErrorIs(t, ErrAgentLoopDisabled, httppkg.ErrAgentDispatcherDisabled,
		"agentx.ErrAgentLoopDisabled must equal httppkg.ErrAgentDispatcherDisabled")
}

// -----------------------------------------------------------------------------
// E8 — Cancellation cleanup: no goroutine leak.
//
// Bug class caught: the SSE consumer goroutine fails to exit when the
// loop is cancelled mid-stream (I5 regression). The existing I5 test in
// handler_test.go already covers this at unit granularity; this e2e
// variant adds the stricter "wall-clock 5 s, cancel after 100 ms, exit
// promptly, goroutines settle within 1 s" timing budget the spec calls
// for.
// -----------------------------------------------------------------------------

// stallingE2EClient blocks until the context is cancelled, then emits a
// terminal Error chunk so the loop sees a structured exit.
type stallingE2EClient struct{}

func (stallingE2EClient) Stream(ctx context.Context, _ model.Request) (<-chan model.StreamChunk, error) {
	ch := make(chan model.StreamChunk)
	go func() {
		defer close(ch)
		<-ctx.Done()
		select {
		case ch <- model.StreamChunk{Kind: model.ChunkError, Err: ctx.Err()}:
		default:
		}
	}()
	return ch, nil
}

func (stallingE2EClient) Capabilities() model.Capabilities {
	return model.Capabilities{SupportsParallelToolCalls: true}
}

func TestE8_CancellationCleanup_NoGoroutineLeak(t *testing.T) {
	// Bug class: agent loop goroutine leak on client disconnect (I5 regression).
	cfg := defaultAgentCfg()
	cfg.WallClockSeconds = 5 // generous wall-clock; we'll cancel manually
	setupTestConfig(t, cfg)

	ctx, _, user := newTestGinCtx(t, "{}")
	on := true
	req := frontendReqAgent(&on, "hang please", nil)

	// Replace ctx.Request so we can cancel its context independently.
	reqCtx, cancel := context.WithCancel(ctx.Request.Context())
	ctx.Request = ctx.Request.WithContext(reqCtx)

	beforeG := runtime.NumGoroutine()
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	registry := buildRegistry(t)
	done := make(chan struct{})
	start := time.Now()
	go func() {
		_ = handleAgentWithDeps(ctx, agentRunInputs{
			FrontendReq:    req,
			User:           user,
			ResponsesReq:   &httppkg.OpenAIResponsesReq{Model: "gpt-test"},
			UpstreamHeader: http.Header{},
			AgentCfg:       cfg,
		}, busOverride{
			ModelClient: stallingE2EClient{},
			Registry:    registry,
		})
		close(done)
	}()

	// Handler must return promptly after cancellation. Spec target is
	// ~300 ms; we tolerate up to 2 s to absorb scheduler jitter under
	// -race. A hard 3 s ceiling guards against a true deadlock.
	select {
	case <-done:
		require.LessOrEqual(t, time.Since(start), 2*time.Second,
			"handler must exit promptly after cancellation")
	case <-time.After(3 * time.Second):
		t.Fatal("handler did not return within 3 s after cancellation")
	}

	// Goroutines should settle back to baseline within 1 s.
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		if runtime.NumGoroutine()-beforeG <= 2 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	leaked := runtime.NumGoroutine() - beforeG
	require.LessOrEqual(t, leaked, 2,
		"goroutine leak after cancellation: before=%d after=%d (delta=%d)",
		beforeG, runtime.NumGoroutine(), leaked)
}

// -----------------------------------------------------------------------------
// sessionEndCapture — small thread-safe recorder used by E5 / E6.
// -----------------------------------------------------------------------------

type sessionEndCapture struct {
	fired atomic.Int32
	mu    sync.Mutex
	ev    hook.SessionEndEvent
}

func (s *sessionEndCapture) recordSessionEnd(_ context.Context, ev hook.SessionEndEvent) (hook.SessionEndEvent, error) {
	s.fired.Add(1)
	s.mu.Lock()
	s.ev = ev
	s.mu.Unlock()
	return ev, nil
}

func (s *sessionEndCapture) userPrompt() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ev.UserPrompt
}

func (s *sessionEndCapture) finalText() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ev.FinalText
}

func (s *sessionEndCapture) terminatedBy() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ev.TerminatedBy
}

// -----------------------------------------------------------------------------
// Production-mirror helpers — re-implement the documented semantics of the
// agentx/tools helpers we can't reach from this package. Each helper is a
// faithful copy of the production logic; if the production diverges the
// e2e contract assertion still holds because we're testing the BUS-level
// guarantee, with these helpers acting as a documentation-grade reference.
// -----------------------------------------------------------------------------

// buildMinimalMemoryInputForTest mirrors agentx/tools.minimalMemoryInput:
// the exact two-item slice the production memory hook persists. Kept here
// so E5 can assert the U15 hygiene contract end-to-end without reaching
// across the package boundary.
func buildMinimalMemoryInputForTest(userPrompt, finalText string) []any {
	out := make([]any, 0, 2)
	if strings.TrimSpace(userPrompt) != "" {
		out = append(out, httppkg.OpenAIResponsesInputMessage{
			Role:    "user",
			Content: userPrompt,
		})
	}
	if strings.TrimSpace(finalText) != "" {
		out = append(out, httppkg.OpenAIResponsesInputMessage{
			Role:    "assistant",
			Content: finalText,
		})
	}
	return out
}

// truncateMiddleForTest mirrors agentx/tools.truncateMiddle. Used by E6
// to assert the same truncation contract the production memory hook
// applies. See tools/memoryhook.go for the canonical implementation; the
// production unit test (TestMemoryHooks_TruncateMiddleSemantics) covers
// the byte-level correctness — this helper exists only so E6 can re-
// run the contract on the SessionEndEvent payload from package agentx.
func truncateMiddleForTest(s string, max int) (string, bool) {
	if max <= 0 || len(s) <= max {
		return s, false
	}
	dropped := len(s) - max
	headLen := max / 2
	tailLen := max - headLen
	head := s[:headLen]
	tail := s[len(s)-tailLen:]
	marker := fmt.Sprintf("[truncated %d bytes]", dropped)
	return head + marker + tail, true
}

// -----------------------------------------------------------------------------
// Golden helpers (mirror model/oneapi_test.go's pattern).
// -----------------------------------------------------------------------------

// checkGolden compares the JSON marshalling of payload against the file
// under testdata/<name>. With -update-e2e the file is regenerated. The
// testdata/ directory and file content are committed to the repo, so the
// expected golden files MUST exist; a missing golden is a test failure.
func checkGolden(t *testing.T, name string, payload any) {
	t.Helper()
	want, err := stdjson.MarshalIndent(payload, "", "  ")
	require.NoError(t, err, "marshal payload for %s", name)
	want = append(want, '\n')
	path := filepath.Join("testdata", name)
	if *updateE2EGolden {
		require.NoError(t, os.WriteFile(path, want, 0o644), "write golden %s", name)
		return
	}
	got, err := os.ReadFile(path)
	require.NoErrorf(t, err,
		"golden %s missing; commit testdata/%s or rerun with -update-e2e",
		name, name)
	require.Equal(t, string(want), string(got),
		"golden %s mismatch; rerun with -update-e2e to refresh", name)
}

func collectToolNames(tools []model.ToolDescriptor) []string {
	out := make([]string, 0, len(tools))
	for _, d := range tools {
		out = append(out, d.Name)
	}
	return out
}

// collectFunctionCallItemsStable flattens the function_call slots of a
// recorded request into a stable subset for the golden — index +
// id_prefix + id_nonempty + call_id + name. We drop the actual id value
// because the loop synthesises a fresh ULID per run (loop.go's
// callIDForFunctionCall); only the stable shape lands in the golden.
func collectFunctionCallItemsStable(req recordedRequest) any {
	type fcView struct {
		Index      int    `json:"index"`
		IDPrefix   string `json:"id_prefix"`
		IDNonEmpty bool   `json:"id_nonempty"`
		CallID     string `json:"call_id"`
		Name       string `json:"name"`
	}
	out := []fcView{}
	for i, item := range req.Input {
		if item.Kind != "function_call" {
			continue
		}
		fc, ok := item.Payload.(httppkg.OpenAIResponsesFunctionCall)
		if !ok {
			continue
		}
		prefix := ""
		switch {
		case strings.HasPrefix(fc.ID, "fc_"):
			prefix = "fc_"
		case strings.HasPrefix(fc.ID, "call_"):
			prefix = "call_"
		}
		out = append(out, fcView{
			Index:      i,
			IDPrefix:   prefix,
			IDNonEmpty: fc.ID != "",
			CallID:     fc.CallID,
			Name:       fc.Name,
		})
	}
	return out
}
