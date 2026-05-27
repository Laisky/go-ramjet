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

// CuratedBeltExcludes lists MCP tool names that the curated belt must
// drop even when the upstream MCP catalog advertises them. The list is
// the inverse of the original Phase 1 whitelist (proposal §4.3 was
// re-evaluated when the live catalog exposed 17 useful tools and the
// hardcoded include-list silently dropped six of them — `find_tool`,
// `get_user_request`, `mcp_pipe`, `extract_key_info`,
// `memory_list_dir_with_abstract`, `memory_run_maintenance`).
//
// The belt now operates on a fail-OPEN model: every discovered tool is
// registered with SourceCuratedMCP unless it is explicitly listed here.
// Determinism is preserved because the registry orders by Source then
// lex on Name (see proposal §3.2). When a future risk surfaces a tool
// we want suppressed (e.g. an MCP server exposing a shell-exec tool),
// add its name to this slice and the belt will fail closed for that
// single entry without losing the rest of the catalog.
//
// Currently empty — no tools in the laisky MCP catalog need to be
// excluded. Audited against the live catalog 2026-05-26.
var CuratedBeltExcludes = []string{}

// CuratedBeltNames is retained for backward-compatibility with callers
// (notably handler.go's FallbackBelt seeding logic) that still want a
// minimal hard-coded set of well-known tool names. The belt itself
// no longer filters by this list — see CuratedBeltExcludes for the
// active policy.
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

// BuildCuratedBelt assembles a tool.Registry containing send_to_user
// (always), spawn_agent (iff BeltDeps.SubagentEnabled), and every MCP
// tool returned by DiscoverMCPTools — minus any name listed in
// CuratedBeltExcludes.
//
// The belt operates fail-OPEN: when the live MCP catalog drifts (new
// tools added upstream) those tools are immediately available to the
// model. Determinism still holds because the registry orders entries
// by Source then lex on Name; the registration loop sorts the
// discovered slice by name before inserting so the registry receives
// inputs in a stable order even when DiscoverMCPTools returns shuffled
// results.
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

	// Register every discovered tool that is not in CuratedBeltExcludes.
	// Operators get a warning per excluded entry that DOES appear in the
	// catalog so drift in the exclude policy is visible.
	if err := registerCuratedTools(reg, tools, deps.DepsProvider, deps.Logger); err != nil {
		return nil, errors.Wrap(err, "register curated tools")
	}

	return reg, nil
}

// registerCuratedTools registers every discovered MCP tool with the
// registry under SourceCuratedMCP, except those names listed in
// CuratedBeltExcludes (the fail-OPEN policy; see CuratedBeltExcludes
// doc-comment for the rationale).
//
// Tools are sorted by name before registration so the registry receives
// a deterministic order across runs even when the upstream MCP catalog
// shuffles results (the registry itself also sorts on Names()/
// Descriptors(), but sorting on the way in keeps collision-warning logs
// stable for tests).
//
// Excluded entries that DO appear in the discovered catalog produce a
// single warning line so an operator can spot a stale exclude list.
func registerCuratedTools(
	reg tool.Registry,
	tools []httppkg.MCPToolDescriptor,
	deps LegacyDepsProvider,
	logger glog.Logger,
) error {
	excludeSet := make(map[string]struct{}, len(CuratedBeltExcludes))
	for _, n := range CuratedBeltExcludes {
		excludeSet[n] = struct{}{}
	}

	// Discovered tools may arrive in any order; sort by name so
	// registration order is deterministic across runs.
	sortedTools := make([]httppkg.MCPToolDescriptor, len(tools))
	copy(sortedTools, tools)
	sort.SliceStable(sortedTools, func(i, j int) bool {
		return sortedTools[i].Name < sortedTools[j].Name
	})

	var excluded []string
	for _, td := range sortedTools {
		if _, ok := excludeSet[td.Name]; ok {
			excluded = append(excluded, td.Name)
			continue
		}
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

	// Warn once when an excluded tool was actually advertised by the MCP
	// catalog. This is how operators learn the exclude list is doing
	// something (or that it can be pruned once the upstream catalog
	// removes the entry).
	if logger != nil && len(excluded) > 0 {
		logger.Warn("agent_curated_belt_excluded_tools",
			zap.Strings("excluded", excluded),
		)
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
