package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/Laisky/errors/v2"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/hook"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/model"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/session"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
	httppkg "github.com/Laisky/go-ramjet/internal/tasks/gptchat/http"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/memoryx"
)

// installFakeMemoryBefore swaps the package-level BeforeTurn hook with
// fn and returns a cleanup function.
func installFakeMemoryBefore(t *testing.T, fn func(ctx context.Context, conf *config.OpenAI, user *config.UserConfig, h http.Header, input []any, maxTok int) (memoryx.BeforeTurnResult, error)) func() {
	t.Helper()
	orig := defaultMemoryBeforeTurn
	defaultMemoryBeforeTurn = fn
	return func() { defaultMemoryBeforeTurn = orig }
}

// installFakeMemoryAfter swaps the package-level AfterTurn hook with fn
// and returns a cleanup function.
func installFakeMemoryAfter(t *testing.T, fn func(ctx context.Context, conf *config.OpenAI, user *config.UserConfig, keys memoryx.RuntimeKeys, input []any, finalText string) error) func() {
	t.Helper()
	orig := defaultMemoryAfterTurn
	defaultMemoryAfterTurn = fn
	return func() { defaultMemoryAfterTurn = orig }
}

// fakeBeforeResult builds a memoryx.BeforeTurnResult populated with the
// canonical keys used across tests.
func fakeBeforeResult(prepared []any) memoryx.BeforeTurnResult {
	return memoryx.BeforeTurnResult{
		Enabled: true,
		Keys: memoryx.RuntimeKeys{
			Project:   "ramjet",
			SessionID: "s1",
			UserID:    "u1",
			TurnID:    "t1",
		},
		PreparedInput: prepared,
	}
}

// U14 — Memory disabled: when MemoryDeps.Enabled=false the Before/After
// hooks return their input event unchanged with nil error and never call
// into memoryx.
func TestMemoryHooks_U14_DisabledIsNoop(t *testing.T) {
	// Cannot t.Parallel: mutates package-level seams.
	calledBefore := false
	calledAfter := false
	restoreBefore := installFakeMemoryBefore(t, func(_ context.Context, _ *config.OpenAI, _ *config.UserConfig, _ http.Header, _ []any, _ int) (memoryx.BeforeTurnResult, error) {
		calledBefore = true
		return memoryx.BeforeTurnResult{}, nil
	})
	defer restoreBefore()
	restoreAfter := installFakeMemoryAfter(t, func(_ context.Context, _ *config.OpenAI, _ *config.UserConfig, _ memoryx.RuntimeKeys, _ []any, _ string) error {
		calledAfter = true
		return nil
	})
	defer restoreAfter()

	deps := &MemoryDeps{
		Enabled: false,
		State:   NewMemoryState(),
	}
	before := NewMemoryBeforeTurnHook(deps)
	after := NewMemoryAfterTurnHook(deps)

	in := hook.ContextEvent{Input: []model.InputItem{httppkg.OpenAIResponsesInputMessage{Role: "user", Content: "hi"}}}
	out, err := before(context.Background(), in)
	require.NoError(t, err)
	require.Equal(t, in, out, "disabled Before hook must pass the event through unchanged")
	require.False(t, calledBefore, "memoryx.BeforeTurnHook must NOT be called when Enabled=false")

	endIn := hook.SessionEndEvent{
		SessionID:    "sess",
		TerminatedBy: session.TerminatedBySendToUser,
		FinalText:    "the answer is 42",
		UserPrompt:   "what is the answer",
	}
	endOut, err := after(context.Background(), endIn)
	require.NoError(t, err)
	require.Equal(t, endIn, endOut)
	require.False(t, calledAfter, "memoryx.AfterTurnHook must NOT be called when Enabled=false")
}

