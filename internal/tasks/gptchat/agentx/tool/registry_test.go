package tool

import (
	"encoding/json"
	"math/rand"
	"testing"

	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/Laisky/zap/zaptest/observer"
	"github.com/stretchr/testify/require"
)

// newTestLogger returns a glog.Logger backed by an observer.Core so tests can
// inspect warning lines emitted by the registry.
func newTestLogger(t *testing.T) (glog.Logger, *observer.ObservedLogs) {
	t.Helper()
	core, logs := observer.New(zapcore.WarnLevel)
	l, err := glog.NewWithName("test", glog.LevelWarn, zap.WrapCore(func(zapcore.Core) zapcore.Core {
		return core
	}))
	require.NoError(t, err)
	return l, logs
}

// newTool builds a stub Tool for registry tests. The schema is a trivial
// JSON object so callers can distinguish two tools that share a name.
func newTool(name, description, schemaTag string) Tool {
	schema := json.RawMessage(`{"type":"object","tag":"` + schemaTag + `"}`)
	return &stubTool{
		name:        name,
		description: description,
		schema:      schema,
	}
}

func TestRegistry_RegisterAndGet(t *testing.T) {
	t.Parallel()
	logger, _ := newTestLogger(t)
	r := NewRegistry(logger)
	require.NoError(t, r.Register(newTool("alpha", "a", "t1"), SourceLocal))
	got, ok := r.Get("alpha")
	require.True(t, ok)
	require.Equal(t, "alpha", got.Name())
	_, ok = r.Get("missing")
	require.False(t, ok)
}

func TestRegistry_RegisterNilOrEmpty(t *testing.T) {
	t.Parallel()
	logger, _ := newTestLogger(t)
	r := NewRegistry(logger)
	require.Error(t, r.Register(nil, SourceLocal))
	require.Error(t, r.Register(&stubTool{name: ""}, SourceLocal))
}

// TestRegistry_NamesSortedPerSource confirms the documented global ordering:
// per-source sorted slices concatenated in Source priority order.
// Example from prompt: Local has ["x","a"], Curated has ["b"] → ["a","x","b"].
func TestRegistry_NamesSortedPerSource(t *testing.T) {
	t.Parallel()
	logger, _ := newTestLogger(t)
	r := NewRegistry(logger)
	require.NoError(t, r.Register(newTool("x", "", "1"), SourceLocal))
	require.NoError(t, r.Register(newTool("a", "", "1"), SourceLocal))
	require.NoError(t, r.Register(newTool("b", "", "1"), SourceCuratedMCP))
	require.Equal(t, []string{"a", "x", "b"}, r.Names())

	descs := r.Descriptors()
	require.Len(t, descs, 3)
	require.Equal(t, "a", descs[0].Name)
	require.Equal(t, SourceLocal, descs[0].Source)
	require.Equal(t, "x", descs[1].Name)
	require.Equal(t, SourceLocal, descs[1].Source)
	require.Equal(t, "b", descs[2].Name)
	require.Equal(t, SourceCuratedMCP, descs[2].Source)
}

func TestRegistry_NamesAcrossAllThreeSources(t *testing.T) {
	t.Parallel()
	logger, _ := newTestLogger(t)
	r := NewRegistry(logger)
	require.NoError(t, r.Register(newTool("user_z", "", ""), SourceUserMCP))
	require.NoError(t, r.Register(newTool("user_a", "", ""), SourceUserMCP))
	require.NoError(t, r.Register(newTool("curated_m", "", ""), SourceCuratedMCP))
	require.NoError(t, r.Register(newTool("curated_b", "", ""), SourceCuratedMCP))
	require.NoError(t, r.Register(newTool("local_q", "", ""), SourceLocal))
	require.NoError(t, r.Register(newTool("local_p", "", ""), SourceLocal))
	want := []string{
		"local_p", "local_q",
		"curated_b", "curated_m",
		"user_a", "user_z",
	}
	require.Equal(t, want, r.Names())
}

