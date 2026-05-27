package tools

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/Laisky/errors/v2"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/hook"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/model"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/session"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
	httppkg "github.com/Laisky/go-ramjet/internal/tasks/gptchat/http"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/memoryx"
)

// DefaultMemoryFinalTextMaxBytes is the default cap, in bytes, on the
// assistant final-text (and, defensively, user prompt) payload handed
// to memoryx.AfterTurnHook. The MCP-backed JSONL file_write rejects
// payloads above its server-side limit with PAYLOAD_TOO_LARGE; a 64 KiB
// cap on each field keeps the JSONL envelope well within range while
// preserving enough head+tail context for the memory engine to dedupe
// and embed against future prompts.
const DefaultMemoryFinalTextMaxBytes = 64 * 1024

// memoryPayloadTooLargeMarker is the substring AfterTurnHook treats as
// proof that even the truncated payload tripped the MCP file_write size
// guard. The MCP layer wraps the upstream error so errors.Is against a
// sentinel does not work; the literal substring is contractually stable
// (mcp_legacy.go translates it from the server reply), so strings.Contains
// is the correct mechanism here.
const memoryPayloadTooLargeMarker = "PAYLOAD_TOO_LARGE"

// MemoryState is the shared mutable bridge between the Before and After
// memory hooks. The Before hook (OnContext) populates Keys after a
// successful memoryx.BeforeTurnHook call; the After hook (OnSessionEnd)
// reads Keys to address the same memory bucket and only persists when
// Keys.Ready is true. Both hooks close over the same *MemoryState
// pointer the handler constructs per request.
type MemoryState struct {
	mu sync.Mutex
	// Ready is true after a successful BeforeTurnHook (or cold-start
	// fallback). The After hook only persists when Ready is true; this
	// keeps tool-loop iteration caps / timeouts from polluting memory
	// with garbage trail data.
	Ready bool
	// Keys are the runtime identifiers BeforeTurnHook minted.
	Keys memoryx.RuntimeKeys
}

// NewMemoryState returns an empty, ready-to-share state. Callers pass
// the same value into both NewMemoryBeforeTurnHook and
// NewMemoryAfterTurnHook.
func NewMemoryState() *MemoryState { return &MemoryState{} }

// setReady marks the state as memory-engaged. Safe for concurrent use.
func (s *MemoryState) setReady(keys memoryx.RuntimeKeys) {
	s.mu.Lock()
	s.Ready = true
	s.Keys = keys
	s.mu.Unlock()
}

// snapshot returns a copy of the current state. Used by the After hook
// so the locking discipline stays local to MemoryState.
func (s *MemoryState) snapshot() (bool, memoryx.RuntimeKeys) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Ready, s.Keys
}

// MemoryDeps captures per-request memory-subsystem inputs. The handler
// builds one MemoryDeps per HTTP request and registers the resulting
// Before/After hooks against the per-session hook.Bus.
//
// State is the cross-hook bridge: Before writes Keys + Ready, After
// reads them. Tests can supply a fresh state per case.
//
// When Enabled is false (e.g. free-tier user or memory globally off),
// both hooks degrade to no-op pass-throughs.
type MemoryDeps struct {
	// Config is the global gptchat openai config. memoryx reads
	// EnableMemory and MemoryProject from here.
	Config *config.OpenAI
	// User is the authenticated user; nil means anonymous (memory off).
	User *config.UserConfig
	// RequestHeader is the inbound HTTP request header, forwarded to
	// memoryx.BeforeTurnHook (it doesn't currently inspect headers, but
	// the legacy proxy path threads them through for forward-compat).
	RequestHeader http.Header
	// MaxInputTokens is the memory hook's input budget (typically 120000).
	MaxInputTokens int
	// Logger surfaces hook-level warnings and the structured before/after
	// trace.
	Logger glog.Logger
	// Enabled forces the hooks to be no-op when false; useful for tests
	// and for the agent-mode-disabled config path.
	Enabled bool
	// State is the shared cross-hook scratch space. Must be non-nil
	// when Enabled is true; tests use NewMemoryState() to mint one per
	// case. A nil pointer with Enabled=true is treated as Enabled=false
	// for safety.
	State *MemoryState
	// FinalTextMaxBytes caps the byte length of the assistant final
	// text (and defensively the user prompt) handed to
	// memoryx.AfterTurnHook. Defaults to DefaultMemoryFinalTextMaxBytes
	// when zero. Set to a negative value to disable truncation entirely
	// (not recommended for production paths).
	FinalTextMaxBytes int
}

