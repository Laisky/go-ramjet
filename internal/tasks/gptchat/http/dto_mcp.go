package http

import "encoding/json"

// MCPServerConfig carries the MCP server configuration from the web UI.
// It is used by the backend to map tool names to servers and to execute MCP tool calls.
type MCPServerConfig struct {
	ID              string            `json:"id,omitempty"`
	Name            string            `json:"name,omitempty"`
	URL             string            `json:"url"`
	URLPrefix       string            `json:"url_prefix,omitempty"`
	APIKey          string            `json:"api_key,omitempty"`
	Enabled         bool              `json:"enabled"`
	EnabledToolName []string          `json:"enabled_tool_names,omitempty"`
	Tools           []json.RawMessage `json:"tools,omitempty"`

	// Session fields are optional and may be provided by the UI.
	MCPProtocolVersion string `json:"mcp_protocol_version,omitempty"`
	MCPSessionID       string `json:"mcp_session_id,omitempty"`
}
