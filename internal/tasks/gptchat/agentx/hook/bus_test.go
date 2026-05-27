package hook

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/model"
)

// TestBus_NewBus_NilLoggerTolerated ensures NewBus accepts a nil logger and
// that subsequent dispatch with a failing hook does not panic.
func TestBus_NewBus_NilLoggerTolerated(t *testing.T) {
	t.Parallel()

	bus := NewBus(nil)
	require.NotNil(t, bus)

	bus.OnContext(func(_ context.Context, ev ContextEvent) (ContextEvent, error) {
		return ev, errors.New("boom")
	})
	_, err := bus.DispatchContext(context.Background(), ContextEvent{})
	require.Error(t, err)
}

// TestBus_EmptyChainPassesThrough verifies the empty-chain contract: every
// dispatch returns the input event unchanged with nil error.
func TestBus_EmptyChainPassesThrough(t *testing.T) {
	t.Parallel()

	bus := NewBus(nil)
	ctx := context.Background()

	{
		in := SessionStartEvent{SessionID: "s1", Caps: Caps{MaxIterations: 20}}
		out, err := bus.DispatchSessionStart(ctx, in)
		require.NoError(t, err)
		require.Equal(t, in, out)
	}
	{
		in := ContextEvent{Input: []model.InputItem{"a", "b"}}
		out, err := bus.DispatchContext(ctx, in)
		require.NoError(t, err)
		require.Equal(t, in, out)
	}
	{
		in := ToolCallEvent{ToolName: "web_search", CallID: "c1"}
		out, err := bus.DispatchBeforeToolCall(ctx, in)
		require.NoError(t, err)
		require.Equal(t, in, out)
		out, err = bus.DispatchAfterToolCall(ctx, in)
		require.NoError(t, err)
		require.Equal(t, in, out)
	}
	{
		in := CompactEvent{}
		out, err := bus.DispatchBeforeCompact(ctx, in)
		require.NoError(t, err)
		require.Equal(t, in, out)
	}
	{
		in := SessionEndEvent{SessionID: "s1", TerminatedBy: "send_to_user", FinalText: "ok"}
		out, err := bus.DispatchSessionEnd(ctx, in)
		require.NoError(t, err)
		require.Equal(t, in, out)
	}
}

// TestBus_HookOrdering_U21 covers proposal §6.1 U21. Two OnContext hooks A
// then B; B receives A's output unchanged; the dispatch result is B's
// transformation. The verification appends sentinel strings into Input so
// the final order is observable.
func TestBus_HookOrdering_U21(t *testing.T) {
	t.Parallel()

	bus := NewBus(nil)
	bus.OnContext(func(_ context.Context, ev ContextEvent) (ContextEvent, error) {
		ev.Input = append(append([]model.InputItem(nil), ev.Input...), "A")
		return ev, nil
	})
	bus.OnContext(func(_ context.Context, ev ContextEvent) (ContextEvent, error) {
		// B must see the "A" that the prior hook appended.
		require.NotEmpty(t, ev.Input)
		require.Equal(t, "A", ev.Input[len(ev.Input)-1])
		ev.Input = append(append([]model.InputItem(nil), ev.Input...), "B")
		return ev, nil
	})

	out, err := bus.DispatchContext(context.Background(), ContextEvent{Input: []model.InputItem{"seed"}})
	require.NoError(t, err)
	require.Equal(t, []model.InputItem{"seed", "A", "B"}, out.Input)
}

