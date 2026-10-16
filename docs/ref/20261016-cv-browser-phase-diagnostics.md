# CV browser phase evidence references

Reviewed 2026-10-16 against pinned chromedp v0.20.1 source and Go 1.27.2.

- [chromedp action.go](https://github.com/chromedp/chromedp/blob/v0.20.1/action.go): the first Run/Do initializes the browser and target; Do executes actions in order and stops at the first error. An empty Do can therefore label initialization separately from subsequent actions.
- [chromedp context documentation](https://github.com/chromedp/chromedp/blob/v0.20.1/chromedp.go): the first call's context owns browser lifetime. The CV diagnostic change retains the same context and existing 25-second total render deadline for startup, navigation, body readiness, fonts and printing; it does not add an untimed startup or enlarge the timeout.
- [chromedp fresh-navigation readiness regression](https://github.com/chromedp/chromedp/blob/v0.20.1/query_test.go#L1500): the dependency tests readiness immediately after fresh data-URL navigation. This establishes relevant coverage, not the phase or cause of the observed CI timeout.
- [chromedp executable discovery](https://github.com/chromedp/chromedp/blob/v0.20.1/allocate.go): test metadata follows the pinned Unix candidate order and reports fixed candidate basename, numeric browser version and Go runtime only. Windows PATH candidates are partial metadata and do not attest the allocator's absolute-path fallbacks.

The two failing CV CI tests currently share the broad navigation error label. Local accepted, candidate and combined browser gates passed. Startup, navigation and readiness remain competing explanations; none is established as the cause. Fixed phase labels improve the next failure's evidence and preserve wrapped cancellation/deadline identity. No browser flags, font/PDF logic or acceptance assertions are relaxed.
