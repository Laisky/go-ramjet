package http

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// asyncOutcomeTestGuard bounds how long a test waits before declaring a stuck sender.
const asyncOutcomeTestGuard = 5 * time.Second

// startSDKStyleSender mimics the Azure Speech SDK async calls, which create an
// unbuffered channel and send exactly one outcome from a goroutine after delay.
// The returned channel is closed once that send has completed.
func startSDKStyleSender(delay time.Duration, value int) (task chan int, sent chan struct{}) {
	task = make(chan int)
	sent = make(chan struct{})
	go func() {
		defer close(sent)
		time.Sleep(delay)
		task <- value
	}()
	return task, sent
}

// TestAwaitAsyncOutcomeReturnsValue verifies an in-time outcome is returned to the
// caller and is not released by the helper.
func TestAwaitAsyncOutcomeReturnsValue(t *testing.T) {
	t.Parallel()
	task, sent := startSDKStyleSender(0, 42)

	released := make(chan int, 1)
	got, err := awaitAsyncOutcome(context.Background(), task, asyncOutcomeTestGuard, func(v int) { released <- v })
	require.NoError(t, err)
	require.Equal(t, 42, got)

	<-sent
	select {
	case v := <-released:
		require.FailNowf(t, "unexpected release", "in-time outcome %d must stay owned by the caller", v)
	default:
	}
}

// TestAwaitAsyncOutcomeTimeoutReleasesLateOutcome verifies that after a timeout the
// SDK sender is not stuck on its unbuffered channel and the late outcome is released.
func TestAwaitAsyncOutcomeTimeoutReleasesLateOutcome(t *testing.T) {
	t.Parallel()
	task, sent := startSDKStyleSender(100*time.Millisecond, 7)

	released := make(chan int, 1)
	_, err := awaitAsyncOutcome(context.Background(), task, 10*time.Millisecond, func(v int) { released <- v })
	require.Error(t, err)
	require.Contains(t, err.Error(), "timeout")

	select {
	case <-sent:
	case <-time.After(asyncOutcomeTestGuard):
		require.FailNow(t, "SDK sender is blocked forever on its unbuffered outcome channel")
	}
	select {
	case v := <-released:
		require.Equal(t, 7, v)
	case <-time.After(asyncOutcomeTestGuard):
		require.FailNow(t, "late outcome was never released")
	}
}

// TestAwaitAsyncOutcomeContextCanceled verifies that a canceled request stops the
// wait promptly and still releases the outcome that arrives afterwards.
func TestAwaitAsyncOutcomeContextCanceled(t *testing.T) {
	t.Parallel()
	task, sent := startSDKStyleSender(200*time.Millisecond, 9)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	released := make(chan int, 1)
	start := time.Now()
	_, err := awaitAsyncOutcome(ctx, task, asyncOutcomeTestGuard, func(v int) { released <- v })
	require.ErrorIs(t, err, context.Canceled)
	require.Less(t, time.Since(start), asyncOutcomeTestGuard/2, "wait must not outlive the request")

	select {
	case <-sent:
	case <-time.After(asyncOutcomeTestGuard):
		require.FailNow(t, "SDK sender is blocked forever on its unbuffered outcome channel")
	}
	select {
	case v := <-released:
		require.Equal(t, 9, v)
	case <-time.After(asyncOutcomeTestGuard):
		require.FailNow(t, "late outcome was never released")
	}
}
