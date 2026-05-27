package loop

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/hook"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/tool"
)

// Write-gate modes. The string values match the YAML config in proposal §5.4.
const (
	WriteGateAsk   = "ask"
	WriteGateAllow = "allow"
	WriteGateDeny  = "deny"
)

// writeGateTools is the hard-coded set of write-class tools per proposal §4.3.
// Exposing this as a parameter is future work; for Phase 1 the curated belt
// is fixed.
var writeGateTools = map[string]struct{}{
	"file_write":  {},
	"file_delete": {},
	"file_rename": {},
}

// isWriteTool reports whether the named tool is gated by the write-gate.
func isWriteTool(name string) bool {
	_, ok := writeGateTools[name]
	return ok
}

// NewWriteGateHook returns an OnBeforeToolCall hook that enforces the
// write-class tool policy per proposal §3.7 / §4.5:
//
//   - WriteGateAsk:   returns *hook.ErrAskUser{Code:"write_gate", Message:…}.
//     The loop catches this and terminates with TerminatedBy=ask_user.
//   - WriteGateDeny:  synthesizes IsError result on the event and returns
//     nil. The loop continues; the model sees the failure and may pick
//     another approach.
//   - WriteGateAllow: pass-through. Default for unrecognized modes also
//     passes through (fail-open is preferred over hard-fail on a config
//     typo; a misconfigured agent is better than a broken one).
//
// Non-write tools always pass through unchanged regardless of mode.
func NewWriteGateHook(mode string) func(context.Context, hook.ToolCallEvent) (hook.ToolCallEvent, error) {
	return func(_ context.Context, ev hook.ToolCallEvent) (hook.ToolCallEvent, error) {
		if ev.Result != nil {
			// Only fire on Before-tool-call; defensive pass-through if we
			// somehow get an After-shaped event.
			return ev, nil
		}
		if !isWriteTool(ev.ToolName) {
			return ev, nil
		}
		switch mode {
		case WriteGateAsk:
			return ev, &hook.ErrAskUser{
				Code:    "write_gate",
				Message: writeGateAskMessage(ev.ToolName, ev.Args),
				Details: map[string]any{
					"tool": ev.ToolName,
					"args": ev.Args,
				},
			}
		case WriteGateDeny:
			ev.Result = &tool.Result{
				Content: "write tools are disabled in this session",
				IsError: true,
			}
			return ev, nil
		case WriteGateAllow, "":
			return ev, nil
		default:
			// Unknown mode — pass through (fail-open).
			return ev, nil
		}
	}
}

// writeGateAskMessage renders the user-facing prompt shown when the model
// proposes a write. The format intentionally mirrors the example in
// proposal §3.7 so the UX is predictable.
func writeGateAskMessage(toolName string, args json.RawMessage) string {
	pretty := prettyJSON(args)
	return fmt.Sprintf(
		"I want to call `%s` with arguments:\n\n```json\n%s\n```\n\nShould I proceed? Reply 'yes' to confirm, or tell me a different approach.",
		toolName, pretty,
	)
}

// prettyJSON returns a 2-space indented JSON rendering of raw. Falls back to
// the raw bytes if parsing fails so we never panic on a malformed args
// payload from a hostile model.
func prettyJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "{}"
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return string(raw)
	}
	return string(b)
}
