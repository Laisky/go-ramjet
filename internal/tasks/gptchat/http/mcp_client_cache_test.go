package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

// resetMCPTransportCache clears the package-level cache so tests start
// from a known state.
func resetMCPTransportCache() {
	mcpTransportCache.Range(func(k, _ any) bool {
		mcpTransportCache.Delete(k)
		return true
	})
}

// pathCounter is a per-path request counter used to verify the cache
// short-circuits REST probing on subsequent calls.
type pathCounter struct {
	counts sync.Map // map[string]*int64
}

func (p *pathCounter) inc(path string) {
	v, _ := p.counts.LoadOrStore(path, new(int64))
	atomic.AddInt64(v.(*int64), 1)
}

func (p *pathCounter) get(path string) int64 {
	v, ok := p.counts.Load(path)
	if !ok {
		return 0
	}
	return atomic.LoadInt64(v.(*int64))
}

func (p *pathCounter) total() int64 {
	var sum int64
	p.counts.Range(func(_, v any) bool {
		sum += atomic.LoadInt64(v.(*int64))
		return true
	})
	return sum
}

// TestMCPTransportCache_RESTHitSkipsProbe verifies that after a
// successful REST call the cache is populated and a follow-up call hits
// only the cached endpoint instead of sweeping all 8 candidates.
func TestMCPTransportCache_RESTHitSkipsProbe(t *testing.T) {
	if err := SetupHTTPCli(); err != nil {
		t.Skipf("failed to setup http client: %v", err)
	}
	resetMCPTransportCache()
	t.Cleanup(resetMCPTransportCache)

	pc := &pathCounter{}
	// Only the canonical REST suffix succeeds; everything else 404s.
	successPath := "/v1/tools/call"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pc.inc(r.URL.Path)
		if r.URL.Path != successPath {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content": []any{map[string]any{"type": "text", "text": "ok"}},
		})
	}))
	t.Cleanup(ts.Close)

	server := &MCPServerConfig{URL: ts.URL, APIKey: "k", Enabled: true}

	// First call: probes until /v1/tools/call succeeds. With the
	// guessMCPToolCallURLs order /v1/tools/call is first, so only one
	// request lands on the success path.
	res, err := callMCPTool(context.Background(), server, "tool", `{}`, nil)
	require.NoError(t, err)
	require.Contains(t, res, "ok")

	firstCallTotal := pc.total()
	require.GreaterOrEqual(t, firstCallTotal, int64(1))
	require.Equal(t, int64(1), pc.get(successPath))

	cached, ok := loadMCPTransport(server.URL)
	require.True(t, ok, "cache should be populated after first success")
	require.Equal(t, mcpTransportREST, cached.Transport)
	require.Equal(t, ts.URL+successPath, cached.Endpoint)

	// Second call: must hit only the cached endpoint, no other paths.
	res, err = callMCPTool(context.Background(), server, "tool", `{}`, nil)
	require.NoError(t, err)
	require.Contains(t, res, "ok")

	// Exactly one new request, all on the cached path.
	require.Equal(t, int64(2), pc.get(successPath))
	require.Equal(t, firstCallTotal+1, pc.total(),
		"second call should only hit the cached endpoint")
}

// TestMCPTransportCache_FailureInvalidates verifies that when the
// cached endpoint stops working the entry is dropped and a follow-up
// call re-probes.
func TestMCPTransportCache_FailureInvalidates(t *testing.T) {
	if err := SetupHTTPCli(); err != nil {
		t.Skipf("failed to setup http client: %v", err)
	}
	resetMCPTransportCache()
	t.Cleanup(resetMCPTransportCache)

	pc := &pathCounter{}
	successPath := "/v1/tools/call"
	// `breakSuccess` flips on between requests to simulate the cached
	// endpoint going dark.
	var breakSuccess atomic.Bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pc.inc(r.URL.Path)
		if r.URL.Path != successPath || breakSuccess.Load() {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content": []any{map[string]any{"type": "text", "text": "ok"}},
		})
	}))
	t.Cleanup(ts.Close)

	server := &MCPServerConfig{URL: ts.URL, APIKey: "k", Enabled: true}

	// Populate cache.
	_, err := callMCPTool(context.Background(), server, "tool", `{}`, nil)
	require.NoError(t, err)
	cached, ok := loadMCPTransport(server.URL)
	require.True(t, ok)
	require.Equal(t, mcpTransportREST, cached.Transport)

	// Break the previously-working path. Next call should drop the cache
	// and surface an error (no other path returns success either).
	breakSuccess.Store(true)
	_, err = callMCPTool(context.Background(), server, "tool", `{}`, nil)
	require.Error(t, err)
	_, ok = loadMCPTransport(server.URL)
	require.False(t, ok, "cache entry should be invalidated after cached-path failure")
}

