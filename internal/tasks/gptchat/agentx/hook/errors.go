package hook

import "fmt"

// ErrAskUser, returned from any hook, exits the agent loop and produces a
// session.Final event whose text is Message. Equivalent to the model calling
// send_to_user with that message — same wire format, same UX. The loop
// detects this via errors.As(err, &hook.ErrAskUser{}).
//
// Living in the hook package (rather than loop) avoids the loop -> hook ->
// loop import cycle: hooks need to construct the sentinel, and the loop
// needs to detect it, but the hook package must not depend on loop.
//
// See proposal §3.7 for the design rationale (we diverge from Codex's
// retry-with-escalation pattern — the conversation turn boundary is the
// approval gate).
type ErrAskUser struct {
	// Code is a structured identifier for telemetry: "write_gate",
	// "circuit_breaker", … Hooks of the same kind share a Code.
	Code string
	// Message is the user-facing prompt rendered as the assistant message.
	Message string
	// Details carries optional structured context (proposed tool call,
	// arguments, etc.). May be nil.
	Details map[string]any
}

// Error renders the sentinel in the canonical "ask_user[code]: message"
// shape. The pointer receiver is intentional so callers that propagate the
// sentinel through wrap-and-unwrap chains can extract it with
// errors.As(err, &hook.ErrAskUser{}).
func (e *ErrAskUser) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("ask_user[%s]: %s", e.Code, e.Message)
}
