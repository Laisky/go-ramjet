//go:build browser_integration

package cv

import (
	"context"
	"io"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"
	"time"
)

// logCVBrowserRuntime records fixed candidate identity and numeric version metadata without paths, environment or raw browser output.
func logCVBrowserRuntime(t *testing.T) {
	t.Helper()
	t.Logf("CV browser runtime: go=%s os=%s arch=%s", runtime.Version(), runtime.GOOS, runtime.GOARCH)
	// The Unix order follows pinned chromedp v0.20.1 execLocations; other platforms record PATH candidates only.
	candidates := []string{"headless_shell", "headless-shell", "chromium", "chromium-browser", "google-chrome", "google-chrome-stable", "google-chrome-beta", "google-chrome-unstable", "/usr/bin/google-chrome", "/usr/local/bin/chrome", "/snap/bin/chromium", "chrome"}
	switch runtime.GOOS {
	case "darwin":
		candidates = []string{"/Applications/Chromium.app/Contents/MacOS/Chromium", "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"}
	case "windows":
		candidates = []string{"chrome", "chrome.exe", "msedge", "msedge.exe"}
	}
	for _, candidate := range candidates {
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
