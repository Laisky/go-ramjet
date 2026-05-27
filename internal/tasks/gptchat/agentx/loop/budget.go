package loop

import (
	"context"
	"sync/atomic"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/hook"
)

// BudgetCounter tracks total tool calls and total tool errors across a single
// agent run. The parallel executor increments these from multiple goroutines,
// so the underlying storage is atomic.
//
// The counters are intentionally monotonic: the loop driver reads them once
// per round to enforce caps and never resets them.
type BudgetCounter struct {
	toolCalls atomic.Int64
	errors    atomic.Int64
}

// NewBudgetCounter returns a fresh zeroed counter.
func NewBudgetCounter() *BudgetCounter {
	return &BudgetCounter{}
}

// RecordToolCall increments the tool-call counter by one.
func (b *BudgetCounter) RecordToolCall() {
	if b == nil {
		return
	}
	b.toolCalls.Add(1)
}

// RecordError increments the error counter by one.
func (b *BudgetCounter) RecordError() {
	if b == nil {
		return
	}
	b.errors.Add(1)
}

// ToolCalls returns the current tool-call count.
func (b *BudgetCounter) ToolCalls() int64 {
	if b == nil {
		return 0
	}
	return b.toolCalls.Load()
}

// Errors returns the current error count.
func (b *BudgetCounter) Errors() int64 {
	if b == nil {
		return 0
	}
	return b.errors.Load()
}

// NewBudgetEnforcerHook returns an OnAfterToolCall hook that records each
// terminal tool result against the supplied counter. Errors observed via
// ev.Result.IsError increment the error counter; every invocation increments
// the tool-call counter.
//
// The hook itself never terminates the loop — the loop driver inspects the
// counter after each round and decides whether to terminate. This keeps the
// hook side-effect-free apart from the counter mutation, and keeps the
// termination decision in one place.
func NewBudgetEnforcerHook(budget *BudgetCounter) func(context.Context, hook.ToolCallEvent) (hook.ToolCallEvent, error) {
	return func(_ context.Context, ev hook.ToolCallEvent) (hook.ToolCallEvent, error) {
		if budget == nil {
			return ev, nil
		}
		budget.RecordToolCall()
		if ev.Result != nil && ev.Result.IsError {
			budget.RecordError()
		}
		return ev, nil
	}
}
