//go:build browser_integration

package cv

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// cvBrowserWarmUpTimeout bounds the one-time cold launch of the owned CV browser. Playwright's default launch
// timeout is three minutes; two minutes keeps the warm-up inside the CI driver's 240-second package watchdog.
const cvBrowserWarmUpTimeout = 2 * time.Minute

// cvBrowserWarmUp records the single process-wide warm-up launch shared by the owned CV browser tests.
var cvBrowserWarmUp struct {
	once     sync.Once
	err      error
	duration time.Duration
}

// logCVBrowserRuntime records fixed candidate identity and numeric version metadata without paths, environment or raw browser output.
func logCVBrowserRuntime(t *testing.T) {
	t.Helper()
	t.Logf("CV browser runtime: go=%s os=%s arch=%s", runtime.Version(), runtime.GOOS, runtime.GOARCH)
	for _, candidate := range cvBrowserCandidates() {
		executable, err := exec.LookPath(candidate)
		if err != nil {
			continue
		}
		// Only the fixed candidate basename is printed; resolved filesystem paths remain private.
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		command := exec.CommandContext(ctx, executable, "--version")
		command.Stderr = io.Discard
		output, commandErr := command.Output()
		cancel()
		version := "unavailable"
		if commandErr != nil {
			t.Logf("CV browser metadata: candidate_name=%q version=%s", filepath.Base(candidate), version)
			return
		}
		if len(output) <= 512 {
			if match := regexp.MustCompile(`([0-9]{1,5}[.]){3}[0-9]{1,5}`).Find(output); len(match) > 0 {
				version = string(match)
			}
		}
		t.Logf("CV browser metadata: candidate_name=%q version=%s", filepath.Base(candidate), version)
		return
	}
	t.Log("CV browser metadata: candidate=unavailable version=unavailable")
}

// cvBrowserCandidates returns the browser names that implicit chromedp selection probes, in lookup order.
// The Unix order follows pinned chromedp v0.20.1 execLocations; other platforms record PATH candidates only.
func cvBrowserCandidates() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{"/Applications/Chromium.app/Contents/MacOS/Chromium", "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"}
	case "windows":
		return []string{"chrome", "chrome.exe", "msedge", "msedge.exe"}
	}
	return []string{"headless_shell", "headless-shell", "chromium", "chromium-browser", "google-chrome", "google-chrome-stable", "google-chrome-beta", "google-chrome-unstable", "/usr/bin/google-chrome", "/usr/local/bin/chrome", "/snap/bin/chromium", "chrome"}
}

// cvBrowserExecutable resolves the browser the owned CV tests use: the explicit google-chrome probe mode, or the
// first implicit chromedp candidate. It takes the test and returns the resolved executable path, failing the test
// when no candidate is installed.
func cvBrowserExecutable(t *testing.T) string {
	t.Helper()
	candidates := cvBrowserCandidates()
	if os.Getenv("CV_PDF_PRINT_PROBE_BROWSER") == "google-chrome" {
		candidates = []string{"google-chrome"}
	}
	for _, candidate := range candidates {
		if executable, err := exec.LookPath(candidate); err == nil {
			return executable
		}
	}
	t.Fatal("no owned CV browser candidate is installed")
	return ""
}

// warmCVBrowser launches the owned CV browser once per test process, through the production render path, before
// any test starts its own deadline. A fresh CI runner pages a never-executed browser binary in from a cold disk on
// its first launch, which can take tens of seconds; production budgets and assertions stay unchanged for the
// timed renders. It takes the test, logs the warm-up duration, and fails the test when the warm-up failed.
func warmCVBrowser(t *testing.T) {
	t.Helper()
	executable := cvBrowserExecutable(t)
	cvBrowserWarmUp.once.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), cvBrowserWarmUpTimeout)
		defer cancel()
		// A silent observer enables the explicit owned browser path without emitting probe lines for any test.
		ctx = context.WithValue(ctx, cvPDFPrintProbeKey{}, cvPDFPrintProbeObserver(func(cvPDFPrintProbeEvent) {}))
		ctx = context.WithValue(ctx, cvPDFPrintProbeBrowserKey{}, executable)
		started := time.Now()
		_, cvBrowserWarmUp.err = renderHTMLToPDFInBrowserWithBudgets(ctx, `<!doctype html><html><body><p>Warm-up</p></body></html>`,
			pdfBrowserBudgets{startup: cvBrowserWarmUpTimeout, render: pdfRenderTimeout})
		cvBrowserWarmUp.duration = time.Since(started)
	})
	t.Logf("CV browser warm-up: duration_ms=%d", cvBrowserWarmUp.duration.Milliseconds())
	require.NoError(t, cvBrowserWarmUp.err, "warm up the owned CV browser")
}
