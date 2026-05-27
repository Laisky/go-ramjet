// Package hook provides the Phase 1 agent-loop HookBus and ErrAskUser
// sentinel (proposal §3.5, §3.7). Every cross-cutting concern — memory,
// redaction, telemetry, write-gate — lands here as a hook, not a loop branch.
package hook

import (
	"context"
	"sync"

	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
)

// Bus stores ordered hook chains per Point. Registration order is the firing
// order (verified by U21). The Bus itself does NOT classify errors: it stops
// the chain on the first non-nil error and returns it as-is. The loop runner
// is the one that inspects via errors.As(err, &ErrAskUser{}) and decides
// terminate-the-loop (ErrAskUser) vs. synthesize-an-IsError-result (anything
// else). See proposal §3.5 / §3.7.
//
// Concurrency:
//   - Registration: append-only; safe for concurrent use but Phase 1 callers
//     register every hook at session-start before any dispatch goroutine
//     reads. The internal sync.RWMutex protects the slice header and the
//     append. RWMutex is chosen over the alternative "freeze on first
//     dispatch" snapshot scheme because it is strictly simpler (no state
//     machine, no registration-after-freeze panic to document) and the
//     RLock cost per dispatch is dwarfed by the hook bodies themselves
//     (memory I/O, redaction, etc.).
//   - Dispatch: safe for concurrent invocation; the parallel executor in
//     §3.8 calls DispatchBeforeToolCall / DispatchAfterToolCall from
//     bounded goroutines and the verification race test in this package
//     covers that.
type Bus struct {
	logger glog.Logger
	mu     sync.RWMutex

	sessionStart   []func(context.Context, SessionStartEvent) (SessionStartEvent, error)
	contextHooks   []func(context.Context, ContextEvent) (ContextEvent, error)
	beforeToolCall []func(context.Context, ToolCallEvent) (ToolCallEvent, error)
	afterToolCall  []func(context.Context, ToolCallEvent) (ToolCallEvent, error)
	beforeCompact  []func(context.Context, CompactEvent) (CompactEvent, error)
	sessionEnd     []func(context.Context, SessionEndEvent) (SessionEndEvent, error)
}

// NewBus returns an empty Bus that logs hook-chain progress through the
// supplied logger. A nil logger is tolerated (lifecycle traces become
// silent); production callers must provide one.
func NewBus(logger glog.Logger) *Bus {
	return &Bus{logger: logger}
}

// OnSessionStart appends h to the session-start chain. Hooks fire in
// registration order; later hooks receive earlier hooks' (possibly
// transformed) output.
func (b *Bus) OnSessionStart(h func(context.Context, SessionStartEvent) (SessionStartEvent, error)) {
	if h == nil {
		return
	}
	b.mu.Lock()
	b.sessionStart = append(b.sessionStart, h)
	b.mu.Unlock()
}

// OnContext appends h to the context chain.
func (b *Bus) OnContext(h func(context.Context, ContextEvent) (ContextEvent, error)) {
	if h == nil {
		return
	}
	b.mu.Lock()
	b.contextHooks = append(b.contextHooks, h)
	b.mu.Unlock()
}

// OnBeforeToolCall appends h to the before-tool-call chain.
func (b *Bus) OnBeforeToolCall(h func(context.Context, ToolCallEvent) (ToolCallEvent, error)) {
	if h == nil {
		return
	}
	b.mu.Lock()
	b.beforeToolCall = append(b.beforeToolCall, h)
	b.mu.Unlock()
}

// OnAfterToolCall appends h to the after-tool-call chain.
func (b *Bus) OnAfterToolCall(h func(context.Context, ToolCallEvent) (ToolCallEvent, error)) {
	if h == nil {
		return
	}
	b.mu.Lock()
	b.afterToolCall = append(b.afterToolCall, h)
	b.mu.Unlock()
}

// OnBeforeCompact appends h to the before-compact chain.
func (b *Bus) OnBeforeCompact(h func(context.Context, CompactEvent) (CompactEvent, error)) {
	if h == nil {
		return
	}
	b.mu.Lock()
	b.beforeCompact = append(b.beforeCompact, h)
	b.mu.Unlock()
}