// U14 (auxiliary) — when State pointer is nil the hooks must also be
// no-ops, defensively.
func TestMemoryHooks_U14_NilStateIsNoop(t *testing.T) {
	calledBefore := false
	calledAfter := false
	restoreBefore := installFakeMemoryBefore(t, func(_ context.Context, _ *config.OpenAI, _ *config.UserConfig, _ http.Header, _ []any, _ int) (memoryx.BeforeTurnResult, error) {
		calledBefore = true
		return memoryx.BeforeTurnResult{}, nil
	})
	defer restoreBefore()
	restoreAfter := installFakeMemoryAfter(t, func(_ context.Context, _ *config.OpenAI, _ *config.UserConfig, _ memoryx.RuntimeKeys, _ []any, _ string) error {
		calledAfter = true
		return nil
	})
	defer restoreAfter()

	deps := &MemoryDeps{Enabled: true, State: nil} // intentionally broken
	before := NewMemoryBeforeTurnHook(deps)
	after := NewMemoryAfterTurnHook(deps)
	_, err := before(context.Background(), hook.ContextEvent{})
	require.NoError(t, err)
	require.False(t, calledBefore)
	_, err = after(context.Background(), hook.SessionEndEvent{TerminatedBy: session.TerminatedBySendToUser})
	require.NoError(t, err)
	require.False(t, calledAfter)
}

func TestMemoryHooks_BeforeSwapsInputAndStashesKeys(t *testing.T) {
	prepared := []any{
		httppkg.OpenAIResponsesInputMessage{Role: "system", Content: "remembered:..."},
		httppkg.OpenAIResponsesInputMessage{Role: "user", Content: "hi"},
	}
	restoreBefore := installFakeMemoryBefore(t, func(_ context.Context, _ *config.OpenAI, _ *config.UserConfig, _ http.Header, _ []any, _ int) (memoryx.BeforeTurnResult, error) {
		return fakeBeforeResult(prepared), nil
	})
	defer restoreBefore()

	state := NewMemoryState()
	deps := &MemoryDeps{
		Enabled:        true,
		Config:         &config.OpenAI{EnableMemory: true},
		User:           &config.UserConfig{UserName: "u1"},
		MaxInputTokens: 1000,
		State:          state,
	}
	before := NewMemoryBeforeTurnHook(deps)
	out, err := before(context.Background(), hook.ContextEvent{
		Input: []model.InputItem{httppkg.OpenAIResponsesInputMessage{Role: "user", Content: "hi"}},
	})
	require.NoError(t, err)
	require.Len(t, out.Input, 2, "Before hook must swap in PreparedInput")

	ready, keys := state.snapshot()
	require.True(t, ready)
	require.Equal(t, "ramjet", keys.Project)
	require.Equal(t, "u1", keys.UserID)
	require.Equal(t, "t1", keys.TurnID)
}

func TestMemoryHooks_BeforeFailureIsPassThrough(t *testing.T) {
	restoreBefore := installFakeMemoryBefore(t, func(_ context.Context, _ *config.OpenAI, _ *config.UserConfig, _ http.Header, _ []any, _ int) (memoryx.BeforeTurnResult, error) {
		return memoryx.BeforeTurnResult{Enabled: true}, errors.New("boom")
	})
	defer restoreBefore()

	state := NewMemoryState()
	deps := &MemoryDeps{Enabled: true, State: state}
	before := NewMemoryBeforeTurnHook(deps)
	in := hook.ContextEvent{Input: []model.InputItem{httppkg.OpenAIResponsesInputMessage{Role: "user", Content: "x"}}}
	out, err := before(context.Background(), in)
	require.NoError(t, err, "memory failure must NOT escape to the bus")
	require.Equal(t, in, out)

	ready, _ := state.snapshot()
	require.False(t, ready, "failed Before must leave state un-ready so After is skipped")
}

func TestMemoryHooks_BeforeColdStartFallbackMarksReady(t *testing.T) {
	restoreBefore := installFakeMemoryBefore(t, func(_ context.Context, _ *config.OpenAI, _ *config.UserConfig, _ http.Header, _ []any, _ int) (memoryx.BeforeTurnResult, error) {
		// Cold-start: BeforeTurnHook returns (result with ColdStartFallback=true, nil error) in the real implementation,
		// but defensive: also accept (result, err). Our hook honours both.
		return memoryx.BeforeTurnResult{
			Enabled:           true,
			ColdStartFallback: true,
			Keys: memoryx.RuntimeKeys{
				Project: "ramjet", SessionID: "s1", UserID: "u1", TurnID: "t1",
			},
		}, errors.New("not_found")
	})
	defer restoreBefore()

	state := NewMemoryState()
	deps := &MemoryDeps{Enabled: true, State: state}
	before := NewMemoryBeforeTurnHook(deps)
	_, err := before(context.Background(), hook.ContextEvent{})
	require.NoError(t, err)
	ready, keys := state.snapshot()
	require.True(t, ready, "cold-start fallback must still mark state ready")
	require.Equal(t, "t1", keys.TurnID)
}

