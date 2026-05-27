package loop

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/hook"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/tool"
)

func TestBudgetCounter_AtomicIncrements(t *testing.T) {
	t.Parallel()
	b := NewBudgetCounter()
	require.Equal(t, int64(0), b.ToolCalls())
	require.Equal(t, int64(0), b.Errors())

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b.RecordToolCall()
			b.RecordError()
		}()
	}
	wg.Wait()

	require.Equal(t, int64(100), b.ToolCalls())
	require.Equal(t, int64(100), b.Errors())
}

func TestBudgetCounter_NilSafe(t *testing.T) {
	t.Parallel()
	var b *BudgetCounter
	require.NotPanics(t, func() {
		b.RecordToolCall()
		b.RecordError()
	})
	require.Equal(t, int64(0), b.ToolCalls())
	require.Equal(t, int64(0), b.Errors())
}

func TestBudgetEnforcerHook_RecordsCalls(t *testing.T) {
	t.Parallel()
	b := NewBudgetCounter()
	h := NewBudgetEnforcerHook(b)

	out, err := h(context.Background(), hook.ToolCallEvent{
		ToolName: "web_search",
		CallID:   "c1",
		Result:   &tool.Result{Content: "ok"},
	})
	require.NoError(t, err)
	require.NotNil(t, out.Result)
	require.Equal(t, int64(1), b.ToolCalls())
	require.Equal(t, int64(0), b.Errors())

	_, err = h(context.Background(), hook.ToolCallEvent{
		ToolName: "web_search",
		CallID:   "c2",
		Result:   &tool.Result{Content: "boom", IsError: true},
	})
	require.NoError(t, err)
	require.Equal(t, int64(2), b.ToolCalls())
	require.Equal(t, int64(1), b.Errors())
}

func TestBudgetEnforcerHook_NilCounter(t *testing.T) {
	t.Parallel()
	h := NewBudgetEnforcerHook(nil)
	_, err := h(context.Background(), hook.ToolCallEvent{
		ToolName: "x",
		Result:   &tool.Result{},
	})
	require.NoError(t, err)
}