// OnSessionEnd appends h to the session-end chain.
func (b *Bus) OnSessionEnd(h func(context.Context, SessionEndEvent) (SessionEndEvent, error)) {
	if h == nil {
		return
	}
	b.mu.Lock()
	b.sessionEnd = append(b.sessionEnd, h)
	b.mu.Unlock()
}

// DispatchSessionStart runs the session-start chain in registration order.
// Each hook receives the previous hook's output as its input. The first
// non-nil error short-circuits the chain and is returned to the caller.
func (b *Bus) DispatchSessionStart(ctx context.Context, ev SessionStartEvent) (SessionStartEvent, error) {
	b.mu.RLock()
	chain := append([]func(context.Context, SessionStartEvent) (SessionStartEvent, error)(nil), b.sessionStart...)
	b.mu.RUnlock()
	for i, h := range chain {
		next, err := h(ctx, ev)
		if err != nil {
			b.logErr(PointSessionStart, i, err)
			return ev, err
		}
		ev = next
	}
	return ev, nil
}

// DispatchContext runs the context chain in registration order.
func (b *Bus) DispatchContext(ctx context.Context, ev ContextEvent) (ContextEvent, error) {
	b.mu.RLock()
	chain := append([]func(context.Context, ContextEvent) (ContextEvent, error)(nil), b.contextHooks...)
	b.mu.RUnlock()
	for i, h := range chain {
		next, err := h(ctx, ev)
		if err != nil {
			b.logErr(PointContext, i, err)
			return ev, err
		}
		ev = next
	}
	return ev, nil
}

// DispatchBeforeToolCall runs the before-tool-call chain in registration
// order. An error here is what the loop translates into either a synthetic
// IsError result (generic error) or an ask_user Final (ErrAskUser).
func (b *Bus) DispatchBeforeToolCall(ctx context.Context, ev ToolCallEvent) (ToolCallEvent, error) {
	b.mu.RLock()
	chain := append([]func(context.Context, ToolCallEvent) (ToolCallEvent, error)(nil), b.beforeToolCall...)
	b.mu.RUnlock()
	for i, h := range chain {
		next, err := h(ctx, ev)
		if err != nil {
			b.logErr(PointBeforeToolCall, i, err)
			return ev, err
		}
		ev = next
	}
	return ev, nil
}

// DispatchAfterToolCall runs the after-tool-call chain in registration order.
func (b *Bus) DispatchAfterToolCall(ctx context.Context, ev ToolCallEvent) (ToolCallEvent, error) {
	b.mu.RLock()
	chain := append([]func(context.Context, ToolCallEvent) (ToolCallEvent, error)(nil), b.afterToolCall...)
	b.mu.RUnlock()
	for i, h := range chain {
		next, err := h(ctx, ev)
		if err != nil {
			b.logErr(PointAfterToolCall, i, err)
			return ev, err
		}
		ev = next
	}
	return ev, nil
}

// DispatchBeforeCompact runs the before-compact chain in registration order.
func (b *Bus) DispatchBeforeCompact(ctx context.Context, ev CompactEvent) (CompactEvent, error) {
	b.mu.RLock()
	chain := append([]func(context.Context, CompactEvent) (CompactEvent, error)(nil), b.beforeCompact...)
	b.mu.RUnlock()
	for i, h := range chain {
		next, err := h(ctx, ev)
		if err != nil {
			b.logErr(PointBeforeCompact, i, err)
			return ev, err
		}
		ev = next
	}
	return ev, nil
}

// DispatchSessionEnd runs the session-end chain in registration order.
func (b *Bus) DispatchSessionEnd(ctx context.Context, ev SessionEndEvent) (SessionEndEvent, error) {
	b.mu.RLock()
	chain := append([]func(context.Context, SessionEndEvent) (SessionEndEvent, error)(nil), b.sessionEnd...)
	b.mu.RUnlock()
	for i, h := range chain {
		next, err := h(ctx, ev)
		if err != nil {
			b.logErr(PointSessionEnd, i, err)
			return ev, err
		}
		ev = next
	}
	return ev, nil
}

// logErr emits a single agent_hook_err line for the failing hook. Kept
// behind a helper so dispatch sites stay readable.
func (b *Bus) logErr(point Point, index int, err error) {
	if b.logger == nil {
		return
	}
	b.logger.Debug("agent_hook_err",
		zap.String("point", string(point)),
		zap.Int("index", index),
		zap.Error(err),
	)
}
