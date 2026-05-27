package tools

import (
	"context"
	"encoding/json"
	"sort"

	"github.com/Laisky/errors/v2"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/session"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/tool"
	httppkg "github.com/Laisky/go-ramjet/internal/tasks/gptchat/http"
)

// CuratedBeltNames is the Phase 1 curated MCP tool list (proposal §4.3).
// BuildCuratedBelt filters DiscoverMCPTools' output to exactly this set;
// extra tools surfaced by the MCP server are silently dropped, missing
// tools generate a warning but the registry is still returned (the model
// will degrade gracefully — typically by calling send_to_user).
var CuratedBeltNames = []string{
	"web_search",
	"web_fetch",
	"file_list",
	"file_stat",
	"file_read",
	"file_search",
	"file_write",
	"file_delete",
	"file_rename",
	"memory_before_turn",
	"memory_after_turn",
}

// BeltDeps captures the inputs BuildCuratedBelt needs.
//
// Logger is required so the function can warn on missing curated tools
// and MCP discovery failures. MCPServer / DepsProvider are required when
// FallbackBelt is empty: discovery failures otherwise leave the registry
// without any executable curated tool. SubagentEnabled gates registration
// of the Phase 1 stub spawn_agent (proposal §3.6 and U20).
//
// FallbackBelt lists the tool names registered as stub error-tools when
// DiscoverMCPTools returns an error; per proposal §9 ("MCP discovery
// flaky at startup → fall back to a hard-coded minimal belt"). When
// FallbackBelt is empty and discovery fails, the function returns the
// underlying error wrapped.
type BeltDeps struct {
	// Logger is the structured logger used for collisions, warnings, and
	// the discovery-failure trace. Required.
	Logger glog.Logger
	// MCPServer is the curated MCP server descriptor (e.g. laisky). Pass
	// nil to skip MCP discovery — useful in tests that exercise only the
	// local + subagent registration.
	MCPServer *httppkg.MCPServerConfig
	// MCPOpts are forwarded to DiscoverMCPTools verbatim. nil is fine.
	MCPOpts *httppkg.MCPCallOption
	// DepsProvider is the per-tool LegacyDeps factory used by every curated
	// MCP tool's Execute. Required when MCPServer is non-nil.
	DepsProvider LegacyDepsProvider
	// SubagentEnabled, when true, registers the Phase 1 stub spawn_agent
	// tool with SourceLocal. Default config leaves it false (U20).
	SubagentEnabled bool
	// SubagentMaxDepth is forwarded to NewSubAgentTool; 0 means "use the
	// proposal default" (2).
	SubagentMaxDepth int
	// FallbackBelt, when non-empty, supplies tool names registered as
	// IsError-returning stubs whenever DiscoverMCPTools fails. Useful in
	// production to keep the loop alive on a flaky MCP catalog.
	FallbackBelt []string
}

// mcpDiscoverer is the function signature shared by the production
// http.DiscoverMCPTools and the test fakes. Unexported seam.
type mcpDiscoverer func(ctx context.Context, server *httppkg.MCPServerConfig, opts *httppkg.MCPCallOption) ([]httppkg.MCPToolDescriptor, error)

// defaultMCPDiscoverer is the package-level discovery seam. Tests swap
// this to inject stub tool catalogs without touching the public API.
var defaultMCPDiscoverer mcpDiscoverer = httppkg.DiscoverMCPTools

// BuildCuratedBelt assembles a tool.Registry containing exactly the Phase 1
// curated belt: send_to_user (always), spawn_agent (iff
// BeltDeps.SubagentEnabled), and every curated MCP tool returned by
// DiscoverMCPTools whose name appears in CuratedBeltNames.
//
// On MCP discovery failure the function logs a warning and registers
// BeltDeps.FallbackBelt as IsError-returning stub tools; if FallbackBelt
// is empty and MCPServer was non-nil, the underlying discovery error is
// returned to the caller wrapped.
//
// The returned registry is NOT subset-restricted; the handler typically
// passes the result directly to the loop or calls Subset for filtering.
//
// BuildCuratedBelt is the only function in this package that consumes
// http.DiscoverMCPTools; the rest of the codebase reaches MCP only
// through the curated registry. See proposal §3.2, §4.3, §9.
func BuildCuratedBelt(ctx context.Context, deps BeltDeps) (tool.Registry, error) {
	reg := tool.NewRegistry(deps.Logger)

	// 1. Always-on synthesized tools.
	if err := reg.Register(NewSendToUserTool(), tool.SourceLocal); err != nil {
		return nil, errors.Wrap(err, "register send_to_user")
	}

	// 2. spawn_agent reservation (proposal §3.6, U20).
	if deps.SubagentEnabled {
		if err := reg.Register(NewSubAgentTool(deps.SubagentMaxDepth), tool.SourceLocal); err != nil {
			return nil, errors.Wrap(err, "register spawn_agent")
		}
	}

	// 3. Curated MCP belt. Skip cleanly when MCPServer is nil.
	if deps.MCPServer == nil {
		return reg, nil
	}

	tools, err := defaultMCPDiscoverer(ctx, deps.MCPServer, deps.MCPOpts)
	if err != nil {
		if deps.Logger != nil {
			deps.Logger.Warn("agent_mcp_discovery_failed",
				zap.String("server_url", deps.MCPServer.URL),
				zap.Error(err),
			)
		}
		// Fallback path: register the minimal belt as stub error tools so
		// the loop can still report a structured failure to the model.
		// Per proposal §9.
		if len(deps.FallbackBelt) > 0 {
			registerFallbackBelt(reg, deps.FallbackBelt, err, deps.Logger)
			return reg, nil
		}
		return nil, errors.Wrap(err, "discover mcp tools")
	}

	// Filter to the known-good curated belt; warn on extras and missing
	// entries (separately, so operators can spot drift in the MCP catalog).
	if err := registerCuratedTools(reg, tools, deps.DepsProvider, deps.Logger); err != nil {
		return nil, errors.Wrap(err, "register curated tools")
	}

	return reg, nil
}

