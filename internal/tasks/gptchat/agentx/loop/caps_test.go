package loop

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDefaultCaps_MatchesProposal(t *testing.T) {
	t.Parallel()
	c := DefaultCaps()
	require.Equal(t, 20, c.MaxIterations)
	require.Equal(t, 40, c.MaxToolCalls)
	require.Equal(t, 8, c.MaxParallelToolCalls)
	require.Equal(t, 6, c.ErrorBudget)
	require.Equal(t, 3, c.CircuitBreakerRepeats)
	require.Equal(t, 8*time.Minute, c.WallClock)
}

func TestCaps_WithDefaults_FillsZeroes(t *testing.T) {
	t.Parallel()
	c := Caps{MaxIterations: 5}.withDefaults()
	require.Equal(t, 5, c.MaxIterations)
	d := DefaultCaps()
	require.Equal(t, d.MaxToolCalls, c.MaxToolCalls)
	require.Equal(t, d.MaxParallelToolCalls, c.MaxParallelToolCalls)
	require.Equal(t, d.ErrorBudget, c.ErrorBudget)
	require.Equal(t, d.CircuitBreakerRepeats, c.CircuitBreakerRepeats)
	require.Equal(t, d.WallClock, c.WallClock)
}

func TestCaps_ToHookCaps(t *testing.T) {
	t.Parallel()
	c := DefaultCaps()
	hc := c.toHookCaps()
	require.Equal(t, c.MaxIterations, hc.MaxIterations)
	require.Equal(t, c.MaxToolCalls, hc.MaxToolCalls)
	require.Equal(t, c.MaxParallelToolCalls, hc.MaxParallelToolCalls)
	require.Equal(t, c.ErrorBudget, hc.ErrorBudget)
	require.Equal(t, c.CircuitBreakerRepeats, hc.CircuitBreakerRepeats)
	require.Equal(t, int(c.WallClock/time.Second), hc.WallClockSeconds)
}