// recordedAfter captures the args passed to memoryx.AfterTurnHook.
type recordedAfter struct {
	called    int
	keys      memoryx.RuntimeKeys
	input     []any
	finalText string
}

func (r *recordedAfter) record(_ context.Context, _ *config.OpenAI, _ *config.UserConfig, k memoryx.RuntimeKeys, in []any, ft string) error {
	r.called++
	r.keys = k
	r.input = in
	r.finalText = ft
	return nil
}

// U15 — Memory hygiene end-to-end: feed a 10-message transcript with 4
// tool calls into the loop, terminate cleanly, verify the AfterTurnHook
// fixture receives exactly 2 items.
func TestMemoryHooks_U15_AfterReceivesOnlyPromptAndFinal(t *testing.T) {
	// Build the "10-message transcript with 4 tool calls" the proposal
	// describes. This is the *input* state at session end — the loop
	// driver does not feed it into the After hook directly, but if the
	// After hook were to inspect ev.Input (it doesn't, only ev.UserPrompt
	// and ev.FinalText), it would see this. The test asserts that the
	// After hook nevertheless calls memoryx.AfterTurnHook with the
	// minimal 2-item payload.
	transcript := []model.InputItem{
		httppkg.OpenAIResponsesInputMessage{Role: "user", Content: "what's the latest claude blog post?"},
		httppkg.OpenAIResponsesFunctionCall{Type: "function_call", CallID: "c1", Name: "web_search", Arguments: `{"q":"claude blog"}`},
		httppkg.OpenAIResponsesFunctionCallOutput{Type: "function_call_output", CallID: "c1", Output: `{"hits":3}`},
		httppkg.OpenAIResponsesFunctionCall{Type: "function_call", CallID: "c2", Name: "web_fetch", Arguments: `{"url":"https://anthropic.com"}`},
		httppkg.OpenAIResponsesFunctionCallOutput{Type: "function_call_output", CallID: "c2", Output: `<html>...</html>`},
		httppkg.OpenAIResponsesInputMessage{Role: "assistant", Content: "let me search more"},
		httppkg.OpenAIResponsesFunctionCall{Type: "function_call", CallID: "c3", Name: "file_search", Arguments: `{"q":"summary"}`},
		httppkg.OpenAIResponsesFunctionCallOutput{Type: "function_call_output", CallID: "c3", Output: `[]`},
		httppkg.OpenAIResponsesFunctionCall{Type: "function_call", CallID: "c4", Name: "web_fetch", Arguments: `{"url":"https://x"}`},
		httppkg.OpenAIResponsesFunctionCallOutput{Type: "function_call_output", CallID: "c4", Output: `{"ok":true}`},
	}
	require.Len(t, transcript, 10, "fixture must be exactly 10 items")
	functionCallCount := 0
	for _, item := range transcript {
		if _, ok := item.(httppkg.OpenAIResponsesFunctionCall); ok {
			functionCallCount++
		}
	}
	require.Equal(t, 4, functionCallCount, "fixture must have exactly 4 tool calls")

	// Run the Before hook so state.Ready=true and the After hook will
	// actually call memoryx.AfterTurnHook.
	restoreBefore := installFakeMemoryBefore(t, func(_ context.Context, _ *config.OpenAI, _ *config.UserConfig, _ http.Header, _ []any, _ int) (memoryx.BeforeTurnResult, error) {
		return fakeBeforeResult(inputItemsToAny(transcript)), nil
	})
	defer restoreBefore()

	rec := &recordedAfter{}
	restoreAfter := installFakeMemoryAfter(t, rec.record)
	defer restoreAfter()

	state := NewMemoryState()
	deps := &MemoryDeps{
		Enabled:        true,
		Config:         &config.OpenAI{EnableMemory: true},
		User:           &config.UserConfig{UserName: "u1"},
		MaxInputTokens: 1000,
		State:          state,
	}

	// Drive Before → After through a bus, mimicking the real loop.
	bus := hook.NewBus(nil)
	bus.OnContext(NewMemoryBeforeTurnHook(deps))
	bus.OnSessionEnd(NewMemoryAfterTurnHook(deps))

	_, err := bus.DispatchContext(context.Background(), hook.ContextEvent{Input: transcript})
	require.NoError(t, err)

	_, err = bus.DispatchSessionEnd(context.Background(), hook.SessionEndEvent{
		SessionID:    "sess",
		TerminatedBy: session.TerminatedBySendToUser,
		FinalText:    "Anthropic published a Claude 4.6 post on 2025-...",
		UserPrompt:   "what's the latest claude blog post?",
	})
	require.NoError(t, err)

	// THE acceptance test: memoryx.AfterTurnHook saw exactly two items.
	require.Equal(t, 1, rec.called, "AfterTurnHook must fire exactly once")
	require.Len(t, rec.input, 2, "AfterTurnHook MUST receive only [user_prompt, final_answer]; the tool transcript MUST NOT leak")

	user, ok := rec.input[0].(httppkg.OpenAIResponsesInputMessage)
	require.True(t, ok, "first item must be an input message")
	require.Equal(t, "user", user.Role)
	require.Equal(t, "what's the latest claude blog post?", user.Content)

	assistant, ok := rec.input[1].(httppkg.OpenAIResponsesInputMessage)
	require.True(t, ok, "second item must be an input message")
	require.Equal(t, "assistant", assistant.Role)
	require.Equal(t, "Anthropic published a Claude 4.6 post on 2025-...", assistant.Content)

	// No function_call items leaked.
	for i, it := range rec.input {
		_, isFC := it.(httppkg.OpenAIResponsesFunctionCall)
		require.Falsef(t, isFC, "rec.input[%d] is a function_call — tool transcript LEAKED", i)
		_, isFCO := it.(httppkg.OpenAIResponsesFunctionCallOutput)
		require.Falsef(t, isFCO, "rec.input[%d] is a function_call_output — tool transcript LEAKED", i)
	}
}

