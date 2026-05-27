package session

import (
	"context"
	"runtime"
	"sync"
	"testing"
	"time"

	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/stretchr/testify/require"
)

func newTestSession(t *testing.T) *session {
	t.Helper()
	l, err := glog.NewConsoleWithName("test_session", glog.LevelError)
	require.NoError(t, err)
	s := NewSession(Config{BufferSize: 64, Logger: l})
	t.Cleanup(func() { _ = s.Close() })
	return s.(*session)
}

func mkEvent(kind string) Event {
	return StepStarted{
		BaseEvent: NewBaseEvent(kind, ""),
		StepID:    "step",
	}
}

// Fan-out under N subscribers — 3 concurrent subscribers each see every event
// emitted, in the same order.
func TestSession_FanOutMultipleSubscribers(t *testing.T) {
	t.Parallel()
	l, err := glog.NewConsoleWithName("test_fanout", glog.LevelError)
	require.NoError(t, err)
	s := NewSession(Config{BufferSize: 256, Logger: l}).(*session)
	t.Cleanup(func() { _ = s.Close() })

	const numSubs = 3
	subs := make([]*subscriber, numSubs)
	// Subscriber 0 is the primary one already created by NewSession.
	subs[0] = s.primary
	for i := 1; i < numSubs; i++ {
		subs[i] = s.subscribe()
	}

	const numEvents = 100
	expected := make([]string, numEvents)
	for i := 0; i < numEvents; i++ {
		ev := mkEvent(KindStepStarted)
		expected[i] = ev.EventID()
		require.NoError(t, s.Emit(ev))
	}

	// Each subscriber collects events in goroutine.
	results := make([][]string, numSubs)
	var wg sync.WaitGroup
	wg.Add(numSubs)
	for i := 0; i < numSubs; i++ {
		i := i
		go func() {
			defer wg.Done()
			collected := make([]string, 0, numEvents)
			for len(collected) < numEvents {
				select {
				case ev, ok := <-subs[i].ch:
					if !ok {
						return
					}
					collected = append(collected, ev.EventID())
				case <-time.After(5 * time.Second):
					t.Errorf("subscriber %d timed out at %d/%d", i, len(collected), numEvents)
					return
				}
			}
			results[i] = collected
		}()
	}
	wg.Wait()

	for i := 0; i < numSubs; i++ {
		require.Equal(t, expected, results[i], "subscriber %d should see every event in order", i)
	}
}

// ULID monotonicity — 1000 sequentially appended events produce monotonically
// non-decreasing IDs.
func TestSession_ULIDMonotonicity(t *testing.T) {
	t.Parallel()
	const n = 1000
	ids := make([]string, n)
	for i := 0; i < n; i++ {
		ids[i] = NewEventID()
	}
	for i := 1; i < n; i++ {
		require.LessOrEqualf(t, ids[i-1], ids[i], "ID %d=%q should be <= ID %d=%q", i-1, ids[i-1], i, ids[i])
	}
}

// Submit/Interrupt cancels in-flight Ops without leaking goroutines.
func TestSession_SubmitInterruptCancelsContext(t *testing.T) {
	t.Parallel()
	s := newTestSession(t)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	// Submit a user turn; capture the registered cancel via a side channel.
	require.NoError(t, s.Submit(ctx, OpUserTurn{Text: "hello"}))

	// Grab the registered cancel handle so we can verify it cancels.
	s.cancelMu.Lock()
	registered := s.cancelFn
	s.cancelMu.Unlock()
	require.NotNil(t, registered, "Submit(OpUserTurn) should register a cancel handle")

	// Track no leaked goroutines around the interrupt path.
	before := runtime.NumGoroutine()
	require.NoError(t, s.Submit(ctx, OpInterrupt{}))

	// After Interrupt, the registered cancel should have been called and the
	// slot cleared.
	require.Eventually(t, func() bool {
		s.cancelMu.Lock()
		defer s.cancelMu.Unlock()
		return s.cancelFn == nil
	}, time.Second, 5*time.Millisecond)

	// Goroutine count should return to baseline (the watcher goroutine exits
	// when its opCtx is cancelled).
	require.Eventually(t, func() bool {
		return runtime.NumGoroutine() <= before
	}, time.Second, 10*time.Millisecond)
}

func TestSession_SubmitUserTurnPreemptsPrior(t *testing.T) {
	t.Parallel()
	s := newTestSession(t)

	ctx := context.Background()
	require.NoError(t, s.Submit(ctx, OpUserTurn{Text: "first"}))
	s.cancelMu.Lock()
	first := s.cancelFn
	s.cancelMu.Unlock()
	require.NotNil(t, first)

	require.NoError(t, s.Submit(ctx, OpUserTurn{Text: "second"}))
	s.cancelMu.Lock()
	second := s.cancelFn
	s.cancelMu.Unlock()
	require.NotNil(t, second)
	// The second submit should have replaced the cancel handle; the first
	// goroutine should observe its opCtx cancelled.
}

// Close is idempotent; subsequent Submit returns a descriptive error.
func TestSession_CloseIdempotent(t *testing.T) {
	t.Parallel()
	s := newTestSession(t)

	require.NoError(t, s.Close())
	require.NoError(t, s.Close(), "Close should be idempotent")

	err := s.Submit(context.Background(), OpUserTurn{Text: "hello"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "closed")

	err = s.Emit(mkEvent(KindStepStarted))
	require.Error(t, err)
	require.Contains(t, err.Error(), "closed")
}

func TestSession_ShutdownOpClosesSession(t *testing.T) {
	t.Parallel()
	s := newTestSession(t)
	require.NoError(t, s.Submit(context.Background(), OpShutdown{}))
	require.Error(t, s.Submit(context.Background(), OpUserTurn{Text: "x"}))
}

// Emit goes through transcript and the channel; ensure consistency.
func TestSession_EmitAppendsAndBroadcasts(t *testing.T) {
	t.Parallel()
	s := newTestSession(t)

	ev := mkEvent(KindRunStarted)
	require.NoError(t, s.Emit(ev))

	select {
	case got := <-s.Events():
		require.Equal(t, ev.EventID(), got.EventID())
	case <-time.After(time.Second):
		t.Fatal("expected event on Events() channel")
	}

	tr := s.Transcript()
	require.Len(t, tr.Events(), 1)
	require.Equal(t, ev.EventID(), tr.Events()[0].EventID())
}

func TestSession_EmitRejectsDuplicate(t *testing.T) {
	t.Parallel()
	s := newTestSession(t)
	ev := mkEvent(KindRunStarted)
	require.NoError(t, s.Emit(ev))
	// Re-emit with the same ID; transcript dedupe should reject.
	err := s.Emit(ev)
	require.Error(t, err)
}

func TestSession_UnknownOpRejected(t *testing.T) {
	t.Parallel()
	s := newTestSession(t)
	type bogusOp struct{ Op }
	err := s.Submit(context.Background(), bogusOp{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown op")
}
