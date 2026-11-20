package cv

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
)

// TestPDFBrowserBudgetsDefaults pins the production budgets: rendering keeps its historical 25 seconds, and the
// separate startup budget is never shorter than the former shared startup-plus-render deadline.
func TestPDFBrowserBudgetsDefaults(t *testing.T) {
	t.Parallel()
	require.Equal(t, 25*time.Second, defaultPDFBrowserBudgets.render)
	require.Equal(t, pdfRenderTimeout, defaultPDFBrowserBudgets.render)
	require.Equal(t, pdfBrowserStartupTimeout, defaultPDFBrowserBudgets.startup)
	require.GreaterOrEqual(t, defaultPDFBrowserBudgets.startup, pdfRenderTimeout)
}

// TestStartPDFBrowserWithinKeepsBrowserAfterStartup verifies a launch that finishes in time leaves the browser
// lifetime context alive after the startup budget would have elapsed, and exposes the startup deadline to start.
func TestStartPDFBrowserWithinKeepsBrowserAfterStartup(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		browserCtx, abort := context.WithCancelCause(context.Background())
		defer abort(nil)
		started := time.Now()
		err := startPDFBrowserWithin(context.Background(), 30*time.Second, abort, func(startupCtx context.Context) error {
			deadline, ok := startupCtx.Deadline()
			require.True(t, ok)
			require.Equal(t, started.Add(30*time.Second), deadline)
			time.Sleep(20 * time.Second)
			return nil
		})
		require.NoError(t, err)
		time.Sleep(time.Minute)
		require.NoError(t, browserCtx.Err(), "a started browser must outlive its startup budget")
	})
}

// TestStartPDFBrowserWithinAbortsSlowStartup verifies an overdue launch aborts the browser with a deadline cause at
// exactly the startup budget and reports a startup-specific error that still wraps context.DeadlineExceeded.
func TestStartPDFBrowserWithinAbortsSlowStartup(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		browserCtx, abort := context.WithCancelCause(context.Background())
		defer abort(nil)
		started := time.Now()
		var observedState error
		err := startPDFBrowserWithin(context.Background(), 30*time.Second, abort, func(startupCtx context.Context) error {
			// Like chromedp, the launch only returns once its browser lifetime context is canceled.
			<-browserCtx.Done()
			observedState = startupCtx.Err()
			return errors.WithStack(browserCtx.Err())
		})
		require.Error(t, err)
		require.Equal(t, 30*time.Second, time.Since(started))
		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.Contains(t, err.Error(), "start PDF browser")
		require.Contains(t, err.Error(), "PDF browser startup exceeded 30s")
		require.ErrorIs(t, context.Cause(browserCtx), context.DeadlineExceeded)
		require.ErrorIs(t, observedState, context.DeadlineExceeded, "probe observations must report the startup deadline")
	})
}

// TestStartPDFBrowserWithinReturnsStartError verifies a launch failure is returned as is and leaves the browser
// lifetime to the caller's deferred cleanup.
func TestStartPDFBrowserWithinReturnsStartError(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		browserCtx, abort := context.WithCancelCause(context.Background())
		defer abort(nil)
		err := startPDFBrowserWithin(context.Background(), 30*time.Second, abort, func(context.Context) error {
			return errors.New("executable not found")
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "start PDF browser: executable not found")
		require.NotErrorIs(t, err, context.DeadlineExceeded)
		require.NoError(t, browserCtx.Err())
	})
}

// TestStartPDFBrowserWithinHonorsCallerCancellation verifies the caller's cancellation, not the startup budget, is
// reported when the caller ends first.
func TestStartPDFBrowserWithinHonorsCallerCancellation(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		callerCtx, cancelCaller := context.WithCancel(context.Background())
		browserCtx, abort := context.WithCancelCause(callerCtx)
		defer abort(nil)
		time.AfterFunc(5*time.Second, cancelCaller)
		err := startPDFBrowserWithin(callerCtx, 30*time.Second, abort, func(context.Context) error {
			<-browserCtx.Done()
			return errors.WithStack(browserCtx.Err())
		})
		require.Error(t, err)
		require.ErrorIs(t, err, context.Canceled)
		require.NotErrorIs(t, err, context.DeadlineExceeded)
		require.NotContains(t, err.Error(), "startup exceeded")
	})
}

// TestStartPDFBrowserWithinRejectsReadyAfterBudget verifies a launch that reports readiness only after the budget
// elapsed is still a startup failure, because the watchdog has already aborted that browser.
func TestStartPDFBrowserWithinRejectsReadyAfterBudget(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		browserCtx, abort := context.WithCancelCause(context.Background())
		defer abort(nil)
		err := startPDFBrowserWithin(context.Background(), 30*time.Second, abort, func(context.Context) error {
			time.Sleep(31 * time.Second)
			return nil
		})
		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.ErrorIs(t, context.Cause(browserCtx), context.DeadlineExceeded)
	})
}