// AfterTurn must be a no-op when terminated_by indicates a diagnostic
// state (iteration_cap, timeout, etc.). Memory should not collect noise.
func TestMemoryHooks_AfterSkipsOnDiagnosticTermination(t *testing.T) {
	restoreBefore := installFakeMemoryBefore(t, func(_ context.Context, _ *config.OpenAI, _ *config.UserConfig, _ http.Header, _ []any, _ int) (memoryx.BeforeTurnResult, error) {
		return fakeBeforeResult(nil), nil
	})
	defer restoreBefore()
	rec := &recordedAfter{}
	restoreAfter := installFakeMemoryAfter(t, rec.record)
	defer restoreAfter()

	state := NewMemoryState()
	deps := &MemoryDeps{
		Enabled:        true,
		Config:         &config.OpenAI{EnableMemory: true},
		User:           &config.UserConfig{UserName: "u1"},
		MaxInputTokens: 1000,
		State:          state,
	}
	bus := hook.NewBus(nil)
	bus.OnContext(NewMemoryBeforeTurnHook(deps))
	bus.OnSessionEnd(NewMemoryAfterTurnHook(deps))
	_, err := bus.DispatchContext(context.Background(), hook.ContextEvent{})
	require.NoError(t, err)

	for _, tb := range []string{
		session.TerminatedByIterationCap,
		session.TerminatedByTimeout,
		session.TerminatedByCircuitBreaker,
		session.TerminatedByErrorBudget,
		session.TerminatedByCancelled,
		session.TerminatedByError,
	} {
		_, err = bus.DispatchSessionEnd(context.Background(), hook.SessionEndEvent{
			SessionID:    "sess",
			TerminatedBy: tb,
			UserPrompt:   "p",
			FinalText:    "f",
		})
		require.NoError(t, err)
	}
	require.Equal(t, 0, rec.called, "AfterTurnHook must NOT fire on diagnostic terminations")

	// Sanity: it DOES fire on the three "real" terminations.
	for _, tb := range []string{
		session.TerminatedBySendToUser,
		session.TerminatedByImplicitFinal,
		session.TerminatedByAskUser,
	} {
		rec.called = 0
		_, err = bus.DispatchSessionEnd(context.Background(), hook.SessionEndEvent{
			SessionID:    "sess",
			TerminatedBy: tb,
			UserPrompt:   "p",
			FinalText:    "f",
		})
		require.NoError(t, err)
		require.Equal(t, 1, rec.called, "AfterTurnHook must fire on %s", tb)
	}
}

