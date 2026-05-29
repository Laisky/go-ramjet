// Package agentx hosts the top-level agent-mode entrypoint. The Phase 1
// ChatHandler delegates to HandleAgent when `laisky_extra.chat_switch.agent_mode`
// is set on the inbound request (proposal §4.1).
//
// Subpackages own the moving parts:
//   - agentx/session  — per-request submit/event split and transcript.
//   - agentx/tool     — Tool interface and Registry.
//   - agentx/model    — Provider-neutral LLM client interface.
//   - agentx/hook     — Named HookBus.
//   - agentx/loop     — ReAct loop driver.
//   - agentx/tools    — Concrete tool wrappers and the curated belt.
//   - agentx/sse      — Session.Event → SSE chunk adapter.
//   - agentx/prompt   — Versioned system prompt renderer.
package agentx

import (
	"context"
	stdjson "encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/distiller"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/hook"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/loop"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/model"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/prompt"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/session"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/sse"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/tool"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/tools"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
	httppkg "github.com/Laisky/go-ramjet/internal/tasks/gptchat/http"
)

// ErrAgentLoopDisabled is returned when the server config has the agent
// loop globally disabled (`openai.agent_loop` absent or
// `agent_loop.enabled=false`) but the inbound request set
// `agent_mode=true`. The HTTP layer surfaces this as a 409
// `agent_mode_disabled` response per proposal §5.4. The same sentinel
// is exported from the http package as ErrAgentDispatcherDisabled and
// errors.Is treats them as equivalent for the dispatch contract.
var ErrAgentLoopDisabled = httppkg.ErrAgentDispatcherDisabled

// init wires HandleAgent into the http package's dispatch table at
// program start. The wiring runs once per process; there is no
// teardown — production binaries link agentx in and never need to
// reset.
func init() {
	httppkg.RegisterAgentDispatcher(HandleAgent)
}

// HandleAgent is the agent-mode entrypoint called from the existing
// ChatHandler when the inbound request opts into the server-side ReAct
// loop. The function streams SSE bytes to the gin response writer using
// the existing chat-completion delta wire format (proposal §4.5) and
// blocks until the loop terminates.
//
// Returns:
//   - nil on clean termination — any TerminatedBy is treated as success
//     because the loop has already streamed the user-visible Final.
//   - ErrAgentLoopDisabled when `config.Config.AgentLoop` is missing or
//     `Enabled=false`. Caller maps this to HTTP 409.
//   - ctx.Err() on client cancellation. Partial SSE bytes will already
//     have been streamed.
//   - wrapped error for unrecoverable setup failures (bad config, MCP
//     discovery rejected with no fallback, etc).
//
// The function never mutates frontendReq — a shallow copy is used
// whenever the inner machinery needs `EnableMCP=true` so the caller's
// view stays exactly as it was passed in (§4.2 decision #3 / U13).
func HandleAgent(
	ctx *gin.Context,
	frontendReq *httppkg.FrontendReq,
	user *config.UserConfig,
	responsesReq *httppkg.OpenAIResponsesReq,
	upstreamHeader http.Header,
) error {
	cfg := agentConfigOrNil()
	if cfg == nil || !cfg.Enabled {
		return ErrAgentLoopDisabled
	}
	return handleAgentWithDeps(ctx, agentRunInputs{
		FrontendReq:    frontendReq,
		User:           user,
		ResponsesReq:   responsesReq,
		UpstreamHeader: upstreamHeader,
		AgentCfg:       cfg,
	}, busOverride{})
}

// agentRunInputs bundles the request-scoped inputs the agent dispatch
// path needs. Kept as a struct so the test-only handleAgentWithBus
// entrypoint can build it without going through HandleAgent's public
// signature.
type agentRunInputs struct {
	FrontendReq    *httppkg.FrontendReq
	User           *config.UserConfig
	ResponsesReq   *httppkg.OpenAIResponsesReq
	UpstreamHeader http.Header
	AgentCfg       *config.AgentLoopConfig
}