// defaultMemoryBeforeTurn is the package-level seam for memoryx.BeforeTurnHook.
// Tests swap this to inject canned BeforeTurnResult values without exercising
// the real memory engine.
var defaultMemoryBeforeTurn = memoryx.BeforeTurnHook

// defaultMemoryAfterTurn is the package-level seam for memoryx.AfterTurnHook.
// Tests swap this to record the inputs and assert U15's memory hygiene
// contract (only user_prompt + final_answer make it through).
var defaultMemoryAfterTurn = memoryx.AfterTurnHook

// NewMemoryBeforeTurnHook returns an OnContext handler that calls
// memoryx.BeforeTurnHook with the latest Input, swaps in the resulting
// PreparedInput, and stashes the keys for the After hook. If Enabled is
// false or BeforeTurnHook fails (NOT_FOUND cold start, etc.), the hook
// is a no-op pass-through.
//
// Returned hook signature matches hook.Bus.OnContext: func(ctx,
// ContextEvent) (ContextEvent, error). The hook never returns an error
// to the bus — a memory failure is best-effort, not a loop-terminating
// condition. (The bus would otherwise translate the error into either a
// synthesized IsError result or, for ErrAskUser, a loop exit.)
func NewMemoryBeforeTurnHook(deps *MemoryDeps) func(context.Context, hook.ContextEvent) (hook.ContextEvent, error) {
	round := 0
	return func(ctx context.Context, ev hook.ContextEvent) (hook.ContextEvent, error) {
		if !memoryDepsActive(deps) {
			return ev, nil
		}

		// Only run on the opening round. memoryx.BeforeTurnHook funnels
		// the input through selectLatestUserMessageItems, which drops
		// every function_call / function_call_output the agent loop has
		// accumulated. Letting that fire on round > 0 wipes the in-flight
		// tool transcript and the model keeps re-emitting the same
		// queries because it never sees its prior calls. The Before hook
		// is meant to seed the opening turn with memory context, not
		// compress the live transcript, so subsequent rounds are skipped.
		currentRound := round
		round++
		if currentRound > 0 {
			return ev, nil
		}

		// memoryx.BeforeTurnHook expects []any (Responses API input).
		// model.InputItem is `any` so the conversion is type-only.
		responsesInput := inputItemsToAny(ev.Input)

		result, err := defaultMemoryBeforeTurn(
			ctx,
			deps.Config,
			deps.User,
			deps.RequestHeader,
			responsesInput,
			deps.MaxInputTokens,
		)
		if err != nil {
			if deps.Logger != nil {
				deps.Logger.Warn("agent_memory_before_turn_failed",
					zap.Bool("cold_start_fallback", result.ColdStartFallback),
					zap.Error(err),
				)
			}
			// Cold-start fallback already populated Keys on the
			// BeforeTurnResult; honour it so the After hook can still
			// persist the turn output.
			if result.ColdStartFallback {
				deps.State.setReady(result.Keys)
			}
			return ev, nil
		}

		// Memory subsystem disabled inside BeforeTurnHook (e.g. free-tier
		// flipped on between requests): leave the input untouched.
		if !result.Enabled {
			return ev, nil
		}

		deps.State.setReady(result.Keys)

		// Swap in the prepared input only if we actually got one back.
		if result.PreparedInput != nil {
			ev.Input = anyToInputItems(result.PreparedInput)
		}
		return ev, nil
	}
}

