package session

import (
	stdjson "encoding/json"
	"time"
)

// Event is the wire-typed transcript entry produced by the agent loop.
// Each event carries a stable ULID id and an optional parent id so the
// transcript can be reconstructed as a tree.
type Event interface {
	isEvent()
	EventID() string
	ParentEventID() string
	Kind() string
	Timestamp() time.Time
}

// Blob is an opaque attachment carried with an OpUserTurn.
type Blob struct {
	Mime string `json:"mime"`
	Data []byte `json:"data"`
}

// BaseEvent is the common header embedded in every concrete event type.
// Construct one via NewBaseEvent (which mints a ULID) or fill the fields
// directly for tests.
type BaseEvent struct {
	ID        string    `json:"id"`
	ParentID  string    `json:"parent_id,omitempty"`
	EventKind string    `json:"kind"`
	At        time.Time `json:"timestamp"`
}

func (b BaseEvent) isEvent()              {}
func (b BaseEvent) EventID() string       { return b.ID }
func (b BaseEvent) ParentEventID() string { return b.ParentID }
func (b BaseEvent) Kind() string          { return b.EventKind }
func (b BaseEvent) Timestamp() time.Time  { return b.At }

// Event kinds. The §3.3 table is normative.
const (
	KindRunStarted              = "run_started"
	KindStepStarted             = "step_started"
	KindAssistantTextDelta      = "assistant_text_delta"
	KindAssistantReasoningDelta = "assistant_reasoning_delta"
	KindToolCallStart           = "tool_call_start"
	KindToolCallEnd             = "tool_call_end"
	KindToolResult              = "tool_result"
	KindStepFinished            = "step_finished"
	KindFinal                   = "final"
	KindRunFinished             = "run_finished"
	KindError                   = "error"
)

// Final.Origin values per §4.5.2 and §3.7.
const (
	FinalOriginSendToUser = "send_to_user"
	FinalOriginImplicit   = "implicit"
	FinalOriginAskUser    = "ask_user"
)

// RunFinished.TerminatedBy values per §7 acceptance criterion #5.
const (
	TerminatedBySendToUser     = "send_to_user"
	TerminatedByImplicitFinal  = "implicit_final"
	TerminatedByAskUser        = "ask_user"
	TerminatedByIterationCap   = "iteration_cap"
	TerminatedByTimeout        = "timeout"
	TerminatedByCircuitBreaker = "circuit_breaker"
	TerminatedByErrorBudget    = "error_budget"
	TerminatedByCancelled      = "cancelled"
	TerminatedByError          = "error"
)

// Citation is the typed reference attached to a Final answer.
type Citation struct {
	URL   string `json:"url,omitempty"`
	Title string `json:"title,omitempty"`
}

// TotalUsage is the aggregate usage attached to RunFinished.
type TotalUsage struct {
	TokensIn   int `json:"tokens_in"`
	TokensOut  int `json:"tokens_out"`
	ToolCalls  int `json:"tool_calls"`
	Iterations int `json:"iterations"`
}

// RunStarted marks the beginning of an agent run.
type RunStarted struct {
	BaseEvent
	RunID        string   `json:"run_id"`
	ModelID      string   `json:"model_id"`
	ToolNames    []string `json:"tool_names"`
	IterationCap int      `json:"iteration_cap"`
}

// StepStarted marks the beginning of a reasoning step inside the loop.
type StepStarted struct {
	BaseEvent
	StepID         string `json:"step_id"`
	IterationIndex int    `json:"iteration_index"`
}

// AssistantTextDelta carries a chunk of the assistant's user-facing prose.
type AssistantTextDelta struct {
	BaseEvent
	StepID string `json:"step_id"`
	Delta  string `json:"delta"`
}

// AssistantReasoningDelta carries a chunk of the assistant's private reasoning.
type AssistantReasoningDelta struct {
	BaseEvent
	StepID string `json:"step_id"`
	Delta  string `json:"delta"`
}

// ToolCallStart announces an outgoing tool invocation. CallID is required
// because §3.8 mandates per-call SSE prefixing.
type ToolCallStart struct {
	BaseEvent
	CallID      string `json:"call_id"`
	ToolName    string `json:"tool_name"`
	ArgsPreview string `json:"args_preview"`
}

// ToolCallEnd records the wall-clock time a tool spent executing.
type ToolCallEnd struct {
	BaseEvent
	CallID     string `json:"call_id"`
	DurationMS int64  `json:"duration_ms"`
}

// ToolResult carries the (possibly capped) tool output.
type ToolResult struct {
	BaseEvent
	CallID         string `json:"call_id"`
	ContentPreview string `json:"content_preview"`
	BytesTotal     int    `json:"bytes_total"`
	IsError        bool   `json:"is_error"`
}

// StepFinished marks the end of a reasoning step.
type StepFinished struct {
	BaseEvent
	StepID    string `json:"step_id"`
	TokensIn  int    `json:"tokens_in"`
	TokensOut int    `json:"tokens_out"`
}

// Final carries the user-visible answer. Origin discriminates between the
// model's explicit `send_to_user` call, an implicit-final fallback, and a
// hook-triggered ErrAskUser termination per §3.7.
type Final struct {
	BaseEvent
	FinalText string     `json:"final_text"`
	Citations []Citation `json:"citations,omitempty"`
	Origin    string     `json:"origin"`
}

// RunFinished is the terminal event emitted exactly once per run.
type RunFinished struct {
	BaseEvent
	RunID        string     `json:"run_id"`
	TerminatedBy string     `json:"terminated_by"`
	TotalUsage   TotalUsage `json:"total_usage"`
}

// Error is emitted for transport / runtime failures distinct from
// tool-level errors (which surface via ToolResult.IsError).
type Error struct {
	BaseEvent
	Code    string `json:"code"`
	Message string `json:"message"`
}

// envelope is the JSONL marshalling shape: header fields are stored alongside
// a kind-specific payload so the same line carries both routing data and the
// typed body.
type envelope struct {
	ID       string             `json:"id"`
	ParentID string             `json:"parent_id,omitempty"`
	Kind     string             `json:"kind"`
	At       time.Time          `json:"timestamp"`
	Payload  stdjson.RawMessage `json:"payload"`
}
