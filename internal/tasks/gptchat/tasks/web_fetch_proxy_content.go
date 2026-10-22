package tasks

import "context"

// rejectWebFetchProxyChallenge retains the legacy HTML-only adapter check.
// Verified acquisition defers this decision to prepareCrawlerProviderContent,
// which distinguishes active HTML from quoted Markdown before returning success.
func rejectWebFetchProxyChallenge(ctx context.Context, content string) bool {
	if _, verified := ctx.Value(verifiedWebFetchClientKey{}).(rendererNetworkDependencies); verified {
		return false
	}
	return isCloudflareChallenge(content)
}
