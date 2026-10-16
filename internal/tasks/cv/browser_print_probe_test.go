//go:build browser_integration

package cv

import (
	"context"
	"flag"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"testing"
)

// withCVPDFPrintProbe enables fixed synthetic phase diagnostics only for the existing owned CV browser tests.
func withCVPDFPrintProbe(ctx context.Context, t *testing.T) context.Context {
	t.Helper()
	mode := os.Getenv("CV_PDF_PRINT_PROBE_BROWSER")
	if mode == "" {
		mode = "auto"
	}
	switch mode {
	case "auto":
	case "google-chrome":
		executable, err := exec.LookPath("google-chrome")
		if err != nil {
			t.Fatal("explicit owned google-chrome candidate unavailable")
		}
		ctx = context.WithValue(ctx, cvPDFPrintProbeBrowserKey{}, executable)
	default:
		t.Fatal("unsupported synthetic browser selection")
	}
	t.Logf("CV_PRINT_PROBE browser_mode=%s", mode)
	parallelism := 0
	if option := flag.Lookup("test.parallel"); option != nil {
		if parsed, err := strconv.Atoi(option.Value.String()); err != nil {
			parallelism = 0
		} else {
			parallelism = min(max(parsed, 0), 65536)
		}
	}
	t.Logf("CV_PRINT_PROBE runtime_parallelism=%d test_parallelism=%d cpu_count=%d", runtime.GOMAXPROCS(0), parallelism, runtime.NumCPU())
	observer := cvPDFPrintProbeObserver(func(event cvPDFPrintProbeEvent) {
		t.Logf("CV_PRINT_PROBE phase=%s outcome=%s phase_ms=%d total_ms=%d remaining_ms=%d context=%s browser_version=%s", event.Phase, event.Outcome, event.PhaseMS, event.TotalMS, event.RemainingMS, event.ContextState, event.BrowserVersion)
	})
	return context.WithValue(ctx, cvPDFPrintProbeKey{}, observer)
}
