package loop

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	gerrors "github.com/Laisky/errors/v2"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/hook"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/model"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/session"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/tool"
)

// ParallelExecutor fans out a round's function calls per proposal §3.8.
//
// Invariants (all enforced; covered by U23-U27):
//
//   - Bounded concurrency: at most Caps.MaxParallelToolCalls in flight.
//   - Stable upstream order: output slice index == input call index.
//   - First-ask-wins: any sibling's ErrAskUser cancels shared ctx, drains
//     goroutines, returns that error; in-flight results are discarded.
//   - Hooks fire per-call: OnBeforeToolCall -> tool.Execute -> OnAfterToolCall.
type ParallelExecutor struct {
	bus      *hook.Bus
	registry tool.Registry
	sink     session.EventSink
	caps     Caps
	budget   *BudgetCounter
	// stepParentID is the StepStarted event ID that should be used as the
	// ParentEventID for any tool_call_* / tool_result events emitted during
	// ExecuteAll. The loop driver sets it once per round via SetStepParent.
	stepParentID string
}

// NewParallelExecutor wires up the bounded fan-out worker.
func NewParallelExecutor(
	bus *hook.Bus,
	registry tool.Registry,
	sink session.EventSink,
	caps Caps,
	budget *BudgetCounter,
) *ParallelExecutor {
	return &ParallelExecutor{
		bus:      bus,
		registry: registry,
		sink:     sink,
		caps:     caps.withDefaults(),
		budget:   budget,
	}
}

// SetStepParent sets the parent event ID for the tool_call_* / tool_result
// events emitted in the next ExecuteAll call. Used by the loop driver to
// thread the per-step ID through without changing the public ExecuteAll
// signature.
func (p *ParallelExecutor) SetStepParent(stepID string) {
	p.stepParentID = stepID
}

// ExecuteAll dispatches calls concurrently and returns outputs in input
// order. The returned error is one of:
//
//   - nil:  every call produced an output (possibly IsError) and was
//     recorded.
//   - *hook.ErrAskUser: at least one Before/After hook surfaced an ask-user
//     request; siblings were cancelled; outputs are not guaranteed to be
//     filled.
//   - context.Canceled / DeadlineExceeded: caller's context cancelled.
//   - wrapped error: an unrecoverable failure not catchable as ErrAskUser
//     (e.g. registry.Get blowup that shouldn't happen post-validation).
//
// The error sentinel for ErrAskUser is returned as the *hook.ErrAskUser
// pointer (not wrapped) so callers can extract it with errors.As.
func (p *ParallelExecutor) ExecuteAll(
	ctx context.Context,
	calls []model.FunctionCall,
) ([]model.FunctionCallOutput, error) {
	outputs := make([]model.FunctionCallOutput, len(calls))
	if len(calls) == 0 {
		return outputs, nil
	}

	maxConc := p.caps.MaxParallelToolCalls
	if maxConc < 1 {
		maxConc = 1
	}
	sem := make(chan struct{}, maxConc)

	// Shared context that any goroutine can cancel by surfacing an
	// ErrAskUser. Cancellation is the signal to siblings to drop their
	// in-flight work.
	subCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		wg       sync.WaitGroup
		askErrPtr atomicPointer[hook.ErrAskUser]
		fatalErr  atomicPointer[wrappedErr]
	)

	for i, call := range calls {
		i, call := i, call
		wg.Add(1)
		go func() {
			defer wg.Done()

			// Acquire semaphore (or bail on cancellation).
			select {
			case sem <- struct{}{}:
			case <-subCtx.Done():
				return
			}
			defer func() { <-sem }()

			if subCtx.Err() != nil {
				return
			}

			out, askErr, hardErr := p.runOne(subCtx, call)
			switch {
			case askErr != nil:
				// First-ask-wins: only the first goroutine to set the
				// pointer "owns" the cancellation.
				if askErrPtr.CompareAndSwap(nil, askErr) {
					cancel()
				}
				return
			case hardErr != nil:
				if fatalErr.CompareAndSwap(nil, &wrappedErr{err: hardErr, callID: call.CallID}) {
					cancel()
				}
				return
			}
			outputs[i] = out
		}()
	}

	wg.Wait()

	if ask := askErrPtr.Load(); ask != nil {
		return nil, ask
	}
	if fe := fatalErr.Load(); fe != nil {
		return nil, gerrors.Wrapf(fe.err, "parallel tool call %q", fe.callID)
	}
	return outputs, nil
}