// When the Before hook did not run successfully (state.Ready=false), the
// After hook must NOT call memoryx.AfterTurnHook even on a clean
// termination. This guards against the case where memory was meant to be
// engaged but couldn't initialise.
func TestMemoryHooks_AfterRequiresReadyState(t *testing.T) {
	rec := &recordedAfter{}
	restoreAfter := installFakeMemoryAfter(t, rec.record)
	defer restoreAfter()

	state := NewMemoryState()
	require.False(t, state.Ready)

	deps := &MemoryDeps{
		Enabled: true,
		Config:  &config.OpenAI{EnableMemory: true},
		User:    &config.UserConfig{UserName: "u1"},
		State:   state,
	}
	after := NewMemoryAfterTurnHook(deps)
	_, err := after(context.Background(), hook.SessionEndEvent{
		SessionID:    "sess",
		TerminatedBy: session.TerminatedBySendToUser,
		UserPrompt:   "p",
		FinalText:    "f",
	})
	require.NoError(t, err)
	require.Equal(t, 0, rec.called)
}

// AfterTurn must trim whitespace-only prompts / finals out of the
// minimal payload (so we don't store empty turns).
func TestMemoryHooks_AfterDropsEmptyParts(t *testing.T) {
	got := minimalMemoryInput("", "  \n\t  ")
	require.Empty(t, got)

	got = minimalMemoryInput("prompt", "")
	require.Len(t, got, 1)
	require.Equal(t, "user", got[0].(httppkg.OpenAIResponsesInputMessage).Role)

	got = minimalMemoryInput("", "answer")
	require.Len(t, got, 1)
	require.Equal(t, "assistant", got[0].(httppkg.OpenAIResponsesInputMessage).Role)

	got = minimalMemoryInput("prompt", "answer")
	require.Len(t, got, 2)
}

// inputItemsToAny / anyToInputItems should round-trip without loss.
func TestMemoryHooks_InputItemConversionRoundTrip(t *testing.T) {
	t.Parallel()
	in := []model.InputItem{
		httppkg.OpenAIResponsesInputMessage{Role: "user", Content: "hi"},
		httppkg.OpenAIResponsesFunctionCall{Type: "function_call", CallID: "c1", Name: "web_search"},
	}
	wide := inputItemsToAny(in)
	require.Len(t, wide, 2)
	back := anyToInputItems(wide)
	require.Equal(t, in, back)

	require.Nil(t, inputItemsToAny(nil))
	require.Nil(t, anyToInputItems(nil))
}

// Truncation happy path: a 200 KB final text must be truncated below the
// 64 KiB cap, must contain the `[truncated <N> bytes]` marker, and the
// AfterTurnHook fake must observe the truncated payload (not the raw
// 200 KB).
func TestMemoryHooks_AfterTruncatesOversizedFinalText(t *testing.T) {
	restoreBefore := installFakeMemoryBefore(t, func(_ context.Context, _ *config.OpenAI, _ *config.UserConfig, _ http.Header, _ []any, _ int) (memoryx.BeforeTurnResult, error) {
		return fakeBeforeResult(nil), nil
	})
	defer restoreBefore()

	rec := &recordedAfter{}
	restoreAfter := installFakeMemoryAfter(t, rec.record)
	defer restoreAfter()

	state := NewMemoryState()
	deps := &MemoryDeps{
		Enabled: true,
		Config:  &config.OpenAI{EnableMemory: true},
		User:    &config.UserConfig{UserName: "u1"},
		State:   state,
		// Leave FinalTextMaxBytes=0 so the default kicks in. Explicit
		// reliance on the package default is part of the contract.
	}
	bus := hook.NewBus(nil)
	bus.OnContext(NewMemoryBeforeTurnHook(deps))
	bus.OnSessionEnd(NewMemoryAfterTurnHook(deps))
	_, err := bus.DispatchContext(context.Background(), hook.ContextEvent{})
	require.NoError(t, err)

	const oversize = 200 * 1024 // 200 KB
	bigFinal := strings.Repeat("X", oversize)
	_, err = bus.DispatchSessionEnd(context.Background(), hook.SessionEndEvent{
		SessionID:    "sess",
		TerminatedBy: session.TerminatedBySendToUser,
		UserPrompt:   "summarise the doc",
		FinalText:    bigFinal,
	})
	require.NoError(t, err)
	require.Equal(t, 1, rec.called)
	require.Equal(t, 2, len(rec.input))

	assistantMsg, ok := rec.input[1].(httppkg.OpenAIResponsesInputMessage)
	require.True(t, ok)
	assistantContent, ok := assistantMsg.Content.(string)
	require.True(t, ok, "assistant Content must be a string after truncation")
	require.Less(t, len(assistantContent), DefaultMemoryFinalTextMaxBytes+512,
		"truncated final text must fit within the default cap plus a small marker overhead")
	require.Contains(t, assistantContent, "[truncated ")
	require.Contains(t, assistantContent, " bytes]")
	require.True(t, strings.HasPrefix(assistantContent, "X"),
		"head fragment must be preserved")
	require.True(t, strings.HasSuffix(assistantContent, "X"),
		"tail fragment must be preserved")

	// The finalText argument forwarded to memoryx must mirror the
	// truncated assistant content so the JSONL runtime-context row
	// matches what ResponsesInputToMemoryItems persisted.
	require.Equal(t, assistantContent, rec.finalText)

	// JSON round-trip: the truncated payload (with the ellipsis marker)
	// must marshal/unmarshal cleanly — the JSONL appender on the other
	// end depends on this.
	encoded, jerr := json.Marshal(assistantContent)
	require.NoError(t, jerr)
	var back string
	require.NoError(t, json.Unmarshal(encoded, &back))
	require.Equal(t, assistantContent, back)
}

