package tools

import (
	"context"
	"encoding/json"
	"sort"
	"testing"

	"github.com/Laisky/errors/v2"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/Laisky/zap/zaptest/observer"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/tool"
	httppkg "github.com/Laisky/go-ramjet/internal/tasks/gptchat/http"
)

// newObservedLogger returns a glog.Logger whose Warn lines are inspectable
// via the returned ObservedLogs. Used to assert the warning-emission
// contracts (missing curated tools, MCP discovery failure).
func newObservedLogger(t *testing.T) (glog.Logger, *observer.ObservedLogs) {
	t.Helper()
	core, logs := observer.New(zapcore.WarnLevel)
	l, err := glog.NewWithName("belt_test", glog.LevelWarn, zap.WrapCore(func(zapcore.Core) zapcore.Core {
		return core
	}))
	require.NoError(t, err)
	return l, logs
}

// installFakeDiscoverer swaps the package-level discoverer with the given
// fn and returns a cleanup function. Tests that exercise BuildCuratedBelt
// without a real MCP server use this seam.
func installFakeDiscoverer(t *testing.T, fn mcpDiscoverer) func() {
	t.Helper()
	orig := defaultMCPDiscoverer
	defaultMCPDiscoverer = fn
	return func() { defaultMCPDiscoverer = orig }
}

func TestBuildCuratedBelt_NoMCPServer_RegistersSendToUserOnly(t *testing.T) {
	t.Parallel()
	logger, _ := newObservedLogger(t)
	reg, err := BuildCuratedBelt(context.Background(), BeltDeps{
		Logger: logger,
	})
	require.NoError(t, err)
	require.Equal(t, []string{SendToUserName}, reg.Names())
}

// U20 — spawn_agent reservation. With SubagentEnabled=false (default),
// Registry.Get("spawn_agent") must return (nil, false). With
// SubagentEnabled=true, the tool exists and Execute returns the Phase 1
// stub error.
func TestBuildCuratedBelt_U20_SubagentReservation(t *testing.T) {
	t.Parallel()
	logger, _ := newObservedLogger(t)

	disabled, err := BuildCuratedBelt(context.Background(), BeltDeps{
		Logger:          logger,
		SubagentEnabled: false,
	})
	require.NoError(t, err)
	_, ok := disabled.Get(SubAgentToolName)
	require.False(t, ok, "default config must NOT register spawn_agent")

	enabled, err := BuildCuratedBelt(context.Background(), BeltDeps{
		Logger:          logger,
		SubagentEnabled: true,
	})
	require.NoError(t, err)
	spawn, ok := enabled.Get(SubAgentToolName)
	require.True(t, ok)
	res, execErr := spawn.Execute(context.Background(), tool.Call{Name: SubAgentToolName, Args: json.RawMessage(`{"profile":"r","task":"t"}`)}, nil)
	require.NoError(t, execErr)
	require.True(t, res.IsError)
	require.Equal(t, SubAgentToolPhase1Error, res.Content)
}

