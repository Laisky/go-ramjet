package tools

import (
	"context"
	"encoding/json"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/session"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/tool"
	httppkg "github.com/Laisky/go-ramjet/internal/tasks/gptchat/http"
)

// LegacyDepsProvider lets the handler build LegacyDeps lazily per call,
// so the executor doesn't capture a stale pointer / closure. Provider is
// called once per tool execution; returning an error fails the call.
//
// The handler typically wires this with a closure that pulls request-scoped
// state (logger, user, frontend payload, raw token) out of its surrounding
// context — see proposal §5.1.
type LegacyDepsProvider interface {
	LegacyDeps(ctx context.Context, callID, toolName string) (httppkg.LegacyDeps, error)
}

// LegacyDepsFunc is the function adapter for LegacyDepsProvider so the
// handler can pass an inline closure without declaring a struct.
type LegacyDepsFunc func(ctx context.Context, callID, toolName string) (httppkg.LegacyDeps, error)

// LegacyDeps implements LegacyDepsProvider.
func (f LegacyDepsFunc) LegacyDeps(ctx context.Context, callID, toolName string) (httppkg.LegacyDeps, error) {
	return f(ctx, callID, toolName)
}

// legacyDispatcher is the function signature shared by the production
// http.ExecuteToolCallCtx and the test fakes. Kept as an unexported seam
// so this package can swap the implementation under test without breaking
// the public NewLegacyDispatchTool API.
type legacyDispatcher func(ctx context.Context, deps httppkg.LegacyDeps, fc httppkg.OpenAIResponsesFunctionCall) (string, string, error)

// defaultLegacyDispatcher routes calls to the production helper. Wrapped
// in a variable so tests can substitute it locally without exporting it.
var defaultLegacyDispatcher legacyDispatcher = httppkg.ExecuteToolCallCtx

// NewLegacyDispatchTool builds a tool.Tool whose Execute forwards to
// http.ExecuteToolCallCtx. Used for every curated MCP tool: the
// (name, description, schema) triple is what BuildCuratedBelt extracts
// from http.DiscoverMCPTools; the LegacyDepsProvider is constructed by
// the agent handler from the request scope.
//
// The returned tool is stateless and safe for concurrent use.
//
// Execute behaviour (per proposal §3.7's error-handling pattern):
//
//  1. Build the OpenAIResponsesFunctionCall envelope from the loop's
//     tool.Call.
//  2. Resolve LegacyDeps through the provider (a per-call closure on the
//     handler side keeps the deps fresh).
//  3. Call http.ExecuteToolCallCtx and return its output verbatim.
//  4. On error: surface the error text as Result.Content and set
//     Result.IsError=true so the model sees the failure on the next round
//     and the loop's error budget increments by one.
func NewLegacyDispatchTool(
	name, description string,
	schema json.RawMessage,
	deps LegacyDepsProvider,
) tool.Tool {
	return &legacyDispatchTool{
		name:        name,
		description: description,
		schema:      schema,
		deps:        deps,
	}
}

// legacyDispatchTool is the concrete implementation behind
// NewLegacyDispatchTool. It looks up the dispatcher at Execute time so
// tests can swap defaultLegacyDispatcher after construction (the curated
// belt is built once per request; tests register stubs after that).
type legacyDispatchTool struct {
	name        string
	description string
	schema      json.RawMessage
	deps        LegacyDepsProvider
}

// Name implements tool.Tool.
func (t *legacyDispatchTool) Name() string { return t.name }

// Description implements tool.Tool.
func (t *legacyDispatchTool) Description() string { return t.description }

// Schema implements tool.Tool.
func (t *legacyDispatchTool) Schema() json.RawMessage { return t.schema }

// Execute implements tool.Tool. The sink is intentionally unused: the
// loop's per-call event sink already wraps Execute with ToolCallStart /
// ToolResult events. Emitting from here would double-publish.
func (t *legacyDispatchTool) Execute(ctx context.Context, call tool.Call, _ session.EventSink) (tool.Result, error) {
	if t.deps == nil {
		return tool.Result{
			Content: "legacy dispatch: missing LegacyDepsProvider",
			IsError: true,
		}, nil
	}
	deps, err := t.deps.LegacyDeps(ctx, call.CallID, t.name)
	if err != nil {
		return tool.Result{
			Content: "legacy dispatch: " + err.Error(),
			IsError: true,
		}, errors.Wrap(err, "resolve legacy deps")
	}

	fc := httppkg.OpenAIResponsesFunctionCall{
		Type:      "function_call",
		CallID:    call.CallID,
		Name:      t.name,
		Arguments: string(call.Args),
	}

	out, _, execErr := defaultLegacyDispatcher(ctx, deps, fc)
	if execErr != nil {
		// Per proposal §3.7: tool-level failures are surfaced as
		// IsError=true Results, not Go errors. The loop's error budget
		// charges this, and the model sees the message on the next round.
		msg := execErr.Error()
		if out != "" {
			msg = out + ": " + msg
		}
		return tool.Result{Content: msg, IsError: true}, nil
	}
	return tool.Result{Content: out}, nil
}