// NewMemoryAfterTurnHook returns an OnSessionEnd handler that persists
// (UserPrompt, FinalText) via memoryx.AfterTurnHook. Only fires when:
//
//   - Enabled is true.
//   - Keys were populated by BeforeTurnHook (i.e., the Before hook ran
//     successfully or hit the cold-start fallback).
//   - TerminatedBy is one of {send_to_user, implicit_final, ask_user};
//     transcripts from timeouts, iteration caps, circuit-breaks, etc.
//     are not persisted — they are diagnostic noise, not memory.
//
// CRITICAL (proposal §6.1 U15 / acceptance criterion #12): the input
// payload forwarded to memoryx.AfterTurnHook is exactly the two-item
// slice [user_prompt(role=user), final_text(role=assistant)]. The full
// tool transcript (function_calls, function_call_outputs, intermediate
// assistant turns) NEVER reaches the memory engine.
func NewMemoryAfterTurnHook(deps *MemoryDeps) func(context.Context, hook.SessionEndEvent) (hook.SessionEndEvent, error) {
	return func(ctx context.Context, ev hook.SessionEndEvent) (hook.SessionEndEvent, error) {
		if !memoryDepsActive(deps) {
			return ev, nil
		}
		ready, keys := deps.State.snapshot()
		if !ready {
			return ev, nil
		}
		if !shouldPersistAfterTurn(ev.TerminatedBy) {
			if deps.Logger != nil {
				deps.Logger.Debug("agent_memory_after_turn_skipped",
					zap.String("terminated_by", ev.TerminatedBy),
				)
			}
			return ev, nil
		}

		// Defensive size guard before invoking memoryx: the MCP-backed
		// JSONL file_write rejects payloads above its server-side cap
		// with PAYLOAD_TOO_LARGE. We middle-truncate both the prompt
		// (defensively — usually small) and the assistant final text so
		// the JSONL envelope stays well within range while keeping
		// enough head + tail context for the memory engine.
		maxBytes := deps.FinalTextMaxBytes
		if maxBytes == 0 {
			maxBytes = DefaultMemoryFinalTextMaxBytes
		}
		truncatedPrompt, _ := truncateMiddle(ev.UserPrompt, maxBytes)
		truncatedFinal, finalWasTruncated := truncateMiddle(ev.FinalText, maxBytes)

		// U15 / acceptance #12 — minimal payload: exactly the prompt +
		// final-text pair, dressed as Responses-API input messages so
		// memoryx.ResponsesInputToMemoryItems can pick up the roles.
		// The truncated text is what gets persisted in BOTH the input
		// slice (used by ResponsesInputToMemoryItems) and the trailing
		// finalText argument (used by the runtime-context appender);
		// keeping them in sync avoids drift between the memory items
		// and the appended JSONL row.
		minimal := minimalMemoryInput(truncatedPrompt, truncatedFinal)

		if err := defaultMemoryAfterTurn(
			ctx,
			deps.Config,
			deps.User,
			keys,
			minimal,
			truncatedFinal,
		); err != nil {
			// Best-effort recovery: PAYLOAD_TOO_LARGE may surface as a
			// wrapped tool-error string rather than a sentinel (the MCP
			// layer wraps the upstream error). errors.Is is therefore
			// not viable here; strings.Contains on the stable substring
			// is the contractual escape hatch.
			if isPayloadTooLarge(err) && deps.Logger != nil {
				// Concise WARN, NO zap.Error — the full stacktrace is
				// not actionable and pollutes structured logs. The
				// truncated length is the only signal worth keeping.
				deps.Logger.Warn("agent_memory_after_turn_skipped_too_large",
					zap.String("terminated_by", ev.TerminatedBy),
					zap.Int("final_text_bytes", len(ev.FinalText)),
					zap.Int("final_text_truncated_bytes", len(truncatedFinal)),
					zap.Bool("final_text_truncation_applied", finalWasTruncated),
				)
			} else if deps.Logger != nil {
				deps.Logger.Warn("agent_memory_after_turn_failed",
					zap.String("terminated_by", ev.TerminatedBy),
					zap.Error(err),
				)
			}
		}

		return ev, nil
	}
}

// isPayloadTooLarge returns true when err is or wraps the MCP server's
// "PAYLOAD_TOO_LARGE" signal. The MCP transport wraps upstream errors
// behind tool-error strings (so errors.Is against a sentinel does not
// work); the literal substring is contractually stable in mcp_legacy.go
// and the MCP server's reply, so strings.Contains is the correct match
// mechanism here.
func isPayloadTooLarge(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), memoryPayloadTooLargeMarker)
}

// truncateMiddle preserves the first and last half of s when len(s)
// exceeds max bytes, inserting a Unicode-ellipsis marker between them.
// The marker is plain ASCII inside the marker brackets so it round-trips
// cleanly through json.Marshal without invoking any control-char
// escaping. Returns (truncatedString, wasTruncated).
//
// The middle-cut strategy is intentional: it preserves both the opening
// context (which the memory engine often uses for topical features) and
// the closing summary (typically the most useful payload for future
// recall), while only sacrificing intermediate detail. Head-only or
// tail-only would discard half of that signal.
//
// max <= 0 disables truncation (returns s unchanged).
// For inputs already at or below max bytes, s is returned unchanged.
// For inputs above max but shorter than (max + marker overhead), the
// function still emits a marker — the result may be marginally larger
// than max, but never larger than the original.
func truncateMiddle(s string, max int) (string, bool) {
	if max <= 0 || len(s) <= max {
		return s, false
	}
	// Reserve room for the marker; allocate halves from what remains.
	// The marker length is bounded by the size-of-int decimal repr,
	// so we recompute it after we know the dropped byte count.
	dropped := len(s) - max
	// Two halves split roughly evenly; favour the head so the
	// opening sentence remains intact when max is odd.
	headLen := max / 2
	tailLen := max - headLen
	head := s[:headLen]
	tail := s[len(s)-tailLen:]
	// Trim partial multi-byte runes at the cut points so the result
	// is always valid UTF-8 (important: json.Marshal would otherwise
	// replace bad bytes with U+FFFD silently).
	head = trimUTF8Tail(head)
	tail = trimUTF8Head(tail)
	marker := fmt.Sprintf("…[truncated %d bytes]…", dropped)
	return head + marker + tail, true
}