// busOverride lets internal callers (test harness or future composition
// hooks) inject hooks ahead of the standard chain without exposing the
// bus on the public API.
type busOverride struct {
	// PreRegister, when set, runs after the bus is constructed and the
	// standard Phase 1 hooks are installed. It typically registers
	// test-only redactors / observers.
	PreRegister func(*hook.Bus)
	// DisableDefaults skips registration of the standard hook chain;
	// only PreRegister hooks fire. Useful in unit tests that want a
	// blank bus. Production callers leave this false.
	DisableDefaults bool
	// ModelClient, when set, overrides the OneAPI client. Used by tests
	// that drive the loop against a scripted fake.
	ModelClient model.Client
	// Registry, when set, overrides the curated belt outright. Tests
	// pass a hand-built tool.Registry; production callers leave it nil
	// so BuildCuratedBelt runs.
	Registry tool.Registry
}

// agentConfigOrNil returns the active AgentLoopConfig or nil when the
// global config does not opt into agent mode.
func agentConfigOrNil() *config.AgentLoopConfig {
	if config.Config == nil {
		return nil
	}
	return config.Config.AgentLoop
}

// handleAgentWithDeps is the dependency-injected core of HandleAgent.
// Public callers go through HandleAgent which builds the standard deps;
// the internal test entrypoint (handler_internal_test.go) constructs
// the busOverride directly so tests can stage redactors before the loop
// runs.
//
// The function performs the seven steps spelled out in the proposal §4.1
// flowchart: extract prompt → build belt → install hooks → spawn SSE
// consumer → submit user-turn → run loop → drain consumer → close
// session.
func handleAgentWithDeps(
	gctx *gin.Context,
	inputs agentRunInputs,
	override busOverride,
) error {
	logger := gmw.GetLogger(gctx)
	if inputs.AgentCfg == nil {
		return ErrAgentLoopDisabled
	}

	// 1. SSE headers must be set before any chunk hits the wire. The
	//    proxy path sets these only after the upstream call lands but
	//    the agent path streams trace lines *before* any upstream byte
	//    arrives, so we set them up-front.
	setAgentStreamHeaders(gctx, inputs.UpstreamHeader)

	// 2. Extract the latest user message from frontendReq.Messages.
	userPrompt := lastUserPrompt(inputs.FrontendReq)
	if userPrompt == "" {
		return errors.New("agent_mode: empty user prompt")
	}

	// 3. Bridge the cross-hook memory state and assemble MemoryDeps.
	memState := tools.NewMemoryState()
	memoryEnabled := config.Config != nil &&
		config.Config.EnableMemory &&
		inputs.User != nil &&
		!inputs.User.IsFree
	memDeps := &tools.MemoryDeps{
		Config:         config.Config,
		User:           inputs.User,
		RequestHeader:  gctx.Request.Header,
		MaxInputTokens: 120000,
		Logger:         logger,
		Enabled:        memoryEnabled,
		State:          memState,
		// Defensive cap on the assistant final text handed to memoryx
		// so the MCP file_write JSONL append stays well below its
		// PAYLOAD_TOO_LARGE threshold. Zero falls back to the package
		// default (64 KiB); we set it explicitly for clarity at the
		// call site.
		FinalTextMaxBytes: tools.DefaultMemoryFinalTextMaxBytes,
	}

	// 4. Curated tool belt. We expose the LegacyDepsProvider through a
	//    closure that always reports `EnableMCP=true` on the FrontendReq
	//    copy it hands the dispatch path — never mutate the caller's
	//    request (decision §4.2 #3 / U13). The closure also injects the
	//    curated MCP server into FrontendReq.MCPServers so the legacy
	//    dispatcher's `findMCPServerForToolName` can resolve curated tool
	//    names even when the user-supplied request carries no MCP servers
	//    (the "MCP" UI toggle is independent from "Agent" and may be off).
	//
	//    curatedServer is built up-front with the URL/Enabled fields and a
	//    pointer is captured by the closure; we populate Tools after
	//    BuildCuratedBelt returns (the registry now knows which curated
	//    names the upstream MCP server actually advertised).
	curatedServer := resolveCuratedMCP(inputs.AgentCfg)
	depsProvider := tools.LegacyDepsFunc(func(ctx context.Context, _ string, _ string) (httppkg.LegacyDeps, error) {
		return httppkg.LegacyDeps{
			User:         inputs.User,
			FrontendReq:  forceMCPEnabledWithCuratedServer(inputs.FrontendReq, curatedServer),
			RawUserToken: httppkg.GetRawUserToken(gctx),
			Logger:       logger,
		}, nil
	})

	var registry tool.Registry
	if override.Registry != nil {
		registry = override.Registry
	} else {
		built, regErr := tools.BuildCuratedBelt(gmw.Ctx(gctx), tools.BeltDeps{
			Logger:           logger,
			MCPServer:        curatedServer,
			DepsProvider:     depsProvider,
			SubagentEnabled:  inputs.AgentCfg.Subagent.Enabled,
			SubagentMaxDepth: inputs.AgentCfg.Subagent.MaxDepth,
			FallbackBelt:     []string{"web_search", "web_fetch", "file_read"},
		})
		if regErr != nil {
			return errors.Wrap(regErr, "build curated belt")
		}
		registry = built
	}
	// Populate curatedServer.Tools so findMCPServerForToolName can resolve
	// the curated belt against this server. Done after BuildCuratedBelt so
	// the list reflects only the tools the MCP catalog actually advertised.
	populateCuratedServerTools(curatedServer, registry)

	// 5. Model client (OneAPI wrapper) — unless a test override forces a
	//    fake. The model's StreamSink is intentionally nil because the
	//    OneAPI adapter consumes typed events itself; the SSE writer
	//    below is the only path that touches the gin response.
	//
	//    The client is wrapped in a coercingModelClient (see coerce.go)
	//    so any map-shaped InputItem (emitted by the loop's userMessage /
	//    appendFunctionCallAndOutput helpers, or arriving via the memory
	//    enrichment hook's PreparedInput at responses_chat_handler.go:789)
	//    is converted to the three concrete structs the OneAPI adapter's
	//    validateInputItem accepts. The wrap is applied to overridden
	//    clients too so tests exercising mixed-shape inputs see the same
	//    boundary contract as production.
	//
	//    Built BEFORE the hook bus so the distill hook can capture it as
	//    the summariser backend.
	var modelClient model.Client
	if override.ModelClient != nil {
		modelClient = override.ModelClient
	} else {
		modelClient = model.NewOneAPIClient(model.OneAPIDeps{
			UpstreamDeps: httppkg.UpstreamDeps{
				User:          inputs.User,
				Logger:        logger,
				RequestHeader: gctx.Request.Header,
				RawQuery:      requestRawQuery(gctx),
			},
			Logger: logger,
		})
	}
	modelClient = newCoercingModelClient(modelClient)

	// 5b. Observation distiller. Falls back to the request's model when
	//     openai.agent_loop.distiller_model is unset. The raw-stash is
	//     per-request, captured by the distill hook closure for both
	//     writes (every oversize observation) and future JIT reads.
	distillerModelID := strings.TrimSpace(inputs.AgentCfg.DistillerModel)
	if distillerModelID == "" {
		distillerModelID = inputs.ResponsesReq.Model
	}
	rawStash := session.NewRawStash()
	llmDistiller := distiller.NewLLMDistiller(modelClient, distillerModelID, distiller.NewCache())
	if secs := inputs.AgentCfg.DistillTimeoutSeconds; secs > 0 {
		llmDistiller.Timeout = time.Duration(secs) * time.Second
	}
	distillThreshold := inputs.AgentCfg.DistillThresholdTokens
	if distillThreshold <= 0 {
		distillThreshold = distiller.DefaultThresholdTokens
	}

	// 6. Hook bus. Registration order is the firing order (verified by
	//    hook U21); ordering here is load-bearing.
	caps := capsFromConfig(inputs.AgentCfg)
	bus := hook.NewBus(logger)
	if !override.DisableDefaults {
		// Prompt comes BEFORE memory so the memory hook sees the
		// system directive in its input — the memory engine often
		// uses surrounding context as feature material; if it ran
		// first the ReAct directive would never reach it.
		bus.OnContext(prompt.NewReactRenderer(caps.MaxIterations).AsContextHook())
		bus.OnContext(tools.NewMemoryBeforeTurnHook(memDeps))
		bus.OnBeforeToolCall(loop.NewCircuitHook(caps.CircuitBreakerRepeats))
		bus.OnBeforeToolCall(loop.NewWriteGateHook(inputs.AgentCfg.WriteGate))
		// Distill BEFORE Wrap: the trust-delimiter encloses the
		// summarised observation, not the raw bytes. See
		// loop/distill.go godoc for the rationale.
		bus.OnAfterToolCall(loop.NewDistillHook(llmDistiller, distillThreshold, rawStash, userPrompt))
		bus.OnAfterToolCall(loop.NewWrapHook())
		bus.OnSessionEnd(tools.NewMemoryAfterTurnHook(memDeps))
	}
	if override.PreRegister != nil {
		override.PreRegister(bus)
	}

	// 7. Session, SSE writer, and the consumer goroutine.
	sess := session.NewSession(session.Config{Logger: logger, BufferSize: 256})
	// Capture the events channel BEFORE spawning the consumer. The
	// session's Close() nils primary; the goroutine must dereference
	// once and hold the channel across the lifecycle.
	events := sess.Events()

	requestID := upstreamRequestID(inputs.UpstreamHeader)
	emit := buildEmitFunc(gctx)
	writer := sse.NewWriter(emit, requestID)

	loopCtx, loopCancel := context.WithCancel(gmw.Ctx(gctx))
	defer loopCancel()

	consumerDone := make(chan error, 1)
	go func() {
		consumerDone <- writer.Consume(loopCtx, events)
	}()

	// Track the user turn so a later OpInterrupt could cancel it.
	if err := sess.Submit(loopCtx, session.OpUserTurn{Text: userPrompt}); err != nil {
		loopCancel()
		<-consumerDone
		return errors.Wrap(err, "submit user turn")
	}

	// 8. RunDeps. The Input slice carries the pre-existing conversation
	//    history (from `convert2UpstreamResponsesRequest` it is already
	//    in Responses-API shape). The loop appends the user prompt on
	//    top before the first model call.
	//
	//    The seed is coerced once at the handler boundary so that even
	//    map-shaped items injected by the memory enrichment hook (see
	//    responses_chat_handler.go:789 — `responsesReq.Input =
	//    beforeResult.PreparedInput`, a []any of map[string]any) land in
	//    the loop as the three concrete struct shapes. The coercingModel
	//    wrapper above re-runs the same coercion just before each Stream
	//    call to catch any maps the loop appends afterwards.
	inputItems, coerceErr := coerceInputItems(inputAsAnySlice(inputs.ResponsesReq.Input))
	if coerceErr != nil {
		return errors.Wrap(coerceErr, "coerce responses input for agent loop")
	}
	runErr := loop.Run(loopCtx, sess, loop.RunDeps{
		Bus:             bus,
		Registry:        registry,
		Model:           modelClient,
		Caps:            caps,
		UserPrompt:      userPrompt,
		SessionID:       requestID,
		Input:           inputItems,
		ModelID:         inputs.ResponsesReq.Model,
		MaxOutputTokens: inputs.ResponsesReq.MaxOutputTokens,
		Reasoning:       translateReasoning(inputs.ResponsesReq.Reasoning),
		Temperature:     inputs.ResponsesReq.Temperature,
		TopP:            inputs.ResponsesReq.TopP,
		Logger:          logger,
	})

	// 9. Drain the SSE consumer. Order matters: close the session FIRST
	//    so the subscriber channel is closed and the consumer's
	//    select-on-events sees ok=false instead of racing the loopCtx
	//    cancel. We do NOT cancel loopCtx here yet — if we did, the
	//    consumer's select might pick the cancel branch and drop any
	//    pending events the fanout had not yet broadcast.
	_ = sess.Close()
	consumerErr := <-consumerDone
	loopCancel() // only safe AFTER the consumer has drained
	if consumerErr != nil &&
		!errors.Is(consumerErr, context.Canceled) &&
		!errors.Is(consumerErr, context.DeadlineExceeded) {
		logger.Warn("agent_sse_consumer_error", zap.Error(consumerErr))
	}

	// 10. Surface the loop's error verbatim. Loop terminations (any
	//     TerminatedBy enum) return nil; only setup/transport failures
	//     bubble up here.
	if runErr != nil {
		return runErr
	}
	return nil
}