// U12 — Tool belt construction (fail-OPEN policy). Discovery returns 15
// tools spanning curated, memory, and operator names. The resulting
// registry's Names() must contain ALL discovered tools plus send_to_user,
// because the belt no longer filters by an include-list. The live MCP
// catalog at https://mcp.laisky.com advertises 17 tools and the prior
// whitelist silently dropped 6 of them; the regression fix is to
// register everything except an opt-out list (currently empty).
func TestBuildCuratedBelt_U12_RegistersEveryDiscoveredTool(t *testing.T) {
	// Cannot t.Parallel: mutates defaultMCPDiscoverer.
	discoveredNames := []string{
		"web_search", "web_fetch",
		"file_list", "file_stat", "file_read", "file_search",
		"file_write", "file_delete", "file_rename",
		"memory_before_turn", "memory_after_turn",
		// Names that the old whitelist would have silently dropped — these
		// are exactly the kind of tool the regression hides from the model.
		"find_tool", "get_user_request", "mcp_pipe", "extract_key_info",
	}
	provided := make([]httppkg.MCPToolDescriptor, 0, len(discoveredNames))
	for _, n := range discoveredNames {
		provided = append(provided, httppkg.MCPToolDescriptor{
			Name:        n,
			Description: "discovered " + n,
			InputSchema: json.RawMessage(`{"type":"object","properties":{"q":{"type":"string"}}}`),
		})
	}
	require.Len(t, provided, 15, "test fixture must have 15 tools")

	restore := installFakeDiscoverer(t, func(_ context.Context, _ *httppkg.MCPServerConfig, _ *httppkg.MCPCallOption) ([]httppkg.MCPToolDescriptor, error) {
		return provided, nil
	})
	defer restore()

	logger, logs := newObservedLogger(t)
	reg, err := BuildCuratedBelt(context.Background(), BeltDeps{
		Logger:       logger,
		MCPServer:    &httppkg.MCPServerConfig{URL: "https://mcp.laisky.com"},
		DepsProvider: LegacyDepsFunc(func(_ context.Context, _, _ string) (httppkg.LegacyDeps, error) { return httppkg.LegacyDeps{}, nil }),
	})
	require.NoError(t, err)

	names := reg.Names()
	// Expected: every discovered name + send_to_user, no drops.
	wantSet := append([]string{SendToUserName}, discoveredNames...)
	sort.Strings(wantSet)
	gotSorted := append([]string(nil), names...)
	sort.Strings(gotSorted)
	require.Equal(t, wantSet, gotSorted)
	require.Len(t, names, len(discoveredNames)+1,
		"belt must register all discovered tools + send_to_user")

	// Specifically guard the previously-silenced tools: web_search and
	// web_fetch are the live regression that triggered this fix; the
	// rest were never advertised under the old whitelist.
	for _, n := range []string{"web_search", "web_fetch", "find_tool", "mcp_pipe"} {
		_, ok := reg.Get(n)
		require.True(t, ok, "discovered tool %q must be registered", n)
	}

	// No exclude warning should fire on the empty-exclude default.
	require.Empty(t, logs.FilterMessage("agent_curated_belt_excluded_tools").All())
	// No missing-tool warning either — the old key must be gone.
	require.Empty(t, logs.FilterMessage("agent_curated_belt_missing_tools").All())
}

// CuratedBeltExcludes acts as an opt-out filter. When the exclude list
// contains a name and the MCP catalog advertises it, the registry must
// not register that tool and a single warning line must fire.
func TestBuildCuratedBelt_RespectsCuratedBeltExcludes(t *testing.T) {
	// Cannot t.Parallel: mutates defaultMCPDiscoverer AND CuratedBeltExcludes.
	originalExcludes := CuratedBeltExcludes
	CuratedBeltExcludes = []string{"shell_exec"}
	defer func() { CuratedBeltExcludes = originalExcludes }()

	provided := []httppkg.MCPToolDescriptor{
		{Name: "web_search", Description: "search", InputSchema: json.RawMessage(`{}`)},
		{Name: "shell_exec", Description: "danger", InputSchema: json.RawMessage(`{}`)},
		{Name: "file_read", Description: "read", InputSchema: json.RawMessage(`{}`)},
	}
	restore := installFakeDiscoverer(t, func(_ context.Context, _ *httppkg.MCPServerConfig, _ *httppkg.MCPCallOption) ([]httppkg.MCPToolDescriptor, error) {
		return provided, nil
	})
	defer restore()

	logger, logs := newObservedLogger(t)
	reg, err := BuildCuratedBelt(context.Background(), BeltDeps{
		Logger:       logger,
		MCPServer:    &httppkg.MCPServerConfig{URL: "https://mcp.laisky.com"},
		DepsProvider: LegacyDepsFunc(func(_ context.Context, _, _ string) (httppkg.LegacyDeps, error) { return httppkg.LegacyDeps{}, nil }),
	})
	require.NoError(t, err)

	_, ok := reg.Get("web_search")
	require.True(t, ok, "non-excluded tool must remain")
	_, ok = reg.Get("file_read")
	require.True(t, ok, "non-excluded tool must remain")
	_, ok = reg.Get("shell_exec")
	require.False(t, ok, "excluded tool must be dropped")

	excludedLogs := logs.FilterMessage("agent_curated_belt_excluded_tools").All()
	require.Len(t, excludedLogs, 1, "exclude warning must fire once")
	excludedField := excludedLogs[0].ContextMap()["excluded"]
	require.Contains(t, excludedField, "shell_exec")
}