// U12: Subset construction returns exactly the requested names in sorted
// order and excludes everything else.
func TestRegistry_Subset_U12(t *testing.T) {
	t.Parallel()
	logger, _ := newTestLogger(t)
	r := NewRegistry(logger)
	// 15 tools across the three sources, matching the curated-belt sketch
	// in proposal §4.3.
	mcpNames := []string{
		"web_search", "web_fetch",
		"file_list", "file_stat", "file_read", "file_search",
		"file_write", "file_delete", "file_rename",
		"calc", "weather", "stocks",
	}
	for _, n := range mcpNames {
		require.NoError(t, r.Register(newTool(n, "", ""), SourceCuratedMCP))
	}
	require.NoError(t, r.Register(newTool("send_to_user", "", ""), SourceLocal))
	require.NoError(t, r.Register(newTool("debug_tool", "", ""), SourceLocal))
	require.NoError(t, r.Register(newTool("spawn_agent", "", ""), SourceLocal))
	require.Equal(t, 15, len(r.Names()))

	sub, err := r.Subset([]string{"web_search", "web_fetch", "send_to_user"})
	require.NoError(t, err)
	require.Equal(t, []string{"send_to_user", "web_fetch", "web_search"}, sub.Names())

	_, ok := sub.Get("file_write")
	require.False(t, ok)
	_, ok = sub.Get("web_search")
	require.True(t, ok)
}

