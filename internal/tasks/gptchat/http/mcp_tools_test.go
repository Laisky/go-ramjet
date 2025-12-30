package http

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestExtractToolsFromMCPServers_Basic tests basic tool extraction from MCP servers.
func TestExtractToolsFromMCPServers_Basic(t *testing.T) {
	servers := []MCPServerConfig{
		{
			ID:      "srv1",
			Name:    "Test Server",
			URL:     "https://mcp.example.com",
			Enabled: true,
			Tools: []json.RawMessage{
				json.RawMessage(`{"name":"web_search","description":"Search the web","input_schema":{"type":"object","properties":{"query":{"type":"string"}}}}`),
				json.RawMessage(`{"name":"calculator","description":"Perform calculations","parameters":{"type":"object","properties":{"expression":{"type":"string"}}}}`),
			},
		},
	}

	tools := extractToolsFromMCPServers(servers)
	require.Len(t, tools, 2)

	require.Equal(t, "function", tools[0].Type)
	require.Equal(t, "web_search", tools[0].Name)
	require.Equal(t, "Search the web", tools[0].Description)

	require.Equal(t, "function", tools[1].Type)
	require.Equal(t, "calculator", tools[1].Name)
	require.Equal(t, "Perform calculations", tools[1].Description)
}

// TestExtractToolsFromMCPServers_DisabledServer tests that disabled servers are skipped.
func TestExtractToolsFromMCPServers_DisabledServer(t *testing.T) {
	servers := []MCPServerConfig{
		{
			ID:      "srv1",
			Name:    "Disabled Server",
			URL:     "https://mcp.example.com",
			Enabled: false,
			Tools: []json.RawMessage{
				json.RawMessage(`{"name":"web_search","description":"Search the web"}`),
			},
		},
		{
			ID:      "srv2",
			Name:    "Enabled Server",
			URL:     "https://mcp2.example.com",
			Enabled: true,
			Tools: []json.RawMessage{
				json.RawMessage(`{"name":"calculator","description":"Calculate"}`),
			},
		},
	}

	tools := extractToolsFromMCPServers(servers)
	require.Len(t, tools, 1)
	require.Equal(t, "calculator", tools[0].Name)
}

// TestExtractToolsFromMCPServers_EnabledToolNames tests filtering by enabled_tool_names.
func TestExtractToolsFromMCPServers_EnabledToolNames(t *testing.T) {
	servers := []MCPServerConfig{
		{
			ID:              "srv1",
			Name:            "Test Server",
			URL:             "https://mcp.example.com",
			Enabled:         true,
			EnabledToolName: []string{"web_search"}, // Only web_search is enabled
			Tools: []json.RawMessage{
				json.RawMessage(`{"name":"web_search","description":"Search the web"}`),
				json.RawMessage(`{"name":"calculator","description":"Calculate"}`),
				json.RawMessage(`{"name":"weather","description":"Get weather"}`),
			},
		},
	}

	tools := extractToolsFromMCPServers(servers)
	require.Len(t, tools, 1)
	require.Equal(t, "web_search", tools[0].Name)
}

// TestExtractToolsFromMCPServers_NestedFunction tests extraction from nested function format.
func TestExtractToolsFromMCPServers_NestedFunction(t *testing.T) {
	servers := []MCPServerConfig{
		{
			ID:      "srv1",
			Name:    "Test Server",
			URL:     "https://mcp.example.com",
			Enabled: true,
			Tools: []json.RawMessage{
				json.RawMessage(`{"type":"function","function":{"name":"web_search","description":"Search the web","parameters":{"type":"object"}}}`),
			},
		},
	}

	tools := extractToolsFromMCPServers(servers)
	require.Len(t, tools, 1)
	require.Equal(t, "web_search", tools[0].Name)
	require.Equal(t, "Search the web", tools[0].Description)
}

// TestExtractToolsFromMCPServers_EmptyServers tests handling of empty input.
func TestExtractToolsFromMCPServers_EmptyServers(t *testing.T) {
	tools := extractToolsFromMCPServers(nil)
	require.Empty(t, tools)

	tools = extractToolsFromMCPServers([]MCPServerConfig{})
	require.Empty(t, tools)
}