// TestBus_HookDeny_U22 covers proposal §6.1 U22. An OnBeforeToolCall hook
// returns a non-ErrAskUser error; dispatch surfaces it faithfully. The
// loop-side test that this becomes a synthetic IsError result lives in the
// loop package; here we only assert the Bus contract.
func TestBus_HookDeny_U22(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("denied by policy")
	bus := NewBus(nil)
	bus.OnBeforeToolCall(func(_ context.Context, ev ToolCallEvent) (ToolCallEvent, error) {
		return ev, sentinel
	})

	in := ToolCallEvent{ToolName: "file_write", CallID: "c1"}
	out, err := bus.DispatchBeforeToolCall(context.Background(), in)
	require.ErrorIs(t, err, sentinel)
	// On error, the input event is returned unchanged.
	require.Equal(t, in, out)
	// Importantly: this is NOT an ErrAskUser; the loop must distinguish.
	var asAsk *ErrAskUser
	require.False(t, errors.As(err, &asAsk))
}

// TestBus_MultipleHooks_StopOnError registers 5 hooks; hook #3 errors;
// hooks #4 and #5 must not fire and the dispatch error is the one from #3.
func TestBus_MultipleHooks_StopOnError(t *testing.T) {
	t.Parallel()

	bus := NewBus(nil)
	var calls []int
	stop := errors.New("stop here")
	for i := 1; i <= 5; i++ {
		i := i
		bus.OnContext(func(_ context.Context, ev ContextEvent) (ContextEvent, error) {
			calls = append(calls, i)
			if i == 3 {
				return ev, stop
			}
			return ev, nil
		})
	}

	_, err := bus.DispatchContext(context.Background(), ContextEvent{})
	require.ErrorIs(t, err, stop)
	require.Equal(t, []int{1, 2, 3}, calls)
}

// TestBus_MultipleHooks_AllFireOnSuccess registers 5 hooks; verifies all
// fire in registration order when none error.
func TestBus_MultipleHooks_AllFireOnSuccess(t *testing.T) {
	t.Parallel()

	bus := NewBus(nil)
	var calls []int
	for i := 1; i <= 5; i++ {
		i := i
		bus.OnAfterToolCall(func(_ context.Context, ev ToolCallEvent) (ToolCallEvent, error) {
			calls = append(calls, i)
			return ev, nil
		})
	}

	_, err := bus.DispatchAfterToolCall(context.Background(), ToolCallEvent{})
	require.NoError(t, err)
	require.Equal(t, []int{1, 2, 3, 4, 5}, calls)
}

// TestBus_AllRegistrationMethods sanity-checks one hook registered on each
// point fires exactly once with the correct payload type.
func TestBus_AllRegistrationMethods(t *testing.T) {
	t.Parallel()

	bus := NewBus(nil)
	ctx := context.Background()

	var seen [6]bool
	bus.OnSessionStart(func(_ context.Context, ev SessionStartEvent) (SessionStartEvent, error) {
		seen[0] = true
		return ev, nil
	})
	bus.OnContext(func(_ context.Context, ev ContextEvent) (ContextEvent, error) {
		seen[1] = true
		return ev, nil
	})
	bus.OnBeforeToolCall(func(_ context.Context, ev ToolCallEvent) (ToolCallEvent, error) {
		seen[2] = true
		return ev, nil
	})
	bus.OnAfterToolCall(func(_ context.Context, ev ToolCallEvent) (ToolCallEvent, error) {
		seen[3] = true
		return ev, nil
	})
	bus.OnBeforeCompact(func(_ context.Context, ev CompactEvent) (CompactEvent, error) {
		seen[4] = true
		return ev, nil
	})
	bus.OnSessionEnd(func(_ context.Context, ev SessionEndEvent) (SessionEndEvent, error) {
		seen[5] = true
		return ev, nil
	})

	_, _ = bus.DispatchSessionStart(ctx, SessionStartEvent{})
	_, _ = bus.DispatchContext(ctx, ContextEvent{})
	_, _ = bus.DispatchBeforeToolCall(ctx, ToolCallEvent{})
	_, _ = bus.DispatchAfterToolCall(ctx, ToolCallEvent{})
	_, _ = bus.DispatchBeforeCompact(ctx, CompactEvent{})
	_, _ = bus.DispatchSessionEnd(ctx, SessionEndEvent{})

	for i, ok := range seen {
		require.Truef(t, ok, "hook at point %d not fired", i)
	}
}

