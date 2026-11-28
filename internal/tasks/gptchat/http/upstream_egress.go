package http

import (
	"context"
	"net/http"
	"net/url"
	"sync/atomic"
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
	// tenantUpstreamHeaderTimeout bounds the wait for response headers. It
	// equals the whole-exchange budget, as with the server client, because
	// non-streaming model calls may compute for many minutes before answering.
	tenantUpstreamHeaderTimeout = tenantUpstreamTimeout
)

var (
	// tenantEgressDeps supplies the network seams of caller-selected egress.
	// Zero values use the system resolver and dialer; tests replace them.
	tenantEgressDeps egress.Dependencies
	// serverProxy is the outbound proxy of the server client httpcli, set by
	// SetupHTTPCli; nil means the deployment has none.
	serverProxy atomic.Pointer[url.URL]
)

// tenantEgressClient returns a client for destinations a caller selected that
// replaces a request previously sent with the server client. Every hop,
// including redirects, must pass the shared public-destination policy.
//
// Without an operator proxy each hop connects only to the addresses admitted
// for it. With one, admitted hops leave through that same proxy, as they did
// before; the proxy resolves the name again, which leaves a small DNS-rebinding
// window in proxied deployments only.
func tenantEgressClient() *http.Client {
	proxy := serverProxy.Load()
	if proxy == nil {
		return pinnedTenantClient()
	}
	client, err := egress.NewProxiedHTTPClient(tenantUpstreamTimeout, proxy, tenantEgressDeps, tenantClientOptions()...)
	if err != nil {
		// SetupHTTPCli validates the proxy, so this is unreachable; fail
		// closed rather than bypass the proxy.
		return &http.Client{Transport: refusingTransport{err: errors.Wrap(err, "build proxied egress client")}}
	}
	return client
}

// pinnedTenantClient returns a client for destinations a caller selected that
// connects directly, admitting and pinning every hop. It serves requests that
// never used the operator proxy, such as one-shot chat.
func pinnedTenantClient() *http.Client {
	return egress.NewHTTPClient(tenantUpstreamTimeout, tenantEgressDeps, tenantClientOptions()...)
}

// tenantClientOptions returns the settings shared by every caller-selected
// client. They match the server client: the same response-header budget, and
// gzip negotiated and decoded transparently, so callers that relied on the
// server client's decoding still read plain bodies. Unlike the server client,
// every body is bounded (1 MiB outside 2xx), because a caller-selected
// upstream chooses how much its answers make this server read.
func tenantClientOptions() []egress.ClientOption {
	return []egress.ClientOption{
		egress.WithResponseHeaderTimeout(tenantUpstreamHeaderTimeout),
		egress.WithContentDecoding(),
	}
}

// refusingTransport fails every request with err.
type refusingTransport struct{ err error }

// RoundTrip closes request's body and returns the transport's error.
func (t refusingTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.Body != nil {
		if closeErr := request.Body.Close(); closeErr != nil {
			return nil, errors.Join(t.err, closeErr)
		}
	}
	return nil, t.err
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
	if model == "" {
		model = openai.DefaultChatModel
	}
	if err := consumeInferenceCall(ctx, user, model); err != nil {
		return "", errors.Wrap(err, "admit one-shot request")
	}
	var opts []openai.OneshotOption
	if user.IsFree {
		if CountTextTokens(systemPrompt)+CountTextTokens(userPrompt) > tokenQuotaLimit {
			return "", errors.New("Free-tier auxiliary prompt exceeds its input limit")
		}
		opts = append(opts, openai.WithHTTPClient(mediaInferenceClient(user)), openai.WithMaxTokens(1024))
	}
	if user.APIBaseUserSelected {
		// Custom one-shot inference remains direct with pinned egress.
		opts = append(opts, openai.WithHTTPClient(pinnedTenantClient()))
	} else if !user.IsFree {
		opts = append(opts, openai.WithHTTPClient(httpcli))
	}
	answer, err := openai.OneshotChat(ctx, user.APIBase, user.OpenaiToken, model, systemPrompt, userPrompt, opts...)
	if err != nil {
		return "", errors.Wrap(err, "chat with user upstream")
	}
	return answer, nil
}
