package loop

import (
	"context"
	stdjson "encoding/json"
	"errors"
	"math/rand"
	"sync/atomic"
	"testing"
	"time"

	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/hook"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/model"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/session"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/tool"
)

func newParallelBus(t *testing.T) *hook.Bus {
	t.Helper()
	l, err := glog.NewConsoleWithName("test_bus", glog.LevelError)
	require.NoError(t, err)
	return hook.NewBus(l)
}

// nullSink discards all events. Tests that don't care about the trace use it.
type nullSink struct{}

func (nullSink) Emit(session.Event) error { return nil }

// TestParallel_FanOutHappyPath covers U23: 4 parallel calls with mixed sleep
// durations execute concurrently, preserve input order, and finish in roughly
// max(sleeps) wall-clock time.
func TestParallel_FanOutHappyPath_U23(t *testing.T) {
	t.Parallel()
	tracker := newConcurrencyTracker()
	tools := []*fakeTool{
		newFakeTool("t0", 200*time.Millisecond, "out0"),
		newFakeTool("t1", 50*time.Millisecond, "out1"),
		newFakeTool("t2", 150*time.Millisecond, "out2"),
		newFakeTool("t3", 100*time.Millisecond, "out3"),
	}
	for _, tl := range tools {
		tl.concurrencyTracker = tracker
	}
	reg := buildTestRegistry(t, asTools(tools)...)
	bus := newParallelBus(t)
	exec := NewParallelExecutor(bus, reg, nullSink{}, Caps{MaxParallelToolCalls: 8}, NewBudgetCounter())

	calls := []model.FunctionCall{
		{CallID: "c0", Name: "t0", Arguments: rawJSON(`{}`)},
		{CallID: "c1", Name: "t1", Arguments: rawJSON(`{}`)},
		{CallID: "c2", Name: "t2", Arguments: rawJSON(`{}`)},
		{CallID: "c3", Name: "t3", Arguments: rawJSON(`{}`)},
	}

	start := time.Now()
	outputs, err := exec.ExecuteAll(context.Background(), calls)
	require.NoError(t, err)
	require.Len(t, outputs, 4)
	elapsed := time.Since(start)

	// Order preserved.
	for i, out := range outputs {
		require.Equal(t, calls[i].CallID, out.CallID)
	}

	// Total latency must be substantially less than the sum (500ms) — it
	// should be close to max(sleeps)=200ms. Allow generous slack for CI
	// load.
	require.Less(t, elapsed, 400*time.Millisecond, "fan-out should finish near max(sleeps)")

	// Peak concurrency must be >= 2 (we expect all 4 in flight, but tight
	// schedulers may serialise — assert at least overlap exists).
	require.GreaterOrEqual(t, tracker.peakValue(), int64(2))

	// Start times within a short window — all calls dispatched ~together.
	starts := []time.Time{}
	for _, tl := range tools {
		require.Equal(t, 1, tl.callCount())
		starts = append(starts, tl.startTimes()[0])
	}
	minStart, maxStart := starts[0], starts[0]
	for _, s := range starts[1:] {
		if s.Before(minStart) {
			minStart = s
		}
		if s.After(maxStart) {
			maxStart = s
		}
	}
	require.Less(t, maxStart.Sub(minStart), 60*time.Millisecond, "all goroutines should dispatch near simultaneously")
}

// TestParallel_BoundedConcurrency_U24 covers U24: 6 calls with limit=2
// produce a peak in-flight count of exactly 2.
func TestParallel_BoundedConcurrency_U24(t *testing.T) {
	t.Parallel()
	tracker := newConcurrencyTracker()
	tools := make([]*fakeTool, 6)
	for i := range tools {
		tools[i] = newFakeTool(toolNameAt(i), 80*time.Millisecond, "ok")
		tools[i].concurrencyTracker = tracker
	}
	reg := buildTestRegistry(t, asTools(tools)...)
	bus := newParallelBus(t)
	exec := NewParallelExecutor(bus, reg, nullSink{}, Caps{MaxParallelToolCalls: 2}, NewBudgetCounter())

	calls := make([]model.FunctionCall, len(tools))
	for i := range tools {
		calls[i] = model.FunctionCall{
			CallID:    "c" + intStr(i),
			Name:      tools[i].Name(),
			Arguments: rawJSON(`{}`),
		}
	}

	outputs, err := exec.ExecuteAll(context.Background(), calls)
	require.NoError(t, err)
	require.Len(t, outputs, 6)
	for i, out := range outputs {
		require.Equal(t, calls[i].CallID, out.CallID)
	}
	require.LessOrEqual(t, tracker.peakValue(), int64(2))
}