// setAgentStreamHeaders writes the SSE response headers and request-id
// echo before the first chunk. Mirrors the proxy path's setStreamHeaders
// behaviour so the frontend sees an identical wire-level handshake.
func setAgentStreamHeaders(ctx *gin.Context, upstream http.Header) {
	ctx.Header("content-type", "text/event-stream")
	ctx.Header("cache-control", "no-cache")
	ctx.Header("connection", "keep-alive")
	ctx.Header("Access-Control-Expose-Headers", "x-oneapi-request-id, x-request-id")
	if upstream != nil {
		if rid := upstream.Get("x-oneapi-request-id"); rid != "" {
			ctx.Header("x-oneapi-request-id", rid)
		}
		if rid := upstream.Get("x-request-id"); rid != "" &&
			ctx.Writer.Header().Get("x-oneapi-request-id") == "" {
			ctx.Header("x-oneapi-request-id", rid)
		}
	}
}

// lastUserPrompt mirrors the image-model code path at
// responses_chat_handler.go:67-74: walks frontendReq.Messages bottom-up
// and returns the first user message's textual content. Empty when the
// request has no user messages.
func lastUserPrompt(req *httppkg.FrontendReq) string {
	if req == nil {
		return ""
	}
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == httppkg.OpenaiMessageRoleUser {
			return strings.TrimSpace(req.Messages[i].Content.String())
		}
	}
	return ""
}