// registerCuratedTools intersects the discovered MCP tools with the
// hardcoded CuratedBeltNames list, registers every match, and emits a
// single warning line for any curated name the MCP catalog did not provide.
// Extras (discovered tools outside CuratedBeltNames) are silently dropped —
// the curated belt is the contract; "extra" tools mean the upstream catalog
// drifted and operators get a separate metric for that elsewhere.
func registerCuratedTools(
	reg tool.Registry,
	tools []httppkg.MCPToolDescriptor,
	deps LegacyDepsProvider,
	logger glog.Logger,
) error {
	curatedSet := make(map[string]struct{}, len(CuratedBeltNames))
	for _, n := range CuratedBeltNames {
		curatedSet[n] = struct{}{}
	}

	provided := make(map[string]struct{}, len(tools))
	// Discovered tools may arrive in any order; sort by name so
	// registration order is deterministic across runs (the registry
	// itself also sorts on Names()/Descriptors(), but registering in a
	// stable order keeps the collision-warning log stable for tests).
	sortedTools := make([]httppkg.MCPToolDescriptor, len(tools))
	copy(sortedTools, tools)
	sort.SliceStable(sortedTools, func(i, j int) bool {
		return sortedTools[i].Name < sortedTools[j].Name
	})

	for _, td := range sortedTools {
		if _, ok := curatedSet[td.Name]; !ok {
			// Extra; not part of the curated belt.
			continue
		}
		provided[td.Name] = struct{}{}
		schema := td.InputSchema
		if len(schema) == 0 {
			schema = json.RawMessage(`{"type":"object"}`)
		}
		if err := reg.Register(
			NewLegacyDispatchTool(td.Name, td.Description, schema, deps),
			tool.SourceCuratedMCP,
		); err != nil {
			return errors.Wrapf(err, "register curated tool %q", td.Name)
		}
	}

	// Warn once per missing curated tool. We do not fail-loud here — the
	// loop must continue even if one of the curated tools is temporarily
	// unavailable; the model will pick a different strategy.
	if logger != nil {
		var missing []string
		for _, n := range CuratedBeltNames {
			if _, ok := provided[n]; !ok {
				missing = append(missing, n)
			}
		}
		if len(missing) > 0 {
			logger.Warn("agent_curated_belt_missing_tools",
				zap.Strings("missing", missing),
			)
		}
	}

	return nil
}

// registerFallbackBelt registers every name in fallback as an
// IsError-returning stub tool. Used when MCP discovery failed and the
// caller wants the loop to keep running anyway (proposal §9 risk
// mitigation). Each stub carries the underlying discovery error in its
// content so the model sees a structured signal rather than a silent
// missing-tool error.
func registerFallbackBelt(
	reg tool.Registry,
	fallback []string,
	cause error,
	logger glog.Logger,
) {
	msg := "tool unavailable: mcp discovery failed"
	if cause != nil {
		msg = "tool unavailable: " + cause.Error()
	}
	for _, name := range fallback {
		stub := &stubErrorTool{
			name:    name,
			content: msg,
		}
		if regErr := reg.Register(stub, tool.SourceCuratedMCP); regErr != nil && logger != nil {
			logger.Warn("agent_fallback_belt_register_failed",
				zap.String("name", name),
				zap.Error(regErr),
			)
		}
	}
}

// stubErrorTool is the placeholder tool installed by registerFallbackBelt.
// Every Execute returns IsError=true with the captured failure message;
// the schema is intentionally minimal (open object) since the model has
// no real schema to validate against.
type stubErrorTool struct {
	name    string
	content string
}

func (s *stubErrorTool) Name() string { return s.name }
func (s *stubErrorTool) Description() string {
	return "Temporarily unavailable: curated MCP discovery failed."
}
func (s *stubErrorTool) Schema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (s *stubErrorTool) Execute(_ context.Context, _ tool.Call, _ session.EventSink) (tool.Result, error) {
	return tool.Result{Content: s.content, IsError: true}, nil
}
