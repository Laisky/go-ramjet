package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
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

// TestGuessJSONRPCEndpoints tests endpoint URL generation.
func TestGuessJSONRPCEndpoints(t *testing.T) {
	tests := []struct {
		name     string
		server   *MCPServerConfig
		expected []string
	}{
		{
			name: "basic URL like mcp.laisky.com",
			server: &MCPServerConfig{
				URL: "https://mcp.laisky.com",
			},
			// Order matters: exact URL first, then root /, then /mcp paths
			expected: []string{
				"https://mcp.laisky.com",
				"https://mcp.laisky.com/",
				"https://mcp.laisky.com/mcp",
				"https://mcp.laisky.com/mcp/tools",
			},
		},
		{
			name: "URL with trailing slash",
			server: &MCPServerConfig{
				URL: "https://mcp.laisky.com/",
			},
			expected: []string{
				"https://mcp.laisky.com",
				"https://mcp.laisky.com/",
				"https://mcp.laisky.com/mcp",
				"https://mcp.laisky.com/mcp/tools",
			},
		},
		{
			name: "URL with path",
			server: &MCPServerConfig{
				URL: "https://api.example.com/v1/mcp",
			},
			expected: []string{
				"https://api.example.com/v1/mcp",
				"https://api.example.com/",
				"https://api.example.com/mcp",
				"https://api.example.com/mcp/tools",
			},
		},
		{
			name: "URL with URLPrefix",
			server: &MCPServerConfig{
				URL:       "https://mcp.example.com",
				URLPrefix: "/api/v2",
			},
			expected: []string{
				"https://mcp.example.com",
				"https://mcp.example.com/",
				"https://mcp.example.com/mcp",
				"https://mcp.example.com/mcp/tools",
				"https://mcp.example.com/api/v2",
			},
		},
		{
			name:     "empty URL",
			server:   &MCPServerConfig{},
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := guessJSONRPCEndpoints(tt.server)
			// Check that all expected endpoints are present
			for _, exp := range tt.expected {
				found := false
				for _, r := range result {
					if r == exp {
						found = true
						break
					}
				}
				require.True(t, found, "expected endpoint %q not found in %v", exp, result)
			}
		})
	}
}

// TestMCPSessionHeaders tests header generation.
func TestMCPSessionHeaders(t *testing.T) {
	t.Run("without session ID", func(t *testing.T) {
		server := &MCPServerConfig{
			URL: "https://mcp.example.com",
		}
		base := http.Header{}
		base.Set("content-type", "application/json")

		h := mcpSessionHeaders(server, base)

		require.Equal(t, "application/json", h.Get("content-type"))
		require.Equal(t, "2025-06-18", h.Get("mcp-protocol-version"))
		require.Empty(t, h.Get("mcp-session-id"), "session ID should not be set before initialization")
	})

	t.Run("with session ID", func(t *testing.T) {
		server := &MCPServerConfig{
			URL:                "https://mcp.example.com",
			MCPSessionID:       "mcp-session-abc123",
			MCPProtocolVersion: "2025-06-18",
		}
		base := http.Header{}

		h := mcpSessionHeaders(server, base)

		require.Equal(t, "2025-06-18", h.Get("mcp-protocol-version"))
		require.Equal(t, "mcp-session-abc123", h.Get("mcp-session-id"))
	})
}

// TestMCPAuthCandidates tests authentication header generation.
func TestMCPAuthCandidates(t *testing.T) {
	t.Run("with API key", func(t *testing.T) {
		auths := mcpAuthCandidates("sk-test-key-123")
		require.NotEmpty(t, auths)
		// Should include Bearer format
		found := false
		for _, a := range auths {
			if a == "Bearer sk-test-key-123" {
				found = true
				break
			}
		}
		require.True(t, found, "Bearer format not found in %v", auths)
	})

	t.Run("empty API key", func(t *testing.T) {
		auths := mcpAuthCandidates("")
		require.Empty(t, auths)
	})
}

func TestConvertFrontendToResponsesRequest_MCP_Enablement(t *testing.T) {
	t.Run("EnableMCP is nil (backward compatibility)", func(t *testing.T) {
frontendReq := &FrontendReq{
			Model:     "gpt-4o",
			EnableMCP: nil,
			MCPServers: []MCPServerConfig{
				{
					Enabled: true,
					Tools: []json.RawMessage{
						json.RawMessage(`{"name": "test_tool", "description": "test tool"}`),
					},
				},
			},
		}

		respReq, err := convertFrontendToResponsesRequest(frontendReq)
		require.NoError(t, err)
		require.NotNil(t, respReq)
		require.NotEmpty(t, respReq.Tools)
		require.Equal(t, "test_tool", respReq.Tools[0].Name)
	})

	t.Run("EnableMCP is true", func(t *testing.T) {
enable := true
frontendReq := &FrontendReq{
			Model:     "gpt-4o",
			EnableMCP: &enable,
			MCPServers: []MCPServerConfig{
				{
					Enabled: true,
					Tools: []json.RawMessage{
						json.RawMessage(`{"name": "test_tool", "description": "test tool"}`),
					},
				},
			},
		}

		respReq, err := convertFrontendToResponsesRequest(frontendReq)
		require.NoError(t, err)
		require.NotNil(t, respReq)
		require.NotEmpty(t, respReq.Tools)
		require.Equal(t, "test_tool", respReq.Tools[0].Name)
	})

	t.Run("EnableMCP is false", func(t *testing.T) {
enable := false
frontendReq := &FrontendReq{
			Model:     "gpt-4o",
			EnableMCP: &enable,
			MCPServers: []MCPServerConfig{
				{
					Enabled: true,
					Tools: []json.RawMessage{
						json.RawMessage(`{"name": "test_tool", "description": "test tool"}`),
					},
				},
			},
		}

		respReq, err := convertFrontendToResponsesRequest(frontendReq)
		require.NoError(t, err)
		require.NotNil(t, respReq)
		for _, tool := range respReq.Tools {
			require.NotEqual(t, "test_tool", tool.Name, "MCP tool should not be injected when EnableMCP is false")
		}
	})
}

func TestExecuteToolCall_MCP_Disabled(t *testing.T) {
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)

	enable := false
	frontendReq := &FrontendReq{
		EnableMCP: &enable,
		MCPServers: []MCPServerConfig{
			{
				Enabled: true,
				Tools: []json.RawMessage{
					json.RawMessage(`{"name": "test_tool"}`),
				},
			},
		},
	}

	fc := OpenAIResponsesFunctionCall{
		Name:      "test_tool",
		Arguments: "{}",
	}

	out, info, err := executeToolCall(ctx, nil, frontendReq, fc)
	require.Error(t, err)
	require.Contains(t, err.Error(), "MCP is disabled")
	require.Empty(t, out)
	require.Empty(t, info)
}