// forceMCPEnabled returns a SHALLOW copy of req with EnableMCP forced
// true. The caller's FrontendReq is never mutated (proposal §4.2 #3 /
// U13). Pointer fields are copied verbatim — they describe upstream
// catalog state that the dispatch path treats as read-only, so a
// shallow copy is enough.
func forceMCPEnabled(req *httppkg.FrontendReq) *httppkg.FrontendReq {
	if req == nil {
		return nil
	}
	cp := *req
	on := true
	cp.EnableMCP = &on
	return &cp
}

// forceMCPEnabledWithCuratedServer extends forceMCPEnabled by also
// ensuring the returned copy's MCPServers slice includes the curated MCP
// server descriptor. This is what makes the legacy dispatcher's
// findMCPServerForToolName lookup succeed for curated belt tools —
// without it the lookup walks the caller's (possibly empty) MCPServers
// slice and rejects the call with "tool X not found in enabled MCP
// servers" (Bug 1).
//
// Dedupe rule: if the caller already supplied a server with the same
// URL as curatedServer (e.g. the user enabled MCP from the UI and
// pointed at the same backend), we leave the caller's entry in place
// and do NOT add a second copy. We still build a fresh MCPServers slice
// (never mutating the caller's slice) so the U13 isolation contract
// holds: the caller's FrontendReq is untouched on return.
//
// When curatedServer is nil the function degrades to forceMCPEnabled.
func forceMCPEnabledWithCuratedServer(
	req *httppkg.FrontendReq,
	curatedServer *httppkg.MCPServerConfig,
) *httppkg.FrontendReq {
	cp := forceMCPEnabled(req)
	if cp == nil || curatedServer == nil {
		return cp
	}
	// Build a fresh slice so we never mutate the caller's request — U13.
	merged := make([]httppkg.MCPServerConfig, 0, len(cp.MCPServers)+1)
	curatedURL := strings.TrimSpace(curatedServer.URL)
	seenCurated := false
	for _, s := range cp.MCPServers {
		merged = append(merged, s)
		if curatedURL != "" && strings.TrimSpace(s.URL) == curatedURL {
			seenCurated = true
		}
	}
	if !seenCurated {
		merged = append(merged, *curatedServer)
	}
	cp.MCPServers = merged
	return cp
}

