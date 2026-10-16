package cv

import (
	"context"
	"regexp"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/chromedp/cdproto/browser"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/chromedp"
)

// cvPDFPrintProbeKey scopes synthetic phase observations to an owned test context.
type cvPDFPrintProbeKey struct{}

// cvPDFPrintProbeBrowserKey carries an explicitly selected owned browser path only in the synthetic probe.
type cvPDFPrintProbeBrowserKey struct{}

// cvPDFPrintProbeEvent carries only fixed phase/outcome identities and bounded timing/version metadata.
type cvPDFPrintProbeEvent struct {
	Phase, Outcome, ContextState, BrowserVersion string
	PhaseMS, TotalMS, RemainingMS                int64
}

// cvPDFPrintProbeObserver receives an owned synthetic observation without user content, paths or error text.
type cvPDFPrintProbeObserver func(cvPDFPrintProbeEvent)

// cvPDFPrintProbeFromContext finds an explicitly enabled test observer without reading environment or configuration.
func cvPDFPrintProbeFromContext(ctx context.Context) cvPDFPrintProbeObserver {
	observer, _ := ctx.Value(cvPDFPrintProbeKey{}).(cvPDFPrintProbeObserver)
	return observer
}

// emitCVPDFPrintProbe reports a fixed event and clamps timings to zero through thirty seconds.
func emitCVPDFPrintProbe(ctx context.Context, observer cvPDFPrintProbeObserver, phase, outcome string, started, phaseStarted time.Time, version string) {
	if observer == nil {
		return
	}
	switch phase {
	case "startup", "browser_version", "navigation", "body_ready", "fonts", "print":
	default:
		return
	}
	switch outcome {
	case "begin", "success", "failure":
	default:
		return
	}
	now := time.Now()
	phaseMS, totalMS := now.Sub(phaseStarted).Milliseconds(), now.Sub(started).Milliseconds()
	phaseMS, totalMS = min(max(phaseMS, 0), 30000), min(max(totalMS, 0), 30000)
	remainingMS := int64(0)
	if deadline, ok := ctx.Deadline(); ok {
		remainingMS = min(max(deadline.Sub(now).Milliseconds(), 0), 30000)
	}
	state := "active"
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		state = "deadline_exceeded"
	} else if errors.Is(ctx.Err(), context.Canceled) {
		state = "canceled"
	}
	if version != "unavailable" && (len(version) > 23 || !regexp.MustCompile(`^([0-9]{1,5}[.]){3}[0-9]{1,5}$`).MatchString(version)) {
		version = "unavailable"
	}
	observer(cvPDFPrintProbeEvent{Phase: phase, Outcome: outcome, PhaseMS: phaseMS, TotalMS: totalMS, RemainingMS: remainingMS, ContextState: state, BrowserVersion: version})
}

// observeCVPDFBrowserVersion obtains actual browser identity with at most one second of the existing render budget.
func observeCVPDFBrowserVersion(ctx context.Context, observer cvPDFPrintProbeObserver, started time.Time) {
	observeCVPDFBrowserVersionUsing(ctx, observer, started, func(metadataCtx context.Context) (string, error) {
		result, err := chromedp.CallBrowser(metadataCtx, browser.GetVersion, cdp.Empty{})
		if err != nil {
			return "", err
		}
		return result.Product, nil
	})
}

// observeCVPDFBrowserVersionUsing bounds the owned metadata lookup while observations retain the parent render context state.
func observeCVPDFBrowserVersionUsing(ctx context.Context, observer cvPDFPrintProbeObserver, started time.Time, lookup func(context.Context) (string, error)) {
	if observer == nil {
		return
	}
	phaseStarted := time.Now()
	emitCVPDFPrintProbe(ctx, observer, "browser_version", "begin", started, phaseStarted, "unavailable")
	metadataCtx, cancelMetadata := context.WithTimeout(ctx, time.Second)
	defer cancelMetadata()
	product, err := lookup(metadataCtx)
	if err != nil {
		emitCVPDFPrintProbe(ctx, observer, "browser_version", "failure", started, phaseStarted, "unavailable")
		return
	}
	emitCVPDFPrintProbe(ctx, observer, "browser_version", "success", started, phaseStarted, cvPDFPrintProbeNumericVersion(product))
}

// cvPDFPrintProbeNumericVersion accepts a fixed browser product and bounded numeric version without retaining raw metadata.
func cvPDFPrintProbeNumericVersion(product string) string {
	if len(product) > 128 {
		return "unavailable"
	}
	matches := regexp.MustCompile(`^(Chrome|HeadlessChrome|Chromium)/(([0-9]{1,5}[.]){3}[0-9]{1,5})$`).FindStringSubmatch(product)
	if len(matches) > 2 {
		return matches[2]
	}
	return "unavailable"
}
