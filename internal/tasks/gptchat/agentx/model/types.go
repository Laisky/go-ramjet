// Package model defines the abstract LLM-client surface used by the agent
// loop. The loop knows nothing about OneAPI / OpenAI Responses / Anthropic
// Messages / Gemini etc; it only knows the Client interface and the typed
// Request / StreamChunk shapes in this file.
//
// See docs/proposals/2026-05-26-gptchat-react-agent-mode.md §3.4 for the
// rationale; §3.8 for parallel-tool-call capability gating; §4.5 for the
// streaming SSE event taxonomy that the OneAPI adapter consumes.
package model

import (
	"encoding/json"
)

// InputItem is one item in the Responses-API style transcript. Kept opaque
// here so this package can host future-shaped items (Anthropic blocks,
// Gemini parts) without churn. The OneAPI adapter validates concrete shapes
// at the boundary.
//
// Phase 1 accepts three concrete shapes:
//
//  1. httppkg.OpenAIResponsesInputMessage — a regular role/content message
//     (system, user, assistant). Content may be a string or a list of
//     content parts as defined by the Responses API.
//
//  2. httppkg.OpenAIResponsesFunctionCall — a finalized function_call item
//     to be appended back into the transcript (the model's previous tool
//     decision, replayed on the next turn).
//
//  3. httppkg.OpenAIResponsesFunctionCallOutput — the tool-result item that
//     pairs with a call_id from a prior function_call.
//
// Passing any other shape to Request.Input returns a descriptive error from
// Client.Stream before any upstream call is issued.
type InputItem = any

// FunctionCall is a typed, model-agnostic representation of a single
// function_call the upstream emitted. The loop validates the Arguments
// against the registered tool schema before dispatching.
type FunctionCall struct {
	// CallID is the unique upstream call_id that pairs the call with its
	// matching FunctionCallOutput on the next turn.
	CallID string
	// Name is the tool name as advertised in the Tools list.
	Name string
	// Arguments is the raw, possibly-incomplete JSON object the upstream
	// emitted as the call payload. The loop validates against the tool's
	// JSON Schema; this layer does not parse or massage it.
	Arguments json.RawMessage
}

// FunctionCallOutput carries a tool result back into the next-turn input.
// Output is expected to already be capped and (when policy says so) wrapped
// in <tool_result trust="..."> delimiters; this layer does not enforce that.
type FunctionCallOutput struct {
	CallID string
	Output string
}

// ToolDescriptor names and schematizes a tool the model may call. The
// adapter is responsible for translating this into the upstream's tool
// schema (function-typed OpenAIResponsesTool for OneAPI).
type ToolDescriptor struct {
	Name        string
	Description string
	// Schema is a JSON Schema object describing the tool parameters. The
	// raw bytes are forwarded verbatim to the upstream.
	Schema json.RawMessage
}

// Reasoning configures upstream reasoning behavior. Effort selects the
// compute budget; Summary selects what surface text is returned. Both are
// optional; the adapter omits the upstream "reasoning" field when nil.
type Reasoning struct {
	Effort  string // "low" | "medium" | "high"
	Summary string // "auto" | "concise" | "detailed"
}

// Request is the model-agnostic call to a Client.
type Request struct {
	// Model is the upstream model identifier (e.g. "anthropic/claude-…").
	Model string
	// Input is the ordered transcript fed to the upstream. See InputItem
	// for the shapes accepted by the OneAPI adapter.
	Input []InputItem
	// Tools lists the function tools the model may call this turn.
	Tools []ToolDescriptor
	// ToolChoice forces a particular choice; "auto" | "required" | a tool
	// reference. Nil means the adapter picks a sensible default.
	ToolChoice any
	// MaxOutputTokens caps the assistant's reply length on the wire.
	MaxOutputTokens uint
	// Reasoning configures reasoning effort/summary. Omitted when nil.
	Reasoning *Reasoning
	// Stream selects between SSE-streamed delta chunks and a single
	// non-streaming response that the adapter synthesizes into the same
	// StreamChunk channel.
	Stream bool
	// Temperature is the upstream sampling temperature.
	Temperature float64
	// TopP is the upstream nucleus sampling cap.
	TopP float64
	// ParallelToolCalls requests the upstream to emit multiple
	// function_call items per round when possible. The adapter forces
	// this false on the wire whenever Capabilities().SupportsParallelToolCalls
	// is false, regardless of what the caller set here.
	ParallelToolCalls bool
}

// StreamKind discriminates StreamChunk variants. New kinds added in later
// phases must be additive — consumers that see an unknown kind must skip
// it rather than fail.
type StreamKind int

const (
	// ChunkText carries an assistant content delta (the eventual final
	// answer or interim text). Text holds the delta bytes.
	ChunkText StreamKind = iota
	// ChunkReasoning carries an assistant reasoning / chain-of-thought
	// delta. Text holds the delta bytes.
	ChunkReasoning
	// ChunkFunction carries one finalized function_call. FunctionCall is
	// populated; multiple ChunkFunctions may arrive within a single
	// round when parallel_tool_calls is enabled.
	ChunkFunction
	// ChunkUsage carries a token-usage update. Usage is populated.
	ChunkUsage
	// ChunkDone signals that the upstream finished this round normally.
	// No further chunks will arrive on the channel.
	ChunkDone
	// ChunkError signals an upstream error. Text holds the human-readable
	// message and Err holds a Go error; the channel is closed immediately
	// after this chunk.
	ChunkError
)

// String implements fmt.Stringer for diagnostics and golden tests.
func (k StreamKind) String() string {
	switch k {
	case ChunkText:
		return "text"
	case ChunkReasoning:
		return "reasoning"
	case ChunkFunction:
		return "function"
	case ChunkUsage:
		return "usage"
	case ChunkDone:
		return "done"
	case ChunkError:
		return "error"
	default:
		return "unknown"
	}
}

// StreamChunk is the typed event a Client.Stream emits.
type StreamChunk struct {
	Kind         StreamKind
	Text         string
	FunctionCall *FunctionCall
	Usage        *Usage
	Err          error
}

// Usage describes the token cost of a single upstream call. Fields that
// are not provided by the upstream remain zero.
type Usage struct {
	InputTokens     int
	OutputTokens    int
	ReasoningTokens int
	Total           int
}

// Capabilities describes static properties of a Client implementation.
// The loop reads these at construction time to gate request shaping (e.g.
// disabling parallel_tool_calls for models that don't support it).
type Capabilities struct {
	// SupportsParallelToolCalls is true when the underlying model handles
	// `parallel_tool_calls: true` correctly (returns multiple function_call
	// items in one round).
	SupportsParallelToolCalls bool
	// SupportsReasoning is true when the underlying model exposes
	// reasoning summary deltas over the wire.
	SupportsReasoning bool
	// MaxContextTokens is the model's published context window. The loop
	// uses this as an upper hint for compaction; not enforced here.
	MaxContextTokens int
}