// populateCuratedServerTools sets curatedServer.Tools to a
// []json.RawMessage carrying one `{"name": "..."}` entry per curated
// MCP tool registered in reg. findMCPServerForToolName resolves a tool
// by walking each server's Tools slice and reading the `name` field, so
// this is the minimum payload we must surface for the lookup to find
// the curated server when the legacy dispatcher routes a curated call.
//
// We filter the registry by Source so only tools that actually live
// behind the curated MCP server are listed — local tools
// (send_to_user, spawn_agent) must not be claimed by the server.
func populateCuratedServerTools(curatedServer *httppkg.MCPServerConfig, reg tool.Registry) {
	if curatedServer == nil || reg == nil {
		return
	}
	descriptors := reg.Descriptors()
	if len(descriptors) == 0 {
		return
	}
	tools := make([]stdjson.RawMessage, 0, len(descriptors))
	for _, d := range descriptors {
		if d.Source != tool.SourceCuratedMCP {
			continue
		}
		raw, err := stdjson.Marshal(map[string]string{"name": d.Name})
		if err != nil {
			// Marshalling a static map cannot realistically fail; fall
			// back to a hand-rolled JSON literal so the lookup still
			// finds the name even on the unreachable error path.
			raw = stdjson.RawMessage(`{"name":` + quoteJSONString(d.Name) + `}`)
		}
		tools = append(tools, raw)
	}
	curatedServer.Tools = tools
}