// Determinism: shuffling the discovery response must produce the same
// registry order across runs (the registry sorts on Names()/Descriptors(),
// and the belt also sorts inputs by name before registering). Updated U19
// per the new fail-open contract.
func TestBuildCuratedBelt_DeterministicAcrossShuffledDiscovery(t *testing.T) {
	// Cannot t.Parallel: mutates defaultMCPDiscoverer.
	names := []string{
		"web_search", "web_fetch",
		"file_list", "file_stat", "file_read", "file_search",
		"file_write", "file_delete", "file_rename",
		"memory_before_turn", "memory_after_turn",
		"find_tool", "get_user_request", "mcp_pipe", "extract_key_info",
	}
	require.Len(t, names, 15)
	makeTools := func(order []string) []httppkg.MCPToolDescriptor {
		out := make([]httppkg.MCPToolDescriptor, 0, len(order))
		for _, n := range order {
			out = append(out, httppkg.MCPToolDescriptor{
				Name:        n,
				Description: "d " + n,
				InputSchema: json.RawMessage(`{"type":"object"}`),
			})
		}
		return out
	}
	// Build two distinct permutations of the same name set.
	rotated := append(append([]string{}, names[7:]...), names[:7]...)
	require.ElementsMatch(t, names, rotated)
	require.NotEqual(t, names, rotated)

	run := func(order []string) []string {
		restore := installFakeDiscoverer(t, func(_ context.Context, _ *httppkg.MCPServerConfig, _ *httppkg.MCPCallOption) ([]httppkg.MCPToolDescriptor, error) {
			return makeTools(order), nil
		})
		defer restore()
		logger, _ := newObservedLogger(t)
		reg, err := BuildCuratedBelt(context.Background(), BeltDeps{
			Logger:       logger,
			MCPServer:    &httppkg.MCPServerConfig{URL: "https://mcp.laisky.com"},
			DepsProvider: LegacyDepsFunc(func(_ context.Context, _, _ string) (httppkg.LegacyDeps, error) { return httppkg.LegacyDeps{}, nil }),
		})
		require.NoError(t, err)
		return reg.Names()
	}
	first := run(names)
	second := run(rotated)
	require.Equal(t, first, second, "registry order must be invariant under discovery shuffles")
	require.Len(t, first, len(names)+1, "all 15 + send_to_user must be registered")
}

// Fallback belt on MCP failure. Stub DiscoverMCPTools returning an error;
// BuildCuratedBelt registers FallbackBelt names as IsError-returning
// stubs and emits a discovery warning.
func TestBuildCuratedBelt_FallbackBelt_OnDiscoveryFailure(t *testing.T) {
	// Cannot t.Parallel: mutates defaultMCPDiscoverer.
	cause := errors.New("connect: connection refused")
	restore := installFakeDiscoverer(t, func(_ context.Context, _ *httppkg.MCPServerConfig, _ *httppkg.MCPCallOption) ([]httppkg.MCPToolDescriptor, error) {
		return nil, cause
	})
	defer restore()

	logger, logs := newObservedLogger(t)
	reg, err := BuildCuratedBelt(context.Background(), BeltDeps{
		Logger:       logger,
		MCPServer:    &httppkg.MCPServerConfig{URL: "https://mcp.laisky.com"},
		DepsProvider: LegacyDepsFunc(func(_ context.Context, _, _ string) (httppkg.LegacyDeps, error) { return httppkg.LegacyDeps{}, nil }),
		FallbackBelt: []string{"web_search", "web_fetch", "file_read"},
	})
	require.NoError(t, err)

	// All fallback names exist as IsError stubs.
	for _, n := range []string{"web_search", "web_fetch", "file_read"} {
		got, ok := reg.Get(n)
		require.True(t, ok, "fallback tool %q must be registered", n)
		res, execErr := got.Execute(context.Background(), tool.Call{Name: n, Args: json.RawMessage(`{}`)}, nil)
		require.NoError(t, execErr)
		require.True(t, res.IsError)
		require.Contains(t, res.Content, "connect: connection refused")
	}

	// send_to_user still present (it is registered before discovery).
	_, ok := reg.Get(SendToUserName)
	require.True(t, ok)

	// Warning fired with the discovery failure.
	discoveryWarn := logs.FilterMessage("agent_mcp_discovery_failed").All()
	require.Len(t, discoveryWarn, 1)
	require.Contains(t, discoveryWarn[0].ContextMap()["server_url"], "mcp.laisky.com")
}

