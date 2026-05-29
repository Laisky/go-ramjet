package loop

import (
	"context"
	"fmt"
	"strings"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/distiller"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/hook"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/session"
)

// NewDistillHook returns an OnAfterToolCall hook that promotes the
// classic ReAct "Observation" role to first-class status: when a tool
// returns more than `thresholdTokens` worth of content the hook calls
// the distiller, replaces the raw bytes with a short, high-density
// summary, and stashes the original bytes on `stash` so the UI / audit
// trail can recover them.
//
// Ordering note: this hook must be registered BEFORE NewWrapHook so the
// untrusted-content delimiter wraps the *distilled* string (not the raw
// bytes the summariser already consumed). Wiring the other way around
// would waste tokens on the wrapping and force the summariser to look
// past the delimiter on every call.
//
// Pass-through conditions (no Distill call, no stash, no mutation):
//
//   - ev.Result == nil
//   - ev.Result.IsError — error text is usually short and the exact
//     wording matters; do not paraphrase errors.
//   - content already starts with `<tool_result ` — already wrapped on
//     an earlier hook fire (idempotence with NewWrapHook).
//   - EstimateTokens(content) <= thresholdTokens.
//
// The distiller is allowed to fall back to deterministic truncation
// when the LLM call errors or times out; the hook treats that as a
// successful distill (the fallback content already carries a visible
// failure header for the model).
func NewDistillHook(d distiller.Distiller, thresholdTokens int, stash *session.RawStash, userPrompt string) func(context.Context, hook.ToolCallEvent) (hook.ToolCallEvent, error) {
	if thresholdTokens <= 0 {
		thresholdTokens = distiller.DefaultThresholdTokens
	}
	return func(ctx context.Context, ev hook.ToolCallEvent) (hook.ToolCallEvent, error) {
		if d == nil || ev.Result == nil {
			return ev, nil
		}
		raw := ev.Result.Content
		if ev.Result.IsError {
			return ev, nil
		}
		if strings.HasPrefix(raw, wrapPrefix) {
			return ev, nil
		}
		if distiller.EstimateTokens(raw) <= thresholdTokens {
			return ev, nil
		}

		stash.Stash(ev.CallID, raw)

		res, err := d.Distill(ctx, distiller.Request{
			ToolName:   ev.ToolName,
			Args:       ev.Args,
			Raw:        raw,
			UserPrompt: userPrompt,
			CallID:     ev.CallID,
		})
		if err != nil && res.Content == "" {
			return ev, nil
		}

		footer := fmt.Sprintf("\n\n[observation distilled from %d-byte raw output; raw retained for call_id=%s]",
			len(raw), ev.CallID)
		next := *ev.Result
		next.Content = res.Content + footer
		ev.Result = &next
		return ev, nil
	}
}
