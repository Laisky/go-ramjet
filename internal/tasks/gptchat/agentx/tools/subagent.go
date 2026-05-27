package tools

import (
	"context"
	"encoding/json"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/session"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/tool"
)

// SubAgentToolName is the reserved name for the spawn_agent tool. Per
// proposal §3.6 the name is locked in now so Phase 2 (when execution is
// implemented) ships without an API break. The tool is only registered
// when BeltDeps.SubagentEnabled=true; with default config callers see
// (nil, false) from Registry.Get(SubAgentToolName) — verified by U20.
const SubAgentToolName = "spawn_agent"

// SubAgentToolPhase1Error is the canonical content surfaced by the
// Phase 1 stub Execute. Exposed as a const so tests can pin against it.
const SubAgentToolPhase1Error = "subagent execution not enabled in this build"

// subAgentDescription is the description forwarded to the upstream
// tools catalog. Even though Phase 1 returns an error from Execute, the
// description still ships so the catalog stays stable across phases.
const subAgentDescription = "Spawn a constrained sub-agent for a focused " +
	"sub-task. The child sees only the tools listed in `allow_tools` and " +
	"returns once it calls send_to_user."

// subAgentSchema mirrors SubAgentArgs verbatim. Locked-in shape per
// proposal §3.6.
var subAgentSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "profile": {
      "type": "string",
      "description": "Child profile name (e.g. researcher, coder)."
    },
    "task": {
      "type": "string",
      "description": "What the child should accomplish."
    },
    "allow_tools": {
      "type": "array",
      "items": { "type": "string" },
      "description": "Subset of the parent's tool registry the child may use."
    },
    "output_mode": {
      "type": "string",
      "enum": ["inline", "file", "none"],
      "description": "How the child returns its result."
    }
  },
  "required": ["profile", "task"],
  "additionalProperties": false
}`)

// SubAgentArgs is the locked-in argument shape for spawn_agent. Phase 2
// will consume the same shape; Phase 1 only validates and rejects.
type SubAgentArgs struct {
	Profile    string   `json:"profile"`
	Task       string   `json:"task"`
	AllowTools []string `json:"allow_tools,omitempty"`
	OutputMode string   `json:"output_mode,omitempty"`
}

// SubAgentTool is the Phase 1 stub for the spawn_agent capability. Its
// presence in the registry is gated by BeltDeps.SubagentEnabled (proposal
// §3.6). When enabled, Execute returns a structured error result so the
// model sees an unmistakable signal that the capability is not yet wired
// up; when disabled, the tool is not registered and the model is never
// shown its existence.
type SubAgentTool struct {
	// MaxDepth is carried for Phase 2's recursion guard. Not enforced
	// today — Execute returns an error before any depth check runs.
	MaxDepth int
}

// NewSubAgentTool returns the Phase 1 stub spawn_agent tool. maxDepth is
// stored for Phase 2; pass <= 0 to keep the proposal's default (2) when
// the field is consulted by the not-yet-implemented executor.
func NewSubAgentTool(maxDepth int) tool.Tool {
	if maxDepth <= 0 {
		maxDepth = 2
	}
	return &SubAgentTool{MaxDepth: maxDepth}
}

// Name implements tool.Tool.
func (*SubAgentTool) Name() string { return SubAgentToolName }

// Description implements tool.Tool.
func (*SubAgentTool) Description() string { return subAgentDescription }

// Schema implements tool.Tool.
func (*SubAgentTool) Schema() json.RawMessage { return subAgentSchema }

// Execute implements tool.Tool. Phase 1 always returns an IsError result
// with SubAgentToolPhase1Error as the content. The model sees the
// failure on the next round, increments the loop's error budget by one,
// and (per the proposal's "default-exclude-self" hygiene) usually moves
// on rather than retrying.
func (*SubAgentTool) Execute(_ context.Context, _ tool.Call, _ session.EventSink) (tool.Result, error) {
	return tool.Result{
		Content: SubAgentToolPhase1Error,
		IsError: true,
	}, nil
}
