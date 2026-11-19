//go:build browser_integration && linux

package cv

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestBrowserPDFAbortReapsBrowser verifies a real Chrome stopped in the middle of a render, by the render budget or
// by the caller leaving, is killed together with its child processes, and its temporary profile is removed, before
// the render returns.
func TestBrowserPDFAbortReapsBrowser(t *testing.T) {
	t.Parallel()
	warmCVBrowser(t)
	// The page replaces its font promise with one that never settles, so rendering stays in the fonts phase.
	const hangingFonts = `<!doctype html><html><body><p>Hang</p><script>
Object.defineProperty(document.fonts, 'ready', {value: new Promise(() => {})});
</script></body></html>`

	for _, tc := range []struct {
		name          string
		render        time.Duration
		cancelAtFonts bool
		want          error
	}{
		{name: "render budget elapses", render: 2 * time.Second, want: context.DeadlineExceeded},
		{name: "caller leaves", render: time.Minute, cancelAtFonts: true, want: context.Canceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			launcher := newFakeBrowserLauncher(t, shellQuote(cvBrowserExecutable(t))+` "$@"`)
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
			defer cancel()
			var reachedFonts atomic.Bool
			observer := cvPDFPrintProbeObserver(func(event cvPDFPrintProbeEvent) {
				if event.Phase == "fonts" && event.Outcome == "begin" {
					reachedFonts.Store(true)
					if tc.cancelAtFonts {
						cancel()
					}
				}
			})
			ctx = context.WithValue(context.WithValue(ctx, cvPDFPrintProbeKey{}, observer), cvPDFPrintProbeBrowserKey{}, launcher.path)

			payload, err := renderHTMLToPDFInBrowserWithBudgets(ctx, hangingFonts,
				pdfBrowserBudgets{startup: pdfBrowserStartupTimeout, render: tc.render})
			require.Nil(t, payload)
			require.ErrorIs(t, err, tc.want)
			require.True(t, reachedFonts.Load(), "the browser must start and reach the fonts phase before it is stopped")
			launcher.requireReaped(t)
		})
	}
}
