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

// curatedNamesFixture returns the 11 curated names (10 curated + 1 known
// missing in some tests). The function is used to drive U12's "10 in
// belt + 5 extras" setup deterministically.
func curatedNamesFixture() []string {
	out := make([]string, len(CuratedBeltNames))
	copy(out, CuratedBeltNames)
	return out
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

// U12 — Tool belt construction. Discovery returns 15 tools (10 curated +
// 5 extras like system_exec). The resulting registry's Names() contains
// exactly the curated subset + send_to_user.
func TestBuildCuratedBelt_U12_FiltersToCuratedSet(t *testing.T) {
	// Cannot t.Parallel: mutates defaultMCPDiscoverer.
	curated := curatedNamesFixture()
	// Build a set of 10 curated tools (drop one to exercise the
	// missing-curated-tool warning path too).
	const dropped = "memory_after_turn"
	provided := make([]httppkg.MCPToolDescriptor, 0, 15)
	for _, n := range curated {
		if n == dropped {
			continue
		}
		provided = append(provided, httppkg.MCPToolDescriptor{
			Name:        n,
			Description: "curated " + n,
			InputSchema: json.RawMessage(`{"type":"object","properties":{"q":{"type":"string"}}}`),
		})
	}
	// Add 5 extras that must be filtered out.
	for _, extra := range []string{"system_exec", "shell_run", "git_log", "kube_apply", "ssh_exec"} {
		provided = append(provided, httppkg.MCPToolDescriptor{
			Name:        extra,
			Description: "extra " + extra,
			InputSchema: json.RawMessage(`{"type":"object"}`),
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
	// Expected: send_to_user (local) + all curated names provided.
	wantSet := make([]string, 0, len(curated))
	wantSet = append(wantSet, SendToUserName)
	for _, n := range curated {
		if n == dropped {
			continue
		}
		wantSet = append(wantSet, n)
	}
	sort.Strings(wantSet)
	gotSorted := append([]string(nil), names...)
	sort.Strings(gotSorted)
	require.Equal(t, wantSet, gotSorted)

	// Spot-check that an extra was NOT registered.
	_, ok := reg.Get("system_exec")
	require.False(t, ok, "extras outside CuratedBeltNames must be dropped")

	// Missing-curated-tool warning fires once with the dropped name.
	missingLogs := logs.FilterMessage("agent_curated_belt_missing_tools").All()
	require.Len(t, missingLogs, 1)
	missingField := missingLogs[0].ContextMap()["missing"]
	require.Contains(t, missingField, dropped)
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