// TestMCPTransportCache_JSONRPCHitSkipsRESTSweep verifies that a
// JSON-RPC cache hit bypasses the REST probe entirely.
func TestMCPTransportCache_JSONRPCHitSkipsRESTSweep(t *testing.T) {
	if err := SetupHTTPCli(); err != nil {
		t.Skipf("failed to setup http client: %v", err)
	}
	resetMCPTransportCache()
	t.Cleanup(resetMCPTransportCache)

	pc := &pathCounter{}
	// Mimic mcp.laisky.com: REST suffixes 404, JSON-RPC at "/" works.
	// All requests are POSTs with a body; REST bodies have a "name"
	// field, JSON-RPC bodies have "jsonrpc". The bare URL is the only
	// path that accepts the JSON-RPC envelope.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pc.inc(r.URL.Path)
		// REST sweep hits various /v1/tools/... paths; all must 404.
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		// At root, accept only JSON-RPC envelopes. The REST fallback
		// also tries the bare URL with a {"name": ...} body — 404 it.
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if _, ok := body["jsonrpc"]; !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      body["id"],
			"result": map[string]any{
				"content": []any{map[string]any{"type": "text", "text": "rpc-ok"}},
			},
		})
	}))
	t.Cleanup(ts.Close)

	server := &MCPServerConfig{URL: ts.URL, APIKey: "k", Enabled: true}

	// First call: REST probe sweeps and fails, then JSON-RPC succeeds.
	res, err := callMCPTool(context.Background(), server, "tool", `{}`, nil)
	require.NoError(t, err)
	require.Contains(t, res, "rpc-ok")

	cached, ok := loadMCPTransport(server.URL)
	require.True(t, ok)
	require.Equal(t, mcpTransportJSONRPC, cached.Transport)

	// Snapshot counts: any REST suffix hit during sweep should appear,
	// e.g. /v1/tools/call. Verify they were probed at least once.
	require.GreaterOrEqual(t, pc.get("/v1/tools/call"), int64(1),
		"first call should sweep REST candidates")
	restProbeCountBefore := pc.get("/v1/tools/call")

	// Second call: cache hit on JSON-RPC. REST paths must not be probed
	// again.
	res, err = callMCPTool(context.Background(), server, "tool", `{}`, nil)
	require.NoError(t, err)
	require.Contains(t, res, "rpc-ok")
	require.Equal(t, restProbeCountBefore, pc.get("/v1/tools/call"),
		"REST paths must not be probed on cache hit")
}

// TestMCPTransportCache_DistinctServers verifies entries are keyed per
// URL and do not bleed between unrelated servers.
func TestMCPTransportCache_DistinctServers(t *testing.T) {
	if err := SetupHTTPCli(); err != nil {
		t.Skipf("failed to setup http client: %v", err)
	}
	resetMCPTransportCache()
	t.Cleanup(resetMCPTransportCache)

	mkServer := func() *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v1/tools/call" {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"content": []any{map[string]any{"type": "text", "text": "ok"}},
			})
		}))
	}
	ts1 := mkServer()
	t.Cleanup(ts1.Close)
	ts2 := mkServer()
	t.Cleanup(ts2.Close)

	srv1 := &MCPServerConfig{URL: ts1.URL, APIKey: "k", Enabled: true}
	srv2 := &MCPServerConfig{URL: ts2.URL, APIKey: "k", Enabled: true}

	_, err := callMCPTool(context.Background(), srv1, "tool", `{}`, nil)
	require.NoError(t, err)
	_, err = callMCPTool(context.Background(), srv2, "tool", `{}`, nil)
	require.NoError(t, err)

	c1, ok := loadMCPTransport(srv1.URL)
	require.True(t, ok)
	c2, ok := loadMCPTransport(srv2.URL)
	require.True(t, ok)
	require.NotEqual(t, c1.Endpoint, c2.Endpoint, "cache must be per-URL")
}