// TestBus_NilHook_Ignored ensures registering a nil function is a no-op
// rather than a deferred nil-pointer panic at dispatch time.
func TestBus_NilHook_Ignored(t *testing.T) {
	t.Parallel()

	bus := NewBus(nil)
	bus.OnContext(nil)
	bus.OnBeforeToolCall(nil)
	bus.OnAfterToolCall(nil)
	bus.OnSessionStart(nil)
	bus.OnBeforeCompact(nil)
	bus.OnSessionEnd(nil)

	_, err := bus.DispatchContext(context.Background(), ContextEvent{})
	require.NoError(t, err)
}

// TestBus_ConcurrentDispatch fires 100 goroutines through a 3-hook chain on
// the same Bus; -race verifies the read snapshot is safe. Each goroutine
// gets a unique input sentinel and asserts the chain's transformations
// land on its own input — no cross-talk.
func TestBus_ConcurrentDispatch(t *testing.T) {
	t.Parallel()

	bus := NewBus(nil)
	var dispatched atomic.Int64

	bus.OnContext(func(_ context.Context, ev ContextEvent) (ContextEvent, error) {
		dispatched.Add(1)
		ev.Input = append(append([]model.InputItem(nil), ev.Input...), "x")
		return ev, nil
	})
	bus.OnContext(func(_ context.Context, ev ContextEvent) (ContextEvent, error) {
		ev.Input = append(append([]model.InputItem(nil), ev.Input...), "y")
		return ev, nil
	})
	bus.OnContext(func(_ context.Context, ev ContextEvent) (ContextEvent, error) {
		ev.Input = append(append([]model.InputItem(nil), ev.Input...), "z")
		return ev, nil
	})

	const N = 100
	var wg sync.WaitGroup
	wg.Add(N)
	errCh := make(chan error, N)
	for i := 0; i < N; i++ {
		i := i
		go func() {
			defer wg.Done()
			seed := model.InputItem(i)
			out, err := bus.DispatchContext(context.Background(), ContextEvent{Input: []model.InputItem{seed}})
			if err != nil {
				errCh <- err
				return
			}
			want := []model.InputItem{seed, "x", "y", "z"}
			if len(out.Input) != len(want) {
				errCh <- errors.New("unexpected length")
				return
			}
			for j, v := range want {
				if out.Input[j] != v {
					errCh <- errors.New("unexpected value")
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		require.NoError(t, err)
	}
	require.Equal(t, int64(N), dispatched.Load())
}

// TestBus_ErrorOnHookReturnsInputUnchanged formalizes the contract that a
// failing hook returns the *input* event to the dispatcher, not a partially
// mutated copy from the failing hook itself.
func TestBus_ErrorOnHookReturnsInputUnchanged(t *testing.T) {
	t.Parallel()

	bus := NewBus(nil)
	bus.OnContext(func(_ context.Context, ev ContextEvent) (ContextEvent, error) {
		ev.Input = append([]model.InputItem(nil), ev.Input...)
		ev.Input = append(ev.Input, "ok")
		return ev, nil
	})
	bus.OnContext(func(_ context.Context, ev ContextEvent) (ContextEvent, error) {
		// Pretend we mutated and then errored. The dispatcher must
		// return whatever we returned alongside the error verbatim.
		ev.Input = append([]model.InputItem(nil), ev.Input...)
		ev.Input = append(ev.Input, "discarded")
		return ev, errors.New("oops")
	})

	in := ContextEvent{Input: []model.InputItem{"seed"}}
	out, err := bus.DispatchContext(context.Background(), in)
	require.Error(t, err)
	// Dispatch returns the *input to the failing hook* (== prior hook's
	// output), not the failing hook's discarded return value.
	require.Equal(t, []model.InputItem{"seed", "ok"}, out.Input)
}