// quoteJSONString is a tiny helper that hands back a JSON-escaped form
// of s suitable for splicing into a manual JSON literal. Used only in
// the unreachable error path of populateCuratedServerTools.
func quoteJSONString(s string) string {
	b, err := stdjson.Marshal(s)
	if err != nil {
		return `""`
	}
	return string(b)
}

// resolveCuratedMCP returns the curated MCP server descriptor or nil
// when the config has nothing usable. The configured value (
// `openai.agent_loop.mcp_server`) is a string that may be either:
//
//   - A direct URL ("https://mcp.example.com"). Phase 1 deployments use
//     this form against the laisky MCP server.
//   - An alias to be looked up in a future `openai.mcp_servers` block.
//     The block does not exist yet; if it appears later this helper is
//     where the lookup happens.
//
// When the configured value is empty we fall back to
// `Config.MemoryStorageMCPURL`, which today happens to be the laisky
// MCP host. The fallback exists so the operator does not have to
// duplicate the same URL in two places.
func resolveCuratedMCP(cfg *config.AgentLoopConfig) *httppkg.MCPServerConfig {
	if cfg == nil {
		return nil
	}
	raw := strings.TrimSpace(cfg.MCPServer)
	if raw == "" {
		if config.Config != nil {
			raw = strings.TrimSpace(config.Config.MemoryStorageMCPURL)
		}
	}
	if raw == "" {
		return nil
	}
	// Phase 1 only supports the direct-URL form; alias lookup is a
	// Phase ≥ 2 feature. We accept anything that parses as a URL and
	// forward it; the discovery helper validates the shape.
	if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
		// Treat non-URL values as aliases. There is no alias registry
		// yet; return nil so the belt builder falls back to its
		// FallbackBelt and the loop still runs.
		return nil
	}
	return &httppkg.MCPServerConfig{
		URL:     raw,
		Enabled: true,
	}
}