// runOne executes a single call with the full hook chain. Returns:
//
//   - output (set on success or synthetic-error path)
//   - *hook.ErrAskUser if any hook surfaced one
//   - error for unrecoverable failures (e.g. unknown tool name)
//
// Tool execution errors (the tool.Execute returning a non-nil error) are
// folded into a synthetic IsError result so the model can see the failure.
// That counts toward the error budget via the after-hook chain (the budget
// enforcer hook is wired in loop.Run).
func (p *ParallelExecutor) runOne(
	ctx context.Context,
	call model.FunctionCall,
) (model.FunctionCallOutput, *hook.ErrAskUser, error) {
	startEvent := session.ToolCallStart{
		BaseEvent:   session.NewBaseEvent(session.KindToolCallStart, p.stepParentID),
		CallID:      call.CallID,
		ToolName:    call.Name,
		ArgsPreview: argsPreview(call.Arguments),
	}
	_ = p.emit(startEvent)

	before := hook.ToolCallEvent{
		ToolName: call.Name,
		CallID:   call.CallID,
		Args:     call.Arguments,
	}

	startedAt := time.Now()

	before, err := p.bus.DispatchBeforeToolCall(ctx, before)
	if err != nil {
		var ask *hook.ErrAskUser
		if errors.As(err, &ask) {
			return model.FunctionCallOutput{}, ask, nil
		}
		// Generic hook error -> synthesize an IsError result so the loop
		// sees a tool-level failure and the model can recover.
		before.Result = &tool.Result{
			Content: fmt.Sprintf("tool call denied by hook: %v", err),
			IsError: true,
		}
		return p.finishWithResult(ctx, call, before, startEvent.EventID(), startedAt)
	}

	// Hook may have synthesized a Result during Before-chain (e.g. circuit
	// breaker hit). In that case we skip tool execution.
	if before.Result == nil {
		t, ok := p.registry.Get(call.Name)
		if !ok {
			before.Result = &tool.Result{
				Content: fmt.Sprintf("unknown tool: %s", call.Name),
				IsError: true,
			}
		} else {
			res, execErr := t.Execute(ctx, tool.Call{
				CallID: call.CallID,
				Name:   call.Name,
				Args:   call.Arguments,
			}, p.sink)
			if execErr != nil {
				// Distinguish context cancellation from real failures so
				// callers see ctx.Err() and not a synthetic result on
				// abort.
				if ctxErr := ctx.Err(); ctxErr != nil {
					return model.FunctionCallOutput{}, nil, ctxErr
				}
				before.Result = &tool.Result{
					Content: fmt.Sprintf("tool error: %v", execErr),
					IsError: true,
				}
			} else {
				before.Result = &res
			}
		}
	}

	return p.finishWithResult(ctx, call, before, startEvent.EventID(), startedAt)
}

// finishWithResult runs the OnAfterToolCall chain and emits the trailing
// trace events. It returns the FunctionCallOutput keyed by call.CallID so
// the caller can place it at the correct upstream-order index.
func (p *ParallelExecutor) finishWithResult(
	ctx context.Context,
	call model.FunctionCall,
	ev hook.ToolCallEvent,
	startEventID string,
	startedAt time.Time,
) (model.FunctionCallOutput, *hook.ErrAskUser, error) {
	ev, err := p.bus.DispatchAfterToolCall(ctx, ev)
	if err != nil {
		var ask *hook.ErrAskUser
		if errors.As(err, &ask) {
			return model.FunctionCallOutput{}, ask, nil
		}
		// Generic after-hook error -> coerce to IsError so the loop sees
		// it. Don't lose the pre-existing result; append the failure.
		base := ""
		if ev.Result != nil {
			base = ev.Result.Content + "\n"
		}
		ev.Result = &tool.Result{
			Content: fmt.Sprintf("%safter-hook error: %v", base, err),
			IsError: true,
		}
	}

	res := ev.Result
	if res == nil {
		// Shouldn't happen in practice — a Before hook either synthesized
		// the result or tool.Execute populated it. Defensive empty result.
		res = &tool.Result{Content: ""}
	}

	duration := time.Since(startedAt)
	endEvent := session.ToolCallEnd{
		BaseEvent:  session.NewBaseEvent(session.KindToolCallEnd, p.stepParentID),
		CallID:     call.CallID,
		DurationMS: duration.Milliseconds(),
	}
	_ = p.emit(endEvent)

	resultEvent := session.ToolResult{
		BaseEvent:      session.NewBaseEvent(session.KindToolResult, p.stepParentID),
		CallID:         call.CallID,
		ContentPreview: contentPreview(res.Content),
		BytesTotal:     len(res.Content),
		IsError:        res.IsError,
	}
	_ = p.emit(resultEvent)

	return model.FunctionCallOutput{
		CallID: call.CallID,
		Output: res.Content,
	}, nil, nil
}

func (p *ParallelExecutor) emit(ev session.Event) error {
	if p.sink == nil {
		return nil
	}
	return p.sink.Emit(ev)
}

// argsPreview truncates a JSON args payload to a small slice suitable for the
// tool_call_start event preview field. We keep the head only — the model's
// arg shapes are typically short JSON objects.
func argsPreview(args []byte) string {
	const max = 256
	if len(args) <= max {
		return string(args)
	}
	return string(args[:max]) + "…"
}

// contentPreview keeps the first ~256 bytes of tool output for the ToolResult
// event preview field. Full content travels separately via FunctionCallOutput.
func contentPreview(content string) string {
	const max = 256
	if len(content) <= max {
		return content
	}
	return content[:max] + "…"
}

// atomicPointer is a small generic wrapper around atomic.Pointer for
// readability. We avoid pulling in any dep beyond the stdlib.
type atomicPointer[T any] struct {
	p unsafe.Pointer
}

func (a *atomicPointer[T]) Load() *T {
	return (*T)(atomic.LoadPointer(&a.p))
}

func (a *atomicPointer[T]) CompareAndSwap(old, new *T) bool {
	return atomic.CompareAndSwapPointer(&a.p, unsafe.Pointer(old), unsafe.Pointer(new))
}

// wrappedErr carries a fatal sibling error together with the call_id that
// produced it, so the executor can wrap it descriptively.
type wrappedErr struct {
	err    error
	callID string
}
