//go:build linux

package cv

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// fakeBrowserLauncher is a browser executable stand-in that records its process ID and arguments before it
// replaces itself with a target program, so a test can check what the render path left behind.
type fakeBrowserLauncher struct {
	path string
	dir  string
}

// shellQuote quotes value as one single-quoted POSIX shell word. It takes the raw value and returns the quoted word.
func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

// newFakeBrowserLauncher writes a launcher script into a test directory. It takes the test and the command the
// launcher execs after recording its process ID and arguments; exec keeps the recorded process ID. It returns the
// launcher.
func newFakeBrowserLauncher(t *testing.T, execCommand string) *fakeBrowserLauncher {
	t.Helper()
	dir := t.TempDir()
	launcher := &fakeBrowserLauncher{path: filepath.Join(dir, "browser"), dir: dir}
	args, pid := shellQuote(filepath.Join(dir, "args")), shellQuote(filepath.Join(dir, "pid"))
	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$@\" > " + args + ".tmp && mv " + args + ".tmp " + args + "\n" +
		"printf '%s\\n' \"$$\" > " + pid + ".tmp && mv " + pid + ".tmp " + pid + "\n" +
		"exec " + execCommand + "\n"
	require.NoError(t, os.WriteFile(launcher.path, []byte(script), 0o700))
	return launcher
}

// context returns ctx configured to launch this browser through a silent probe observer, the only way the render
// path accepts an explicit executable.
func (l *fakeBrowserLauncher) context(ctx context.Context) context.Context {
	ctx = context.WithValue(ctx, cvPDFPrintProbeKey{}, cvPDFPrintProbeObserver(func(cvPDFPrintProbeEvent) {}))
	return context.WithValue(ctx, cvPDFPrintProbeBrowserKey{}, l.path)
}

// started reports whether the launcher process has recorded its process ID.
func (l *fakeBrowserLauncher) started() bool {
	if _, err := os.Stat(filepath.Join(l.dir, "pid")); err != nil {
		return false // A missing file only means the launcher has not started yet.
	}
	return true
}

// requireReaped verifies the launched browser is gone: its process has exited and been waited for, no process
// still runs with its user data directory, and chromedp removed that temporary directory.
func (l *fakeBrowserLauncher) requireReaped(t *testing.T) {
	t.Helper()
	pid, err := os.ReadFile(filepath.Join(l.dir, "pid"))
	require.NoError(t, err, "the browser process must have started before the render ended")
	_, err = os.Stat(filepath.Join("/proc", strings.TrimSpace(string(pid))))
	require.ErrorIs(t, err, os.ErrNotExist, "the browser process must be killed and reaped before the render returns")

	args, err := os.ReadFile(filepath.Join(l.dir, "args"))
	require.NoError(t, err)
	var userDataDir string
	for arg := range strings.SplitSeq(string(args), "\n") {
		if value, ok := strings.CutPrefix(arg, "--user-data-dir="); ok {
			userDataDir = value
		}
	}
	require.NotEmpty(t, userDataDir, "chromedp must have given the browser a temporary user data directory")
	require.Zero(t, processesUsingArg(t, "--user-data-dir="+userDataDir), "no browser child may outlive the render")
	_, err = os.Stat(userDataDir)
	require.ErrorIs(t, err, os.ErrNotExist, "the temporary user data directory must be removed before the render returns")
}

// processesUsingArg counts running processes whose command line contains arg. It takes the test and the exact
// argument and returns the count.
func processesUsingArg(t *testing.T, arg string) int {
	t.Helper()
	entries, err := os.ReadDir("/proc")
	require.NoError(t, err)
	count := 0
	for _, entry := range entries {
		cmdline, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "cmdline"))
		if err != nil {
			continue // Not a process directory, or the process exited while scanning.
		}
		for field := range bytes.SplitSeq(cmdline, []byte{0}) {
			if string(field) == arg {
				count++
				break
			}
		}
	}
	return count
}

// chromedpGoroutines counts live goroutines whose stack runs chromedp code.
func chromedpGoroutines() int {
	buf := make([]byte, 1<<20)
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			buf = buf[:n]
			break
		}
		buf = make([]byte, 2*len(buf))
	}
	count := 0
	for stack := range bytes.SplitSeq(buf, []byte("\n\n")) {
		if bytes.Contains(stack, []byte("github.com/chromedp/chromedp")) {
			count++
		}
	}
	return count
}

// requireNoChromedpGoroutines waits briefly for chromedp goroutines to finish and fails when any remain.
func requireNoChromedpGoroutines(t *testing.T) {
	t.Helper()
	require.Eventually(t, func() bool { return chromedpGoroutines() == 0 }, 5*time.Second, 20*time.Millisecond,
		"an aborted browser launch must not leak chromedp goroutines")
}

// TestPDFBrowserStartupTimeoutReapsBrowser verifies a browser launch that outlasts the startup budget is killed,
// waited for and cleaned up before the render returns, without leaking chromedp goroutines.
//
// It does not run in parallel because it counts process-wide chromedp goroutines.
func TestPDFBrowserStartupTimeoutReapsBrowser(t *testing.T) {
	launcher := newFakeBrowserLauncher(t, "sleep 300")
	const startup = 2 * time.Second
	started := time.Now()
	payload, err := renderHTMLToPDFInBrowserWithBudgets(launcher.context(t.Context()),
		`<!doctype html><p>never rendered</p>`, pdfBrowserBudgets{startup: startup, render: time.Second})
	elapsed := time.Since(started)

	require.Nil(t, payload)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.ErrorContains(t, err, "PDF browser startup exceeded 2s")
	require.GreaterOrEqual(t, elapsed, startup)
	// chromedp waits for the killed browser and removes its directory; that cleanup is quick, not another budget.
	require.Less(t, elapsed, startup+10*time.Second)
	launcher.requireReaped(t)
	requireNoChromedpGoroutines(t)
}

// TestPDFBrowserStartupCancelReapsBrowser verifies a caller that leaves during a browser launch ends the launch
// with its cancellation and that the browser is killed, waited for and cleaned up before the render returns.
//
// It does not run in parallel because it counts process-wide chromedp goroutines.
func TestPDFBrowserStartupCancelReapsBrowser(t *testing.T) {
	launcher := newFakeBrowserLauncher(t, "sleep 300")
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go func() {
		deadline := time.Now().Add(20 * time.Second)
		for !launcher.started() && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}
		cancel()
	}()

	payload, err := renderHTMLToPDFInBrowserWithBudgets(launcher.context(ctx),
		`<!doctype html><p>never rendered</p>`, pdfBrowserBudgets{startup: time.Minute, render: time.Second})

	require.Nil(t, payload)
	require.ErrorIs(t, err, context.Canceled)
	require.NotErrorIs(t, err, context.DeadlineExceeded, "a caller cancellation is not a startup timeout")
	launcher.requireReaped(t)
	requireNoChromedpGoroutines(t)
}