// No truncation under threshold: a 1 KB final text must be passed
// through untouched and never carry the marker.
func TestMemoryHooks_AfterDoesNotTruncateSmallPayloads(t *testing.T) {
	restoreBefore := installFakeMemoryBefore(t, func(_ context.Context, _ *config.OpenAI, _ *config.UserConfig, _ http.Header, _ []any, _ int) (memoryx.BeforeTurnResult, error) {
		return fakeBeforeResult(nil), nil
	})
	defer restoreBefore()

	rec := &recordedAfter{}
	restoreAfter := installFakeMemoryAfter(t, rec.record)
	defer restoreAfter()

	state := NewMemoryState()
	deps := &MemoryDeps{
		Enabled: true,
		Config:  &config.OpenAI{EnableMemory: true},
		User:    &config.UserConfig{UserName: "u1"},
		State:   state,
	}
	bus := hook.NewBus(nil)
	bus.OnContext(NewMemoryBeforeTurnHook(deps))
	bus.OnSessionEnd(NewMemoryAfterTurnHook(deps))
	_, err := bus.DispatchContext(context.Background(), hook.ContextEvent{})
	require.NoError(t, err)

	const small = 1024 // 1 KB
	finalText := strings.Repeat("Y", small)
	_, err = bus.DispatchSessionEnd(context.Background(), hook.SessionEndEvent{
		SessionID:    "sess",
		TerminatedBy: session.TerminatedBySendToUser,
		UserPrompt:   "say Y a thousand times",
		FinalText:    finalText,
	})
	require.NoError(t, err)
	require.Equal(t, 1, rec.called)

	assistantMsg, ok := rec.input[1].(httppkg.OpenAIResponsesInputMessage)
	require.True(t, ok)
	require.Equal(t, finalText, assistantMsg.Content, "1 KB final text must pass through untouched")
	require.NotContains(t, assistantMsg.Content, "[truncated ")
	require.Equal(t, finalText, rec.finalText)
}