// U19a: Subset with an unknown name errors out and leaves the receiver
// untouched.
func TestRegistry_Subset_UnknownName_U19a(t *testing.T) {
	t.Parallel()
	logger, _ := newTestLogger(t)
	r := NewRegistry(logger)
	require.NoError(t, r.Register(newTool("web_search", "", ""), SourceCuratedMCP))
	require.NoError(t, r.Register(newTool("send_to_user", "", ""), SourceLocal))
	before := r.Names()

	_, err := r.Subset([]string{"web_search", "no_such_tool"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "no_such_tool")

	// Receiver registry is unchanged.
	require.Equal(t, before, r.Names())
	_, ok := r.Get("web_search")
	require.True(t, ok)
}

// U19b: Deterministic resolution under randomized registration order.
//
// Register two distinct tools both named web_search, one with SourceLocal
// and one with SourceCuratedMCP. After 100 runs in randomized order the
// resolved tool is always the SourceLocal one and Names() is identical.
//
// The shadow warning fires exactly once per duplicate registration — never
// on Get(). With two registrations per run there is exactly one collision
// per run, hence exactly one warning entry per run.
func TestRegistry_DeterministicResolution_U19b(t *testing.T) {
	t.Parallel()
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 100; i++ {
		logger, logs := newTestLogger(t)
		r := NewRegistry(logger)

		localTool := newTool("web_search", "local description", "local_schema")
		curatedTool := newTool("web_search", "curated description", "curated_schema")

		tools := []struct {
			tool Tool
			src  Source
		}{
			{localTool, SourceLocal},
			{curatedTool, SourceCuratedMCP},
		}
		rng.Shuffle(len(tools), func(a, b int) { tools[a], tools[b] = tools[b], tools[a] })

		for _, tt := range tools {
			require.NoError(t, r.Register(tt.tool, tt.src))
		}

		got, ok := r.Get("web_search")
		require.True(t, ok, "iter %d: Get must resolve", i)
		require.Same(t, localTool, got, "iter %d: SourceLocal must win", i)
		require.Equal(t, []string{"web_search"}, r.Names())

		// Multiple Get/Names calls must never log.
		for k := 0; k < 5; k++ {
			_, _ = r.Get("web_search")
			_ = r.Names()
		}

		entries := logs.FilterMessage("agent_tool_shadowed").All()
		require.Len(t, entries, 1, "iter %d: exactly one shadow warning per duplicate registration", i)
		fields := entries[0].ContextMap()
		require.Equal(t, "web_search", fields["name"])
		require.Equal(t, "local", fields["kept_source"])
		require.Equal(t, "curated_mcp", fields["dropped_source"])
	}
}

// TestRegistry_ShadowLogFiresOncePerAttempt registers the same duplicate
// three times in a row; the warning fires once per register-attempt and the
// Get path never logs.
func TestRegistry_ShadowLogFiresOncePerAttempt(t *testing.T) {
	t.Parallel()
	logger, logs := newTestLogger(t)
	r := NewRegistry(logger)
	primary := newTool("web_search", "primary", "p")
	require.NoError(t, r.Register(primary, SourceLocal))
	// Three attempts at a lower-priority duplicate: each must emit one warning.
	for i := 0; i < 3; i++ {
		require.NoError(t, r.Register(newTool("web_search", "dup", "d"), SourceCuratedMCP))
	}
	got, ok := r.Get("web_search")
	require.True(t, ok)
	require.Same(t, primary, got)
	// Drum on Get and Names to confirm read paths don't log.
	for i := 0; i < 10; i++ {
		_, _ = r.Get("web_search")
		_ = r.Names()
		_ = r.Descriptors()
	}
	entries := logs.FilterMessage("agent_tool_shadowed").All()
	require.Len(t, entries, 3, "one warning per register attempt; reads never log")
}

// TestRegistry_HigherPriorityRegisteredSecondReplacesLower covers the case
// where a lower-priority tool is registered first and a higher-priority
// duplicate arrives later — the new one wins. (The U19b run also catches
// this path stochastically; this is the explicit deterministic check.)
func TestRegistry_HigherPriorityArrivesLast(t *testing.T) {
	t.Parallel()
	logger, logs := newTestLogger(t)
	r := NewRegistry(logger)
	loser := newTool("web_search", "curated", "c")
	winner := newTool("web_search", "local", "l")
	require.NoError(t, r.Register(loser, SourceCuratedMCP))
	require.NoError(t, r.Register(winner, SourceLocal))
	got, ok := r.Get("web_search")
	require.True(t, ok)
	require.Same(t, winner, got)
	entries := logs.FilterMessage("agent_tool_shadowed").All()
	require.Len(t, entries, 1)
	fields := entries[0].ContextMap()
	require.Equal(t, "local", fields["kept_source"])
	require.Equal(t, "curated_mcp", fields["dropped_source"])
}

// TestRegistry_EqualSourceDuplicateKeepsFirst confirms an equal-source
// duplicate registration is a no-op (existing entry retained) and still
// fires the shadow warning so callers can spot accidental double-registers.
func TestRegistry_EqualSourceDuplicate(t *testing.T) {
	t.Parallel()
	logger, logs := newTestLogger(t)
	r := NewRegistry(logger)
	first := newTool("web_search", "first", "1")
	second := newTool("web_search", "second", "2")
	require.NoError(t, r.Register(first, SourceCuratedMCP))
	require.NoError(t, r.Register(second, SourceCuratedMCP))
	got, ok := r.Get("web_search")
	require.True(t, ok)
	require.Same(t, first, got)
	entries := logs.FilterMessage("agent_tool_shadowed").All()
	require.Len(t, entries, 1)
}

func TestRegistry_NilLoggerTolerated(t *testing.T) {
	t.Parallel()
	r := NewRegistry(nil)
	require.NoError(t, r.Register(newTool("a", "", ""), SourceLocal))
	require.NoError(t, r.Register(newTool("a", "", ""), SourceCuratedMCP))
	got, ok := r.Get("a")
	require.True(t, ok)
	require.Equal(t, "a", got.Name())
}

func TestRegistry_SubsetInheritsSources(t *testing.T) {
	t.Parallel()
	logger, _ := newTestLogger(t)
	r := NewRegistry(logger)
	require.NoError(t, r.Register(newTool("send_to_user", "", ""), SourceLocal))
	require.NoError(t, r.Register(newTool("web_search", "", ""), SourceCuratedMCP))
	sub, err := r.Subset([]string{"web_search", "send_to_user"})
	require.NoError(t, err)
	require.Equal(t, []string{"send_to_user", "web_search"}, sub.Names())
	descs := sub.Descriptors()
	require.Equal(t, SourceLocal, descs[0].Source)
	require.Equal(t, SourceCuratedMCP, descs[1].Source)
}
