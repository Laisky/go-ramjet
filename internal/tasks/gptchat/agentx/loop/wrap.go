package loop

import (
	"context"
	"fmt"
	"strings"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/hook"
)

// wrapPrefix is the literal start of the trusted delimiter. We rely on this
// for idempotence: a result already wrapped is left untouched.
const wrapPrefix = "<tool_result "

// closeTagLiteral is the literal closing tag that the model would interpret
// as the end of an untrusted block. Any occurrence inside content must be
// escaped before we wrap, so a hostile tool output cannot inject a fake
// "untrusted" boundary into the system prompt's parsing rules.
const closeTagLiteral = "</tool_result>"

// closeTagEscape is the escape sequence we substitute for any literal close
// tag inside content. Documented in U10's golden expectation: the model
// sees a self-closing sentinel that round-trips visually but is structurally
// distinct from the wrapping delimiter.
const closeTagEscape = "<tool_result_close/>"

// NewWrapHook returns an OnAfterToolCall hook that wraps the (capped) tool
// output in <tool_result tool="…" trust="untrusted">…</tool_result>
// delimiters per proposal §4.4. Idempotent: if Result.Content already starts
// with the wrap prefix the hook is a no-op.
//
// Any literal </tool_result> inside the content is escaped to
// <tool_result_close/> first, so a hostile tool cannot inject a fake close
// tag that would corrupt the LLM's parsing of the untrusted block.
func NewWrapHook() func(context.Context, hook.ToolCallEvent) (hook.ToolCallEvent, error) {
	return func(_ context.Context, ev hook.ToolCallEvent) (hook.ToolCallEvent, error) {
		if ev.Result == nil {
			return ev, nil
		}
		content := ev.Result.Content
		if strings.HasPrefix(content, wrapPrefix) {
			// Already wrapped (e.g. nested hook re-runs or a tool that
			// pre-wrapped its own output). Keep as-is to preserve
			// idempotence.
			return ev, nil
		}
		escaped := strings.ReplaceAll(content, closeTagLiteral, closeTagEscape)
		wrapped := fmt.Sprintf(
			`<tool_result tool=%q trust="untrusted">%s</tool_result>`,
			ev.ToolName, escaped,
		)
		// Shallow-copy the result so the hook chain sees a new pointer; the
		// upstream caller's Result is not mutated through aliasing.
		next := *ev.Result
		next.Content = wrapped
		ev.Result = &next
		return ev, nil
	}
}