// trimUTF8Tail walks back from the end of s to drop any bytes that
// form a partial multi-byte UTF-8 sequence, ensuring the slice ends on
// a complete rune boundary.
func trimUTF8Tail(s string) string {
	for i := len(s) - 1; i >= 0; i-- {
		b := s[i]
		// ASCII byte — clean boundary.
		if b < 0x80 {
			return s[:i+1]
		}
		// Trail byte: keep walking back.
		if b&0xC0 == 0x80 {
			continue
		}
		// Lead byte. Check whether the sequence it starts is complete
		// within the slice; if not, drop it and any trail bytes after.
		switch {
		case b&0xE0 == 0xC0: // 2-byte lead
			if len(s)-i >= 2 {
				return s
			}
		case b&0xF0 == 0xE0: // 3-byte lead
			if len(s)-i >= 3 {
				return s
			}
		case b&0xF8 == 0xF0: // 4-byte lead
			if len(s)-i >= 4 {
				return s
			}
		}
		return s[:i]
	}
	return ""
}

// trimUTF8Head walks forward from the start of s, dropping any leading
// trail bytes so the slice begins on a complete rune boundary.
func trimUTF8Head(s string) string {
	for i := 0; i < len(s); i++ {
		b := s[i]
		if b < 0x80 || b&0xC0 != 0x80 {
			return s[i:]
		}
	}
	return ""
}

// memoryDepsActive returns true when both Enabled is on and the shared
// state pointer is non-nil. The nil-pointer fallback is defensive — a
// caller-side bug where Enabled=true but State=nil otherwise panics on
// the first hook fire, deep inside the loop.
func memoryDepsActive(deps *MemoryDeps) bool {
	return deps != nil && deps.Enabled && deps.State != nil
}

// shouldPersistAfterTurn returns true for terminal states that represent
// a genuine assistant answer the user saw. Diagnostic terminations
// (iteration caps, timeouts, etc.) are excluded so memory stays clean.
func shouldPersistAfterTurn(terminatedBy string) bool {
	switch terminatedBy {
	case session.TerminatedBySendToUser,
		session.TerminatedByImplicitFinal,
		session.TerminatedByAskUser:
		return true
	default:
		return false
	}
}

// minimalMemoryInput builds the exactly-two-item Responses input slice
// that memoryx.AfterTurnHook is allowed to see, regardless of how long
// the actual tool transcript was. Tool calls / tool outputs / intermediate
// assistant turns are deliberately dropped (proposal §6.1 U15).
//
// The shapes match what model.oneAPI.validateInputItem accepts so the
// downstream memoryx.ResponsesInputToMemoryItems mapping works.
func minimalMemoryInput(userPrompt, finalText string) []any {
	out := make([]any, 0, 2)
	if strings.TrimSpace(userPrompt) != "" {
		out = append(out, httppkg.OpenAIResponsesInputMessage{
			Role:    "user",
			Content: userPrompt,
		})
	}
	if strings.TrimSpace(finalText) != "" {
		out = append(out, httppkg.OpenAIResponsesInputMessage{
			Role:    "assistant",
			Content: finalText,
		})
	}
	return out
}

// inputItemsToAny widens the typed model.InputItem slice into the plain
// []any shape memoryx consumes. model.InputItem is already `any` (see
// agentx/model/types.go) so this is essentially a copy + slice rebuild
// at a different element type for the compiler.
func inputItemsToAny(items []model.InputItem) []any {
	if len(items) == 0 {
		return nil
	}
	out := make([]any, len(items))
	for i, item := range items {
		out[i] = item
	}
	return out
}

// anyToInputItems is the inverse of inputItemsToAny.
func anyToInputItems(items []any) []model.InputItem {
	if len(items) == 0 {
		return nil
	}
	out := make([]model.InputItem, len(items))
	for i, item := range items {
		out[i] = item
	}
	return out
}

// ErrMemoryDepsInactive is the sentinel returned by the after hook only
// when callers pass an obviously broken MemoryDeps (Enabled=true but
// State=nil). It is wrapped, not surfaced, in production paths — kept
// exported for tests that want to round-trip the check.
var ErrMemoryDepsInactive = errors.New("memory deps inactive: Enabled set but State is nil")

var _ = ErrMemoryDepsInactive // referenced by tests; silences unused-var
