//go:build browser_integration

package cv

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestBrowserPDFStartupKeepsRenderBudget delays the browser launch past the render budget, as a cold page cache on a
// fresh CI runner does, and verifies startup is limited by its own budget instead of consuming the render budget.
func TestBrowserPDFStartupKeepsRenderBudget(t *testing.T) {
	t.Parallel()
	warmCVBrowser(t)
	const (
		launchDelay  = 6 * time.Second
		renderBudget = 5 * time.Second
	)
	launcher := filepath.Join(t.TempDir(), "slow-browser")
	script := "#!/bin/sh\nsleep " + strconv.Itoa(int(launchDelay/time.Second)) + "\nexec " + strconv.Quote(cvBrowserExecutable(t)) + " \"$@\"\n"
	require.NoError(t, os.WriteFile(launcher, []byte(script), 0o700))

	var mu sync.Mutex
	phases := map[string]cvPDFPrintProbeEvent{}
	observer := cvPDFPrintProbeObserver(func(event cvPDFPrintProbeEvent) {
		mu.Lock()
		defer mu.Unlock()
		phases[event.Phase+"/"+event.Outcome] = event
	})
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	ctx = context.WithValue(context.WithValue(ctx, cvPDFPrintProbeKey{}, observer), cvPDFPrintProbeBrowserKey{}, launcher)

	// The production startup budget absorbs the delay; the render budget is deliberately shorter than the delay.
	payload, err := renderHTMLToPDFInBrowserWithBudgets(ctx, `<!doctype html><html><body><p>Budget</p></body></html>`,
		pdfBrowserBudgets{startup: pdfBrowserStartupTimeout, render: renderBudget})
	require.NoError(t, err)
	require.True(t, bytes.HasPrefix(payload, []byte("%PDF-")))

	mu.Lock()
	defer mu.Unlock()
	startup, ok := phases["startup/success"]
	require.True(t, ok, "startup must succeed")
	require.GreaterOrEqual(t, startup.PhaseMS, launchDelay.Milliseconds())
	navigation, ok := phases["navigation/begin"]
	require.True(t, ok, "navigation must begin")
	// Rendering starts with most of its budget even though startup alone outlasted that budget; only the bounded
	// one-second browser version observation may precede navigation.
	require.Greater(t, navigation.RemainingMS, (renderBudget / 2).Milliseconds())
	require.Contains(t, phases, "print/success")
}