// capsFromConfig snapshots the runtime Caps from the YAML knobs. Zero
// values land on loop.DefaultCaps via loop.Caps.withDefaults() — but
// we feed the config values explicitly so unit tests can override.
func capsFromConfig(cfg *config.AgentLoopConfig) loop.Caps {
	return loop.Caps{
		MaxIterations:         cfg.MaxIterations,
		MaxToolCalls:          cfg.MaxToolCalls,
		MaxParallelToolCalls:  cfg.MaxParallelToolCalls,
		ErrorBudget:           cfg.ErrorBudget,
		CircuitBreakerRepeats: cfg.CircuitBreakerRepeats,
		WallClock:             time.Duration(cfg.WallClockSeconds) * time.Second,
	}
}

// translateReasoning bridges the wire-shape *OpenAIResponseReasoning
// into the loop-side *model.Reasoning. Returns nil when no reasoning
// fields are populated so the loop's request omits the field upstream.
func translateReasoning(in *httppkg.OpenAIResponseReasoning) *model.Reasoning {
	if in == nil {
		return nil
	}
	r := model.Reasoning{}
	if in.Effort != nil {
		r.Effort = *in.Effort
	}
	if in.Summary != nil {
		r.Summary = *in.Summary
	}
	if r.Effort == "" && r.Summary == "" {
		return nil
	}
	return &r
}

// requestRawQuery is a small null-safe helper.
func requestRawQuery(ctx *gin.Context) string {
	if ctx == nil || ctx.Request == nil || ctx.Request.URL == nil {
		return ""
	}
	return ctx.Request.URL.RawQuery
}

// upstreamRequestID extracts the OneAPI / generic request id, falling
// back to an empty string when neither header is present (the loop's
// model client will overwrite the id once it receives the first
// upstream response).
func upstreamRequestID(h http.Header) string {
	if h == nil {
		return ""
	}
	if rid := h.Get("x-oneapi-request-id"); rid != "" {
		return rid
	}
	return h.Get("x-request-id")
}

// buildEmitFunc wraps the gin-side stream sink + chat-completion chunk
// builder into the sse.EmitFunc interface the agent loop's SSE writer
// expects. Lives here (not in agentx/sse) so the sse package stays
// gin-agnostic — the only piece of gin-coupled code is this closure.
func buildEmitFunc(ctx *gin.Context) sse.EmitFunc {
	ginSink := httppkg.GinStreamSink(ctx)
	return func(kind sse.EmitKind, requestID, text string) error {
		chunk := httppkg.OpenaiCompletionStreamResp{
			ID: requestID,
			Choices: []httppkg.OpenaiCompletionStreamRespChoice{{
				Index: 0,
			}},
		}
		switch kind {
		case sse.EmitReasoning:
			chunk.Choices[0].Delta = httppkg.OpenaiCompletionStreamRespDelta{
				Role:             httppkg.OpenaiMessageRoleAI,
				ReasoningContent: text,
			}
		case sse.EmitContent:
			chunk.Choices[0].Delta = httppkg.OpenaiCompletionStreamRespDelta{
				Role:    httppkg.OpenaiMessageRoleAI,
				Content: text,
			}
		case sse.EmitFinish:
			chunk.Choices[0].Delta = httppkg.OpenaiCompletionStreamRespDelta{
				Role: httppkg.OpenaiMessageRoleAI,
			}
			chunk.Choices[0].FinishReason = "stop"
		}
		return httppkg.WriteChatCompletionChunkToSink(ginSink, chunk)
	}
}