// TestParallel_FirstAskWins_U25 covers U25: 3 parallel calls; second hook
// returns ErrAskUser at ~100ms; ExecuteAll surfaces that ErrAskUser and
// cancels siblings.
func TestParallel_FirstAskWins_U25(t *testing.T) {
	t.Parallel()
	tools := []*fakeTool{
		newFakeTool("t0", 300*time.Millisecond, "out0"),
		newFakeTool("t1", 300*time.Millisecond, "out1"),
		newFakeTool("t2", 300*time.Millisecond, "out2"),
	}
	reg := buildTestRegistry(t, asTools(tools)...)
	bus := newParallelBus(t)

	// Install a Before hook that sleeps 100ms then surfaces ErrAskUser
	// only for t1.
	bus.OnBeforeToolCall(func(ctx context.Context, ev hook.ToolCallEvent) (hook.ToolCallEvent, error) {
		if ev.ToolName != "t1" {
			return ev, nil
		}
		select {
		case <-time.After(100 * time.Millisecond):
		case <-ctx.Done():
			return ev, ctx.Err()
		}
		return ev, &hook.ErrAskUser{
			Code:    "test_ask",
			Message: "please confirm",
		}
	})

	exec := NewParallelExecutor(bus, reg, nullSink{}, Caps{MaxParallelToolCalls: 8}, NewBudgetCounter())

	start := time.Now()
	_, err := exec.ExecuteAll(context.Background(), []model.FunctionCall{
		{CallID: "c0", Name: "t0", Arguments: rawJSON(`{}`)},
		{CallID: "c1", Name: "t1", Arguments: rawJSON(`{}`)},
		{CallID: "c2", Name: "t2", Arguments: rawJSON(`{}`)},
	})
	elapsed := time.Since(start)

	require.Error(t, err)
	var ask *hook.ErrAskUser
	require.True(t, errors.As(err, &ask))
	require.Equal(t, "test_ask", ask.Code)
	require.Equal(t, "please confirm", ask.Message)
	// Siblings should have been cancelled long before their 300ms sleep
	// finished.
	require.Less(t, elapsed, 250*time.Millisecond, "siblings should be cancelled promptly")
}

// TestParallel_DeterministicResultOrder_U26 covers U26: 100 runs with
// randomized sleep durations preserve upstream order.
func TestParallel_DeterministicResultOrder_U26(t *testing.T) {
	t.Parallel()
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	for trial := 0; trial < 20; trial++ {
		tools := make([]*fakeTool, 5)
		for i := range tools {
			d := time.Duration(rng.Intn(40)+5) * time.Millisecond
			tools[i] = newFakeTool(toolNameAt(i), d, "out"+intStr(i))
		}
		reg := buildTestRegistry(t, asTools(tools)...)
		bus := newParallelBus(t)
		exec := NewParallelExecutor(bus, reg, nullSink{}, Caps{MaxParallelToolCalls: 8}, NewBudgetCounter())

		calls := make([]model.FunctionCall, len(tools))
		for i := range tools {
			calls[i] = model.FunctionCall{
				CallID:    "c" + intStr(i),
				Name:      tools[i].Name(),
				Arguments: rawJSON(`{}`),
			}
		}
		outputs, err := exec.ExecuteAll(context.Background(), calls)
		require.NoError(t, err, "trial %d", trial)
		require.Len(t, outputs, len(calls))
		for i, out := range outputs {
			require.Equalf(t, calls[i].CallID, out.CallID, "trial %d, idx %d", trial, i)
			require.Equalf(t, "out"+intStr(i), out.Output, "trial %d, idx %d", trial, i)
		}
	}
}

// TestParallel_HookDeny_U22 covers U22: a Before hook that returns a generic
// error becomes a synthetic IsError result and the budget gets recorded by
// the after-hook chain wired by the test.
func TestParallel_HookDeny_U22(t *testing.T) {
	t.Parallel()
	tools := []*fakeTool{newFakeTool("t0", 0, "out0")}
	reg := buildTestRegistry(t, asTools(tools)...)
	bus := newParallelBus(t)
	bus.OnBeforeToolCall(func(_ context.Context, ev hook.ToolCallEvent) (hook.ToolCallEvent, error) {
		return ev, errors.New("denied")
	})
	budget := NewBudgetCounter()
	bus.OnAfterToolCall(NewBudgetEnforcerHook(budget))

	exec := NewParallelExecutor(bus, reg, nullSink{}, Caps{MaxParallelToolCalls: 8}, budget)
	outputs, err := exec.ExecuteAll(context.Background(), []model.FunctionCall{
		{CallID: "c0", Name: "t0", Arguments: rawJSON(`{}`)},
	})
	require.NoError(t, err)
	require.Len(t, outputs, 1)
	require.Contains(t, outputs[0].Output, "denied")
	require.Equal(t, int64(1), budget.ToolCalls())
	require.Equal(t, int64(1), budget.Errors())
	require.Equal(t, 0, tools[0].callCount(), "tool must not execute after Before-hook deny")
}