// When FallbackBelt is empty and discovery fails, BuildCuratedBelt must
// return the wrapped error rather than silently producing a registry
// containing only send_to_user.
func TestBuildCuratedBelt_DiscoveryFailureWithoutFallback_Errors(t *testing.T) {
	cause := errors.New("upstream 500")
	restore := installFakeDiscoverer(t, func(_ context.Context, _ *httppkg.MCPServerConfig, _ *httppkg.MCPCallOption) ([]httppkg.MCPToolDescriptor, error) {
		return nil, cause
	})
	defer restore()

	logger, _ := newObservedLogger(t)
	_, err := BuildCuratedBelt(context.Background(), BeltDeps{
		Logger:       logger,
		MCPServer:    &httppkg.MCPServerConfig{URL: "https://mcp.laisky.com"},
		DepsProvider: LegacyDepsFunc(func(_ context.Context, _, _ string) (httppkg.LegacyDeps, error) { return httppkg.LegacyDeps{}, nil }),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "upstream 500")
}

// Curated tools must end up with SourceCuratedMCP; send_to_user must be
// SourceLocal. Verifies the source priority is wired correctly so the
// registry's deterministic resolution rule applies.
func TestBuildCuratedBelt_SourcesAssignedCorrectly(t *testing.T) {
	restore := installFakeDiscoverer(t, func(_ context.Context, _ *httppkg.MCPServerConfig, _ *httppkg.MCPCallOption) ([]httppkg.MCPToolDescriptor, error) {
		return []httppkg.MCPToolDescriptor{
			{Name: "web_search", Description: "search", InputSchema: json.RawMessage(`{}`)},
		}, nil
	})
	defer restore()

	logger, _ := newObservedLogger(t)
	reg, err := BuildCuratedBelt(context.Background(), BeltDeps{
		Logger:          logger,
		MCPServer:       &httppkg.MCPServerConfig{URL: "https://mcp.laisky.com"},
		DepsProvider:    LegacyDepsFunc(func(_ context.Context, _, _ string) (httppkg.LegacyDeps, error) { return httppkg.LegacyDeps{}, nil }),
		SubagentEnabled: true,
	})
	require.NoError(t, err)

	descs := reg.Descriptors()
	got := make(map[string]string, len(descs))
	for _, d := range descs {
		got[d.Name] = d.Source.String()
	}
	require.Equal(t, "local", got[SendToUserName])
	require.Equal(t, "local", got[SubAgentToolName])
	require.Equal(t, "curated_mcp", got["web_search"])
}

// Curated tools with empty InputSchema must still get a usable schema in
// the registry (the upstream model requires a non-empty parameters object).
func TestBuildCuratedBelt_EmptySchemaDefaultsToObject(t *testing.T) {
	restore := installFakeDiscoverer(t, func(_ context.Context, _ *httppkg.MCPServerConfig, _ *httppkg.MCPCallOption) ([]httppkg.MCPToolDescriptor, error) {
		return []httppkg.MCPToolDescriptor{
			{Name: "web_search", Description: "search", InputSchema: nil},
		}, nil
	})
	defer restore()

	logger, _ := newObservedLogger(t)
	reg, err := BuildCuratedBelt(context.Background(), BeltDeps{
		Logger:       logger,
		MCPServer:    &httppkg.MCPServerConfig{URL: "https://mcp.laisky.com"},
		DepsProvider: LegacyDepsFunc(func(_ context.Context, _, _ string) (httppkg.LegacyDeps, error) { return httppkg.LegacyDeps{}, nil }),
	})
	require.NoError(t, err)
	got, ok := reg.Get("web_search")
	require.True(t, ok)
	require.JSONEq(t, `{"type":"object"}`, string(got.Schema()))
}
