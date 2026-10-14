package tasks

import "github.com/Laisky/errors/v2"

// crawlerChallengeSignal is a finite matched condition, never page text or a resource URL.
type crawlerChallengeSignal string

const (
	crawlerChallengeWidgetElement     crawlerChallengeSignal = "widget_element"
	crawlerChallengeErrorElement      crawlerChallengeSignal = "error_element"
	crawlerChallengePlatformScript    crawlerChallengeSignal = "challenge_platform_script"
	crawlerChallengeOptionsAssignment crawlerChallengeSignal = "challenge_options_assignment"
)

// approvedCrawlerChallengeSignal accepts a finite signal only for the reviewed render/challenge pair.
func approvedCrawlerChallengeSignal(stage, category string, signal crawlerChallengeSignal) bool {
	if stage != "render" || category != "challenge" {
		return false
	}
	switch signal {
	case crawlerChallengeWidgetElement, crawlerChallengeErrorElement,
		crawlerChallengePlatformScript, crawlerChallengeOptionsAssignment:
		return true
	}
	return false
}

// crawlerChallengeFailure constructs the existing challenge failure with its bounded typed signal.
func crawlerChallengeFailure(signal crawlerChallengeSignal) error {
	if !approvedCrawlerChallengeSignal("render", "challenge", signal) {
		signal = ""
	}
	return errors.WithStack(&crawlerTaskError{code: "challenge", stage: "render", signal: signal})
}

// crawlerDiagnosticSignal extracts only an approved signal without rendering any error or page content.
func crawlerDiagnosticSignal(err error) crawlerChallengeSignal {
	var failure *crawlerTaskError
	if errors.As(err, &failure) && approvedCrawlerChallengeSignal(failure.stage, failure.code, failure.signal) {
		return failure.signal
	}
	return ""
}