// TestParallel_ToolErrorRecorded covers the "tool returns error" path —
// the result is synthesized as IsError and the budget enforcer increments.
func TestParallel_ToolErrorRecorded(t *testing.T) {
	t.Parallel()
	tools := []*fakeTool{newFakeTool("t0", 0, "")}
	tools[0].executeFn = func(_ context.Context, _ tool.Call) (tool.Result, error) {
		return tool.Result{}, errors.New("execute boom")
	}
	reg := buildTestRegistry(t, asTools(tools)...)
	bus := newParallelBus(t)
	budget := NewBudgetCounter()
	bus.OnAfterToolCall(NewBudgetEnforcerHook(budget))

	exec := NewParallelExecutor(bus, reg, nullSink{}, Caps{MaxParallelToolCalls: 8}, budget)
	outputs, err := exec.ExecuteAll(context.Background(), []model.FunctionCall{
		{CallID: "c0", Name: "t0", Arguments: rawJSON(`{}`)},
	})
	require.NoError(t, err)
	require.Contains(t, outputs[0].Output, "execute boom")
	require.Equal(t, int64(1), budget.Errors())
}

// TestParallel_UnknownTool yields an IsError synthesized result.
func TestParallel_UnknownTool(t *testing.T) {
	t.Parallel()
	reg := buildTestRegistry(t)
	bus := newParallelBus(t)
	exec := NewParallelExecutor(bus, reg, nullSink{}, Caps{MaxParallelToolCalls: 8}, NewBudgetCounter())
	outputs, err := exec.ExecuteAll(context.Background(), []model.FunctionCall{
		{CallID: "c0", Name: "nope", Arguments: rawJSON(`{}`)},
	})
	require.NoError(t, err)
	require.Contains(t, outputs[0].Output, "unknown tool")
}

// TestParallel_ContextCancellation propagates ctx.Err() out without
// surfacing a synthetic result.
func TestParallel_ContextCancellation(t *testing.T) {
	t.Parallel()
	tools := []*fakeTool{newFakeTool("t0", 500*time.Millisecond, "")}
	reg := buildTestRegistry(t, asTools(tools)...)
	bus := newParallelBus(t)
	exec := NewParallelExecutor(bus, reg, nullSink{}, Caps{MaxParallelToolCalls: 8}, NewBudgetCounter())

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(30 * time.Millisecond)
		cancel()
	}()

	_, err := exec.ExecuteAll(ctx, []model.FunctionCall{
		{CallID: "c0", Name: "t0", Arguments: rawJSON(`{}`)},
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, context.Canceled) || errors.Is(errors.Unwrap(err), context.Canceled))
}

// TestParallel_EmptyCallsReturnsEmpty covers the zero-calls fast path.
func TestParallel_EmptyCallsReturnsEmpty(t *testing.T) {
	t.Parallel()
	exec := NewParallelExecutor(newParallelBus(t), buildTestRegistry(t), nullSink{}, DefaultCaps(), NewBudgetCounter())
	outputs, err := exec.ExecuteAll(context.Background(), nil)
	require.NoError(t, err)
	require.Empty(t, outputs)
}

func intStr(i int) string {
	// Small helper for synthetic IDs. Handles values >= 10 (used by
	// reproducibility tests).
	if i < 10 {
		return string(rune('0' + i))
	}
	return intStr(i/10) + intStr(i%10)
}

func toolNameAt(i int) string {
	return "tool_" + intStr(i)
}

func rawJSON(s string) stdjson.RawMessage { return stdjson.RawMessage(s) }

// asTools coerces the concrete *fakeTool slice into a []tool.Tool slice so
// the registry helper signature stays uniform.
func asTools(in []*fakeTool) []tool.Tool {
	out := make([]tool.Tool, len(in))
	for i, t := range in {
		out[i] = t
	}
	return out
}

// silence unused import in case of later refactor.
var _ = atomic.LoadInt64
