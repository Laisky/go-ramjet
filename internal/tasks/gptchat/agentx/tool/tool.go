// Package tool defines the unified Tool interface used by the agent loop.
//
// Local synthesized tools, curated MCP tools, and future user-supplied tools
// all sit behind this single shape. The Registry in registry.go applies a
// deterministic source-priority resolution rule so the same input always
// produces the same belt (see proposal §3.2).
package tool

import (
	"context"
	"encoding/json"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/session"
)

// Call describes a single tool invocation requested by the model.
//
// CallID is the upstream-supplied identifier used to correlate the call with
// its result; Name is the tool to invoke; Args is the raw JSON arguments
// already validated against the tool's schema by the caller (the loop).
type Call struct {
	CallID string
	Name   string
	Args   json.RawMessage
}

// Result is what a tool returns to the loop.
//
// Content is the LLM-facing rendering, already capped to the relevant size
// limit by AfterToolCall hooks. Details is an optional structured payload for
// the UI/trace. IsError signals a tool-level failure: the model sees the
// content, the loop's error budget increments by one.
type Result struct {
	Content string
	Details json.RawMessage
	IsError bool
}

// Tool is the single shape every executable tool implements.
//
// Name must be unique across the registry after deterministic resolution.
// Schema returns the JSON Schema for the tool's parameters; the loop forwards
// it to the upstream model. Execute runs the tool and may publish progress
// events through sink (when non-nil).
type Tool interface {
	Name() string
	Description() string
	Schema() json.RawMessage
	Execute(ctx context.Context, call Call, sink session.EventSink) (Result, error)
}

// Source identifies the origin of a tool registration. Lower numeric value
// means higher priority during resolution — see registry.Register.
type Source int

const (
	// SourceLocal is for local / synthesized tools (e.g. send_to_user).
	SourceLocal Source = 0
	// SourceCuratedMCP is for the curated MCP belt configured in
	// openai.agent_loop.
	SourceCuratedMCP Source = 1
	// SourceUserMCP is reserved for future user-supplied MCP tools surfaced
	// through frontendReq.MCPServers.
	SourceUserMCP Source = 2
)

// String returns a stable identifier for logs and error messages.
func (s Source) String() string {
	switch s {
	case SourceLocal:
		return "local"
	case SourceCuratedMCP:
		return "curated_mcp"
	case SourceUserMCP:
		return "user_mcp"
	default:
		return "unknown"
	}
}

// Descriptor is the public, read-only view of a registered tool. The loop
// uses Descriptors() to assemble the upstream tools array; the registry holds
// the live Tool behind it for execution.
type Descriptor struct {
	Name        string
	Description string
	Schema      json.RawMessage
	Source      Source
}
