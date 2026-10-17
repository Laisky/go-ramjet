package cv

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
)

// TestCVPDFPrintProbeVersionPrivacy permits only reviewed product identities and bounded numeric versions.
func TestCVPDFPrintProbeVersionPrivacy(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ product, want string }{
		{"Chrome/155.0.8059.79", "155.0.8059.79"},
		{"HeadlessChrome/155.0.8059.79", "155.0.8059.79"},
		{"Chromium/155.0.8059.79", "155.0.8059.79"},
		{"private-product/155.0.8059.79", "unavailable"},
		{"Chrome/155.0.8059.79 private-version-canary", "unavailable"},
		{"Chrome/155.0.8059.79\nprivate-version-canary", "unavailable"},
		{"Chrome/155.0.8059", "unavailable"},
		{"Chrome/155000.0.8059.79", "unavailable"},
		{strings.Repeat("private-version-canary", 20), "unavailable"},
	} {
		require.Equal(t, tc.want, cvPDFPrintProbeNumericVersion(tc.product))
	}
}

// TestCVPDFPrintProbeFiniteEvents rejects unreviewed fields and bounds serialized timing, state and version observations.
func TestCVPDFPrintProbeFiniteEvents(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	var observed []cvPDFPrintProbeEvent
	observer := cvPDFPrintProbeObserver(func(event cvPDFPrintProbeEvent) { observed = append(observed, event) })
	emitCVPDFPrintProbe(ctx, observer, "private-phase-canary", "success", time.Now(), time.Now(), "")
	emitCVPDFPrintProbe(ctx, observer, "print", "private-outcome-canary", time.Now(), time.Now(), "")
	require.Empty(t, observed)
	emitCVPDFPrintProbe(ctx, observer, "print", "success", time.Now().Add(-time.Hour), time.Now().Add(time.Hour), "private-version-canary")
	require.Len(t, observed, 1)
	event := observed[0]
	require.Equal(t, int64(0), event.PhaseMS)
	require.Equal(t, int64(30000), event.TotalMS)
	require.Equal(t, int64(30000), event.RemainingMS)
	require.Equal(t, "active", event.ContextState)
	require.Equal(t, "unavailable", event.BrowserVersion)
	encoded, err := json.Marshal(event)
	require.NoError(t, err)
	for _, private := range []string{"private-phase-canary", "private-outcome-canary", "private-version-canary", "error", "body", "url"} {
		require.NotContains(t, string(encoded), private)
	}
}

// TestCVPDFPrintProbeMetadataDeadline records parent render state and never leaks metadata errors or extends its budget.
func TestCVPDFPrintProbeMetadataDeadline(t *testing.T) {
	t.Parallel()
	for _, timeout := range []time.Duration{time.Second / 4, 25 * time.Second} {
		ctx, cancel := context.WithTimeout(t.Context(), timeout)
		var observed []cvPDFPrintProbeEvent
		observer := cvPDFPrintProbeObserver(func(event cvPDFPrintProbeEvent) { observed = append(observed, event) })
		calls := 0
		observeCVPDFBrowserVersionUsing(ctx, observer, time.Now(), func(metadataCtx context.Context) (string, error) {
			calls++
			deadline, ok := metadataCtx.Deadline()
			require.True(t, ok)
			parentDeadline, ok := ctx.Deadline()
			require.True(t, ok)
			require.LessOrEqual(t, time.Until(deadline), time.Second)
			require.False(t, deadline.After(parentDeadline))
			return "private-body-canary", errors.New("private-error-canary https://private.invalid/?key=private-key-canary")
		})
		require.Equal(t, 1, calls)
		require.NoError(t, ctx.Err(), "metadata failure does not cancel the render context")
		require.Len(t, observed, 2)
		require.Equal(t, "begin", observed[0].Outcome)
		require.Equal(t, "failure", observed[1].Outcome)
		require.Equal(t, "active", observed[1].ContextState)
		require.Equal(t, "unavailable", observed[1].BrowserVersion)
		require.Positive(t, observed[1].RemainingMS)
		encoded, err := json.Marshal(observed)
		require.NoError(t, err)
		for _, private := range []string{"private-body-canary", "private-error-canary", "private.invalid", "private-key-canary"} {
			require.NotContains(t, string(encoded), private)
		}
		cancel()
	}
	calls := 0
	observeCVPDFBrowserVersionUsing(t.Context(), nil, time.Now(), func(context.Context) (string, error) { calls++; return "Chrome/155.0.8059.79", nil })
	require.Zero(t, calls, "ordinary rendering must not issue the probe metadata lookup")
}

// TestCVPDFPrintProbeEndedContextStates preserves finite canceled and deadline states on the observed render context.
func TestCVPDFPrintProbeEndedContextStates(t *testing.T) {
	t.Parallel()
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	expired, stop := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
	defer stop()
	for _, tc := range []struct {
		ctx   context.Context
		state string
	}{{canceled, "canceled"}, {expired, "deadline_exceeded"}} {
		var observed []cvPDFPrintProbeEvent
		observer := cvPDFPrintProbeObserver(func(event cvPDFPrintProbeEvent) { observed = append(observed, event) })
		emitCVPDFPrintProbe(tc.ctx, observer, "print", "failure", time.Now(), time.Now(), "155.0.8059.79")
		require.Len(t, observed, 1)
		require.Equal(t, tc.state, observed[0].ContextState)
		if tc.state == "deadline_exceeded" {
			require.Zero(t, observed[0].RemainingMS)
		} else {
			require.GreaterOrEqual(t, observed[0].RemainingMS, int64(0))
			require.LessOrEqual(t, observed[0].RemainingMS, int64(30000))
		}
		require.Equal(t, "155.0.8059.79", observed[0].BrowserVersion)
	}
}

// TestCVPDFPrintProbeEmittedVersionGrammar normalizes empty and unreviewed values before any observer receives an event.
func TestCVPDFPrintProbeEmittedVersionGrammar(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ input, want string }{
		{"", "unavailable"},
		{"unknown", "unavailable"},
		{"private-version-canary https://private.invalid/?key=private-key-canary", "unavailable"},
		{"Chrome/155.0.8059.79", "unavailable"},
		{"155.0.8059.79\nprivate-version-canary", "unavailable"},
		{"155000.0.8059.79", "unavailable"},
		{"155.0.8059.79", "155.0.8059.79"},
		{"unavailable", "unavailable"},
	} {
		var observed []cvPDFPrintProbeEvent
		observer := cvPDFPrintProbeObserver(func(event cvPDFPrintProbeEvent) { observed = append(observed, event) })
		emitCVPDFPrintProbe(t.Context(), observer, "print", "begin", time.Now(), time.Now(), tc.input)
		require.Len(t, observed, 1)
		require.NotEmpty(t, observed[0].BrowserVersion)
		require.Equal(t, tc.want, observed[0].BrowserVersion)
		encoded, err := json.Marshal(observed)
		require.NoError(t, err)
		for _, private := range []string{"private-version-canary", "private.invalid", "private-key-canary", "unknown", "Chrome/"} {
			require.NotContains(t, string(encoded), private)
		}
	}
}