// TestExtractToolsFromMCPServers_NoTools tests servers with no tools.
func TestExtractToolsFromMCPServers_NoTools(t *testing.T) {
	servers := []MCPServerConfig{
		{
			ID:      "srv1",
			Name:    "Empty Server",
			URL:     "https://mcp.example.com",
			Enabled: true,
			Tools:   nil,
		},
	}

	tools := extractToolsFromMCPServers(servers)
	require.Empty(t, tools)
}

// TestExtractToolsFromMCPServers_InvalidJSON tests handling of invalid tool JSON.
func TestExtractToolsFromMCPServers_InvalidJSON(t *testing.T) {
	servers := []MCPServerConfig{
		{
			ID:      "srv1",
			Name:    "Test Server",
			URL:     "https://mcp.example.com",
			Enabled: true,
			Tools: []json.RawMessage{
				json.RawMessage(`{invalid json}`),
				json.RawMessage(`{"name":"valid_tool","description":"Valid"}`),
			},
		},
	}

	tools := extractToolsFromMCPServers(servers)
	require.Len(t, tools, 1)
	require.Equal(t, "valid_tool", tools[0].Name)
}

// TestExtractToolsFromMCPServers_EmptyName tests that tools without names are skipped.
func TestExtractToolsFromMCPServers_EmptyName(t *testing.T) {
	servers := []MCPServerConfig{
		{
			ID:      "srv1",
			Name:    "Test Server",
			URL:     "https://mcp.example.com",
			Enabled: true,
			Tools: []json.RawMessage{
				json.RawMessage(`{"description":"No name tool"}`),
				json.RawMessage(`{"name":"","description":"Empty name"}`),
				json.RawMessage(`{"name":"  ","description":"Whitespace name"}`),
				json.RawMessage(`{"name":"valid_tool","description":"Valid"}`),
			},
		},
	}

	tools := extractToolsFromMCPServers(servers)
	require.Len(t, tools, 1)
	require.Equal(t, "valid_tool", tools[0].Name)
}

// TestFindMCPServerForToolName tests finding the correct server for a tool.
func TestFindMCPServerForToolName(t *testing.T) {
	servers := []MCPServerConfig{
		{
			ID:      "srv1",
			Name:    "Server 1",
			URL:     "https://mcp1.example.com",
			Enabled: true,
			Tools: []json.RawMessage{
				json.RawMessage(`{"name":"web_search","description":"Search the web"}`),
			},
		},
		{
			ID:      "srv2",
			Name:    "Server 2",
			URL:     "https://mcp2.example.com",
			Enabled: true,
			Tools: []json.RawMessage{
				json.RawMessage(`{"name":"calculator","description":"Calculate"}`),
			},
		},
	}

	// Find web_search in srv1
	server := findMCPServerForToolName(servers, "web_search")
	require.NotNil(t, server)
	require.Equal(t, "srv1", server.ID)

	// Find calculator in srv2
	server = findMCPServerForToolName(servers, "calculator")
	require.NotNil(t, server)
	require.Equal(t, "srv2", server.ID)

	// Not found
	server = findMCPServerForToolName(servers, "nonexistent")
	require.Nil(t, server)

	// Empty name
	server = findMCPServerForToolName(servers, "")
	require.Nil(t, server)
}

// TestFindMCPServerForToolName_DisabledServer tests that disabled servers are skipped.
func TestFindMCPServerForToolName_DisabledServer(t *testing.T) {
	servers := []MCPServerConfig{
		{
			ID:      "srv1",
			Name:    "Disabled Server",
			URL:     "https://mcp1.example.com",
			Enabled: false,
			Tools: []json.RawMessage{
				json.RawMessage(`{"name":"web_search","description":"Search the web"}`),
			},
		},
	}

	server := findMCPServerForToolName(servers, "web_search")
	require.Nil(t, server)
}
