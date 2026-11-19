package http

import (
	"context"
	"net/http"
	"time"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
	"github.com/Laisky/go-ramjet/library/egress"
	"github.com/Laisky/go-ramjet/library/openai"
)

const (
	// tenantUpstreamTimeout bounds one whole exchange with a destination a
	// caller selected; it matches the server client's budget.
	tenantUpstreamTimeout = 30 * time.Minute
	// tenantUpstreamHeaderTimeout bounds the wait for response headers. Image
	// and non-streaming chat models may compute for minutes before answering.
	tenantUpstreamHeaderTimeout = 10 * time.Minute
)

// tenantEgressDeps supplies the network seams of caller-selected egress.
// Zero values use the system resolver and dialer; tests replace them.
var tenantEgressDeps egress.Dependencies

// tenantEgressClient returns a client for destinations a caller selected. Every
// hop, including redirects, must pass the shared public-destination policy and
// connects only to the addresses admitted for it. It deliberately ignores the
// operator proxy, which would re-resolve the destination outside the pin.
func tenantEgressClient() *http.Client {
	return egress.NewHTTPClient(tenantUpstreamTimeout, tenantEgressDeps,
		egress.WithResponseHeaderTimeout(tenantUpstreamHeaderTimeout))
}

// upstreamHTTPClient returns the client for requests to user's model upstream
// and to URLs that upstream returns. An API base the caller selected through
// X-Laisky-Api-Base gets tenantEgressClient. An operator-configured upstream
// keeps the server client, which may legitimately reach private networks and
// use the configured proxy.
func upstreamHTTPClient(user *config.UserConfig) *http.Client {
	if user != nil && user.APIBaseUserSelected {
		return tenantEgressClient()
	}
	return httpcli
}

// userOneshotChat runs openai.OneshotChat against user's model upstream,
// admitting the destination when the caller selected it.
//
// Parameters:
//   - ctx: Request-scoped context carrying the logger and cancellation.
//   - user: Caller whose API base and model token are used.
//   - model, systemPrompt, userPrompt: Passed through to openai.OneshotChat.
//
// Returns the assistant answer or a wrapped error.
func userOneshotChat(ctx context.Context, user *config.UserConfig, model, systemPrompt, userPrompt string) (string, error) {
	if user == nil {
		return "", errors.New("nil user")
	}
	var opts []openai.OneshotOption
	if user.APIBaseUserSelected {
		opts = append(opts, openai.WithHTTPClient(tenantEgressClient()))
	}
	answer, err := openai.OneshotChat(ctx, user.APIBase, user.OpenaiToken, model, systemPrompt, userPrompt, opts...)
	if err != nil {
		return "", errors.Wrap(err, "chat with user upstream")
	}
	return answer, nil
}
