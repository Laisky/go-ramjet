package loop

import (
	"context"
	stdjson "encoding/json"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/hook"
)

// TestCircuitHook_TripsOnThirdRepeat covers proposal §6.1 U5: three identical
// calls trip the breaker on the third invocation.
func TestCircuitHook_TripsOnThirdRepeat(t *testing.T) {
	t.Parallel()
	h := NewCircuitHook(3)

	args := stdjson.RawMessage(`{"query":"hello"}`)
	for i := 0; i < 2; i++ {
		out, err := h(context.Background(), hook.ToolCallEvent{
			ToolName: "web_search",
			CallID:   "c",
			Args:     args,
		})
		require.NoError(t, err)
		require.Nil(t, out.Result, "call %d should pass through", i+1)
	}
	out, err := h(context.Background(), hook.ToolCallEvent{
		ToolName: "web_search",
		CallID:   "c",
		Args:     args,
	})
	require.NoError(t, err)
	require.NotNil(t, out.Result, "third call must be tripped")
	require.True(t, out.Result.IsError)
	require.Contains(t, out.Result.Content, "repeated tool call detected")
}

func TestCircuitHook_DifferentArgsResetStreak(t *testing.T) {
	t.Parallel()
	h := NewCircuitHook(3)
	for i := 0; i < 2; i++ {
		out, err := h(context.Background(), hook.ToolCallEvent{
			ToolName: "web_search",
			Args:     stdjson.RawMessage(`{"query":"alpha"}`),
		})
		require.NoError(t, err)
		require.Nil(t, out.Result)
	}
	out, err := h(context.Background(), hook.ToolCallEvent{
		ToolName: "web_search",
		Args:     stdjson.RawMessage(`{"query":"beta"}`),
	})
	require.NoError(t, err)
	require.Nil(t, out.Result, "different args must reset the streak")
}

func TestCircuitHook_NormalizesKeyOrder(t *testing.T) {
	t.Parallel()
	h := NewCircuitHook(2)
	// Same logical args but different key order should still be detected
	// as a repeat.
	a := stdjson.RawMessage(`{"a":1,"b":2}`)
	b := stdjson.RawMessage(`{"b":2,"a":1}`)

	_, _ = h(context.Background(), hook.ToolCallEvent{ToolName: "x", Args: a})
	out, err := h(context.Background(), hook.ToolCallEvent{ToolName: "x", Args: b})
	require.NoError(t, err)
	require.NotNil(t, out.Result, "key reorder must not defeat repeat detection")
}

func TestCircuitHook_PassThroughAfterTrip(t *testing.T) {
	t.Parallel()
	h := NewCircuitHook(2)
	args := stdjson.RawMessage(`{"q":"x"}`)
	_, _ = h(context.Background(), hook.ToolCallEvent{ToolName: "t", Args: args})
	_, _ = h(context.Background(), hook.ToolCallEvent{ToolName: "t", Args: args})
	// After trip, streak resets — so next call passes through.
	out, err := h(context.Background(), hook.ToolCallEvent{ToolName: "t", Args: args})
	require.NoError(t, err)
	require.Nil(t, out.Result, "post-trip call should pass through (streak reset)")
}

func TestCircuitHook_DisabledWhenRepeatsLessThanOne(t *testing.T) {
	t.Parallel()
	h := NewCircuitHook(0)
	for i := 0; i < 10; i++ {
		out, err := h(context.Background(), hook.ToolCallEvent{
			ToolName: "x",
			Args:     stdjson.RawMessage(`{}`),
		})
		require.NoError(t, err)
		require.Nil(t, out.Result)
	}
}

// TestCircuitHook_RaceUnderDispatch exercises the per-instance mutex from
// many goroutines so -race confirms there's no data race on the streak
// counter.
func TestCircuitHook_RaceUnderDispatch(t *testing.T) {
	t.Parallel()
	h := NewCircuitHook(50)
	args := stdjson.RawMessage(`{"k":"v"}`)
	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = h(context.Background(), hook.ToolCallEvent{ToolName: "t", Args: args})
		}()
	}
	wg.Wait()
}