// PAYLOAD_TOO_LARGE fallback: even after truncation, if the
// AfterTurnHook returns a PAYLOAD_TOO_LARGE-flavoured error the hook
// emits the concise `agent_memory_after_turn_skipped_too_large` WARN
// (no verbose stacktrace) and returns the event unchanged.
func TestMemoryHooks_AfterRecoversFromPayloadTooLarge(t *testing.T) {
	restoreBefore := installFakeMemoryBefore(t, func(_ context.Context, _ *config.OpenAI, _ *config.UserConfig, _ http.Header, _ []any, _ int) (memoryx.BeforeTurnResult, error) {
		return fakeBeforeResult(nil), nil
	})
	defer restoreBefore()

	// Simulate the MCP file_write wrap: an Errors.v2 stack-augmented
	// error whose message contains "PAYLOAD_TOO_LARGE" deep in the
	// wrap chain. The hook must catch the substring even through
	// errors.Wrap layers.
	innerErr := errors.New("PAYLOAD_TOO_LARGE: file exceeds max size")
	wrappedErr := errors.Wrap(errors.Wrap(innerErr, "call file_write"), "append jsonl")

	restoreAfter := installFakeMemoryAfter(t, func(_ context.Context, _ *config.OpenAI, _ *config.UserConfig, _ memoryx.RuntimeKeys, _ []any, _ string) error {
		return wrappedErr
	})
	defer restoreAfter()

	logger, lerr := glog.NewConsoleWithName("test_memhook", glog.LevelError)
	require.NoError(t, lerr)

	state := NewMemoryState()
	deps := &MemoryDeps{
		Enabled: true,
		Config:  &config.OpenAI{EnableMemory: true},
		User:    &config.UserConfig{UserName: "u1"},
		State:   state,
		Logger:  logger,
	}
	bus := hook.NewBus(nil)
	bus.OnContext(NewMemoryBeforeTurnHook(deps))
	bus.OnSessionEnd(NewMemoryAfterTurnHook(deps))
	_, err := bus.DispatchContext(context.Background(), hook.ContextEvent{})
	require.NoError(t, err)

	in := hook.SessionEndEvent{
		SessionID:    "sess",
		TerminatedBy: session.TerminatedBySendToUser,
		UserPrompt:   "x",
		FinalText:    strings.Repeat("Z", 200*1024),
	}
	out, err := bus.DispatchSessionEnd(context.Background(), in)
	// The hook MUST NOT propagate the error — the loop should
	// continue. The PAYLOAD_TOO_LARGE branch is best-effort recovery.
	require.NoError(t, err)
	// The event itself is returned unchanged.
	require.Equal(t, in, out)
}

// isPayloadTooLarge must catch the substring through arbitrary error
// wrapping levels.
func TestMemoryHooks_IsPayloadTooLargeMatchesThroughWraps(t *testing.T) {
	t.Parallel()
	require.False(t, isPayloadTooLarge(nil))
	require.False(t, isPayloadTooLarge(errors.New("some other error")))

	inner := errors.New("PAYLOAD_TOO_LARGE: file exceeds max size")
	require.True(t, isPayloadTooLarge(inner))
	require.True(t, isPayloadTooLarge(errors.Wrap(inner, "outer1")))
	require.True(t, isPayloadTooLarge(errors.Wrap(errors.Wrap(inner, "outer1"), "outer2")))
}

// truncateMiddle preserves head and tail and emits a UTF-8 safe marker.
func TestMemoryHooks_TruncateMiddleSemantics(t *testing.T) {
	t.Parallel()

	// max=0 disables.
	out, trimmed := truncateMiddle("hello", 0)
	require.Equal(t, "hello", out)
	require.False(t, trimmed)

	// Under threshold: untouched.
	out, trimmed = truncateMiddle("hello", 1024)
	require.Equal(t, "hello", out)
	require.False(t, trimmed)

	// Exactly at threshold: untouched.
	out, trimmed = truncateMiddle("hello", 5)
	require.Equal(t, "hello", out)
	require.False(t, trimmed)

	// Over threshold: marker present, head + tail preserved.
	src := strings.Repeat("A", 100) + strings.Repeat("B", 100)
	out, trimmed = truncateMiddle(src, 64)
	require.True(t, trimmed)
	require.Contains(t, out, "[truncated ")
	require.Contains(t, out, " bytes]")
	require.True(t, strings.HasPrefix(out, "A"), "head must be preserved")
	require.True(t, strings.HasSuffix(out, "B"), "tail must be preserved")

	// Round-trip through JSON: the literal Unicode ellipsis must
	// survive marshal/unmarshal without corruption.
	encoded, err := json.Marshal(out)
	require.NoError(t, err)
	var back string
	require.NoError(t, json.Unmarshal(encoded, &back))
	require.Equal(t, out, back)

	// UTF-8 safety: a multi-byte rune straddling the cut point must
	// not produce invalid UTF-8.
	utf8src := strings.Repeat("文", 200) // 600 bytes
	out, trimmed = truncateMiddle(utf8src, 64)
	require.True(t, trimmed)
	require.True(t, json.Valid([]byte(`"`+jsonEscape(out)+`"`)),
		"truncated UTF-8 string must encode as valid JSON")
}

// jsonEscape minimally escapes a string so it can be wrapped in JSON
// quotes for the json.Valid assertion above. Only the double-quote and
// backslash need escaping for this narrow purpose; the truncateMiddle
// output never contains either.
func jsonEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}
