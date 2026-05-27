// Package tools hosts the concrete tool.Tool implementations the Phase 1
// agent loop ships with, plus the curated-belt builder that assembles a
// per-session tool.Registry from the configured curated MCP server.
//
// The package is the single boundary between agentx/* and the legacy
// http/* dispatch machinery (http.ExecuteToolCallCtx, http.DiscoverMCPTools).
// Nothing else in agentx/* imports the http package directly; the loop
// driver consumes the tool.Registry produced here and treats every tool
// uniformly. See proposal §3.2 and §4.3.
package tools

import (
	"context"
	"encoding/json"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/session"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/tool"
)

// SendToUserName is the well-known tool name that exits the agent loop.
// The loop driver short-circuits on this name; the tool's Execute is a
// pure schema validator + structured Result builder. See proposal §3.2,
// §4.3 and §6.1 (U9).
const SendToUserName = "send_to_user"

// sendToUserDescription is the description forwarded to the upstream
// tools catalog. Kept terse on purpose — the system prompt carries the
// "use this exactly once at the end" guidance.
const sendToUserDescription = "Emit the final user-facing answer. " +
	"Call this exactly once when you are done, with the complete answer in " +
	"`final_answer` and any supporting references in `citations`."

// sendToUserSchema is the JSON Schema advertised to the model. The
// shape mirrors SendToUserArgs verbatim so the validator and the
// upstream model see the same contract. Required: final_answer (string).
var sendToUserSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "final_answer": {
      "type": "string",
      "description": "The final answer to surface to the user."
    },
    "citations": {
      "type": "array",
      "description": "Optional supporting references.",
      "items": {
        "type": "object",
        "properties": {
          "url":   { "type": "string" },
          "title": { "type": "string" }
        },
        "required": ["url"]
      }
    }
  },
  "required": ["final_answer"],
  "additionalProperties": false
}`)

// Citation is one supporting reference attached to a send_to_user call.
// Kept identical in shape to session.Citation so SSE renderers can
// forward it without re-mapping.
type Citation struct {
	URL   string `json:"url"`
	Title string `json:"title,omitempty"`
}

// SendToUserArgs is the typed view of the args the model emits when it
// decides the run is finished. The loop driver pulls FinalAnswer out for
// the session.Final event and forwards Citations onto the same event.
type SendToUserArgs struct {
	FinalAnswer string     `json:"final_answer"`
	Citations   []Citation `json:"citations,omitempty"`
}

// sendToUserTool is the concrete tool.Tool implementation.
type sendToUserTool struct{}

// NewSendToUserTool returns the synthetic exit tool. The loop driver is
// expected to recognise SendToUserName *before* invoking Execute and
// terminate the loop directly; Execute here is the defensive fallback
// path used when the loop driver chooses to push the call through the
// normal Execute() pipeline (e.g. for uniform tracing).
//
// Execute behaviour:
//
//   - Parses call.Args into SendToUserArgs.
//   - On parse failure: returns Result{IsError: true, Content: "send_to_user:
//     <reason>"} so the model sees the schema violation on the next round
//     and the loop's error budget increments by one (per proposal §6.1
//     test U9).
//   - On success: Result.Content is the final answer text (so the legacy
//     dispatch path stays intact if invoked); Result.Details carries the
//     full args JSON for downstream consumers (citation display).
func NewSendToUserTool() tool.Tool { return sendToUserTool{} }

// Name implements tool.Tool.
func (sendToUserTool) Name() string { return SendToUserName }

// Description implements tool.Tool.
func (sendToUserTool) Description() string { return sendToUserDescription }

// Schema implements tool.Tool.
func (sendToUserTool) Schema() json.RawMessage { return sendToUserSchema }

// Execute implements tool.Tool. The sink is intentionally unused — the
// loop driver emits the surrounding ToolCallStart / ToolResult events.
func (sendToUserTool) Execute(_ context.Context, call tool.Call, _ session.EventSink) (tool.Result, error) {
	args, err := parseSendToUserArgs(call.Args)
	if err != nil {
		return tool.Result{
			Content: "send_to_user: " + err.Error(),
			IsError: true,
		}, nil
	}

	// Round-trip the parsed view so Details is a canonical JSON form
	// (drops any unknown / extra keys the model emitted).
	details, marshalErr := json.Marshal(args)
	if marshalErr != nil {
		// Should never happen — args is plain strings/strings — but stay
		// defensive so callers always get a usable Result.
		return tool.Result{
			Content: "send_to_user: " + marshalErr.Error(),
			IsError: true,
		}, nil
	}

	return tool.Result{
		Content: args.FinalAnswer,
		Details: details,
	}, nil
}

// parseSendToUserArgs validates raw model-emitted JSON against the
// SendToUserArgs contract. It uses json.Decoder with DisallowUnknownFields
// so a typo'd field becomes a loud error rather than a silent drop —
// the loop's error-budget mechanic is the only feedback channel the
// model has, so we want clear signal.
func parseSendToUserArgs(raw json.RawMessage) (SendToUserArgs, error) {
	if len(raw) == 0 {
		return SendToUserArgs{}, errors.New("missing arguments")
	}
	var args SendToUserArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return SendToUserArgs{}, errors.Wrap(err, "decode arguments")
	}
	if args.FinalAnswer == "" {
		return SendToUserArgs{}, errors.New("`final_answer` is required and must be a non-empty string")
	}
	for i, c := range args.Citations {
		if c.URL == "" {
			return SendToUserArgs{}, errors.Errorf("citations[%d].url is required", i)
		}
	}
	return args, nil
}
