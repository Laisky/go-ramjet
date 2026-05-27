package loop

import (
	"time"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/agentx/hook"
)

// Caps mirrors hook.Caps but uses time.Duration for the wall-clock budget so
// callers don't have to convert from seconds. We keep an independent struct
// here (rather than aliasing hook.Caps) because the loop package owns the
// canonical defaults; the hook package's Caps is the snapshot view shipped to
// session-start hooks and is intentionally cycle-free.
type Caps struct {
	// MaxIterations bounds the number of ReAct rounds. Default 20.
	MaxIterations int
	// MaxToolCalls bounds the total tool calls across the run. Default 40.
	MaxToolCalls int
	// MaxParallelToolCalls bounds the fan-out per round. Default 8.
	// Setting to 1 forces sequential execution.
	MaxParallelToolCalls int
	// ErrorBudget bounds total tool errors. Default 6.
	ErrorBudget int
	// CircuitBreakerRepeats trips after N identical (name, normalized_args)
	// in a row. Default 3.
	CircuitBreakerRepeats int
	// WallClock bounds total run wall-clock time. Default 8m.
	WallClock time.Duration
}

// DefaultCaps returns the 2026 conservative defaults from proposal §4.2.
func DefaultCaps() Caps {
	return Caps{
		MaxIterations:         20,
		MaxToolCalls:          40,
		MaxParallelToolCalls:  8,
		ErrorBudget:           6,
		CircuitBreakerRepeats: 3,
		WallClock:             8 * time.Minute,
	}
}

// toHookCaps projects loop.Caps onto the hook-friendly view shipped via
// SessionStartEvent. WallClock is converted to whole seconds (rounded down)
// so the hook package can stay cycle-free.
func (c Caps) toHookCaps() hook.Caps {
	return hook.Caps{
		MaxIterations:         c.MaxIterations,
		MaxToolCalls:          c.MaxToolCalls,
		MaxParallelToolCalls:  c.MaxParallelToolCalls,
		ErrorBudget:           c.ErrorBudget,
		CircuitBreakerRepeats: c.CircuitBreakerRepeats,
		WallClockSeconds:      int(c.WallClock / time.Second),
	}
}

// withDefaults fills zero-valued fields from DefaultCaps. Used by the loop
// driver so callers can pass a sparse Caps and still get reasonable behavior.
func (c Caps) withDefaults() Caps {
	d := DefaultCaps()
	if c.MaxIterations <= 0 {
		c.MaxIterations = d.MaxIterations
	}
	if c.MaxToolCalls <= 0 {
		c.MaxToolCalls = d.MaxToolCalls
	}
	if c.MaxParallelToolCalls <= 0 {
		c.MaxParallelToolCalls = d.MaxParallelToolCalls
	}
	if c.ErrorBudget <= 0 {
		c.ErrorBudget = d.ErrorBudget
	}
	if c.CircuitBreakerRepeats <= 0 {
		c.CircuitBreakerRepeats = d.CircuitBreakerRepeats
	}
	if c.WallClock <= 0 {
		c.WallClock = d.WallClock
	}
	return c
}
