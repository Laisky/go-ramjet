package agentx

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
	httppkg "github.com/Laisky/go-ramjet/internal/tasks/gptchat/http"
)

// privateCuratedMCP starts a loopback JSON-RPC MCP server, standing in for an
// operator's curated server on a private or Tailscale address, and counts the
// requests it serves.
func privateCuratedMCP(t *testing.T, hits *atomic.Int32) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		var rpc struct {
			ID     any    `json:"id"`
			Method string `json:"method"`
		}
		payload, err := io.ReadAll(r.Body)
		if err != nil || json.Unmarshal(payload, &rpc) != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		result := map[string]any{}
		if rpc.Method == "tools/list" {
			result = map[string]any{"tools": []any{map[string]any{"name": "curated_tool", "description": "synthetic",
				"inputSchema": map[string]any{"type": "object"}}}}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": rpc.ID, "result": result})
	}))
	t.Cleanup(server.Close)
	return server
}

// TestCuratedMCPOnPrivateAddressStaysReachable keeps the operator-configured
// curated MCP server on the server client: it comes from configuration, not
// from the caller, so it may live on a private address.
func TestCuratedMCPOnPrivateAddressStaysReachable(t *testing.T) {
	require.NoError(t, httppkg.SetupHTTPCli())
	var hits atomic.Int32
	private := privateCuratedMCP(t, &hits)

	curated := resolveCuratedMCP(&config.AgentLoopConfig{MCPServer: private.URL})
	require.NotNil(t, curated)
	tools, err := httppkg.DiscoverMCPTools(t.Context(), curated, nil)
	require.NoError(t, err)
	require.Len(t, tools, 1)
	require.Positive(t, hits.Load())
}

// TestCallerMCPServerWithCuratedURLInheritsOnlyThatTrust marks a caller entry
// that repeats the curated URL as operator-curated, since every endpoint it
// derives shares that origin, while other caller entries stay untrusted.
func TestCallerMCPServerWithCuratedURLInheritsOnlyThatTrust(t *testing.T) {
	curated := resolveCuratedMCP(&config.AgentLoopConfig{MCPServer: "http://100.64.0.20:8080"})
	require.NotNil(t, curated)
	require.True(t, curated.OperatorCurated)

	var fromCaller []httppkg.MCPServerConfig
	require.NoError(t, json.Unmarshal([]byte(`[
		{"url":"http://100.64.0.20:8080","enabled":true,"OperatorCurated":true},
		{"url":"http://10.0.0.5","enabled":true,"OperatorCurated":true,"operator_curated":true}
	]`), &fromCaller))
	for _, server := range fromCaller {
		require.False(t, server.OperatorCurated, "callers cannot claim operator trust")
	}

	merged := forceMCPEnabledWithCuratedServer(&httppkg.FrontendReq{MCPServers: fromCaller}, curated)
	require.Len(t, merged.MCPServers, 2)
	require.True(t, merged.MCPServers[0].OperatorCurated)
	require.False(t, merged.MCPServers[1].OperatorCurated)
	require.False(t, fromCaller[0].OperatorCurated, "the caller's request is never mutated")
}
