package hook

import (
	"encoding/json"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/model"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/tool"
)

// Point names a hook fire point. The six points lifted from pi-agent's
// ExtensionAPI together cover every cross-cutting concern the Phase 1+ loop
// is expected to host (memory, redaction, telemetry, budget enforcement,
// audit, write-gate, prompt-injection scanning).
type Point string

const (
	// PointSessionStart fires once at the top of loop.Run, before the
	// first iteration.
	PointSessionStart Point = "session_start"
	// PointContext fires before every model call. Hooks may rewrite the
	// input transcript (e.g. memory injection, system-prompt assembly).
	PointContext Point = "context"
	// PointBeforeToolCall fires for each tool call the model emits, prior
	// to execution. Hooks may rewrite args or deny via an error.
	PointBeforeToolCall Point = "before_tool_call"
	// PointAfterToolCall fires after a tool returns, before the result is
	// appended to the next-iteration input. Hooks may rewrite the result.
	PointAfterToolCall Point = "after_tool_call"
	// PointBeforeCompact fires before the loop compacts the transcript.
	// Phase 1 ships the point only; transcript snapshot fields land in
	// Phase 3.
	PointBeforeCompact Point = "before_compact"
	// PointSessionEnd fires once at loop termination, after Final has been
	// emitted, regardless of TerminatedBy.
	PointSessionEnd Point = "session_end"
)

// Caps carries the budget snapshot for hooks that want to inspect it.
// It mirrors loop.Caps (kept independent here to avoid the import cycle).
// Phase 1 reads only MaxIterations; the rest are reserved.
type Caps struct {
	MaxIterations          int
	MaxToolCalls           int
	MaxParallelToolCalls   int
	ErrorBudget            int
	CircuitBreakerRepeats  int
	WallClockSeconds       int
}

// SessionStartEvent carries the per-session identifiers and budget snapshot
// supplied to PointSessionStart hooks.
type SessionStartEvent struct {
	// SessionID is the loop-issued session identifier.
	SessionID string
	// Caps is a copy of the loop's iteration / wall-clock budgets.
	Caps Caps
}

// ContextEvent is fired before every model call. Hooks return a (possibly
// modified) copy of Input; the loop forwards the final transformed Input
// to the upstream model.
type ContextEvent struct {
	// Input is the message + function_call + function_call_output sequence
	// staged for the next model invocation. Hooks treat the slice as
	// immutable and return a new (or mutated copy) reference.
	Input []model.InputItem
}

// ToolCallEvent is fired around each tool call. PointBeforeToolCall sees
// Result == nil; PointAfterToolCall sees Result populated.
type ToolCallEvent struct {
	// ToolName is the resolved registry name of the tool the model wants
	// to invoke.
	ToolName string
	// CallID is the upstream-supplied identifier for this invocation.
	CallID string
	// Args is the raw JSON argument payload, already schema-validated by
	// the loop.
	Args json.RawMessage
	// Result is nil for PointBeforeToolCall; populated for
	// PointAfterToolCall.
	Result *tool.Result
}

// CompactEvent is the PointBeforeCompact payload. Phase 1 ships an empty
// placeholder; transcript snapshot fields land in Phase 3.
type CompactEvent struct{}

// SessionEndEvent carries the terminal session state delivered to
// PointSessionEnd hooks. UserPrompt is included so the memory AfterTurnHook
// can persist (prompt, final) without having to dig back through the
// transcript — the loop already has both at hand at termination time.
type SessionEndEvent struct {
	// SessionID matches the one delivered to PointSessionStart.
	SessionID string
	// TerminatedBy matches RunFinished.TerminatedBy in agentx/session
	// ("send_to_user", "implicit_final", "ask_user", "iteration_cap",
	// "timeout", "circuit_breaker", "error_budget", "cancelled", "error").
	TerminatedBy string
	// FinalText is the assistant-facing answer text emitted as the loop's
	// Final event.
	FinalText string
	// UserPrompt is the original user-turn text. Memory hooks persist
	// only the (UserPrompt, FinalText) pair — never the tool trail.
	UserPrompt string
}
