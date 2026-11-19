package egress

import (
	"crypto/tls"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/laisky-blog-graphql/library/crawleregress"
)

// ProxiedTransport is an http.RoundTripper for user-selected targets in a
// deployment whose outbound traffic must leave through an operator proxy.
// Every request, including each redirect hop the http.Client follows, is
// admitted with Admit before it is handed to the proxy, so a refused target
// never reaches the proxy.
//
// The proxy resolves the admitted hostname again on its own. A DNS answer that
// changes between admission and the proxy's lookup can therefore still steer
// the proxy's connection, a small rebinding window that PinnedTransport
// closes. Use ProxiedTransport only where the proxy is required; without one,
// use PinnedTransport. Like PinnedTransport it keeps no idle connections, so a
// client built per request leaves nothing open behind it.
type ProxiedTransport struct {
	resolve   Resolver
	transport *http.Transport
}

// NewProxiedTransport returns a ProxiedTransport that sends admitted requests
// through proxy. deps.Resolve is used for admission and deps.RootCAs verifies
// the target's certificate. deps.Dial is not used: it dials admitted target
// addresses, while this transport only ever connects to the operator's proxy,
// which it reaches with a ten-second dialer. opts adjust the response-header
// timeout as for NewHTTPClient.
func NewProxiedTransport(proxy *url.URL, deps Dependencies, opts ...ClientOption) (*ProxiedTransport, error) {
	if proxy == nil || proxy.Scheme == "" || proxy.Host == "" {
		return nil, errors.New("proxy URL with scheme and host is required")
	}
	options := applyClientOptions(opts)
	dial := (&net.Dialer{Timeout: 10 * time.Second}).DialContext
	return &ProxiedTransport{
		resolve: deps.Resolve,
		transport: &http.Transport{
			Proxy:                  http.ProxyURL(proxy),
			DialContext:            dial,
			DisableKeepAlives:      true,
			ForceAttemptHTTP2:      false,
			TLSClientConfig:        &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: deps.RootCAs},
			TLSHandshakeTimeout:    10 * time.Second,
			ResponseHeaderTimeout:  headerTimeout(options.responseHeaderTimeout),
			MaxResponseHeaderBytes: 1 << 20,
		},
	}, nil
}

// RoundTrip admits request's target and, when admitted, sends the request in
// canonical form through the proxy. Admission refusals are returned as
// *AdmissionError in the error chain.
func (t *ProxiedTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if _, err := Admit(request.Context(), request.URL.String(), t.resolve); err != nil {
		return nil, closeRequestBody(request, errors.Wrap(err, "admit request target"))
	}
	// Send the exact name that was admitted, so the proxy resolves it and
	// not a spelling variant of it.
	canonical, err := url.Parse(crawleregress.RequestURL(request.URL.String()))
	if err != nil {
		return nil, closeRequestBody(request, errors.Wrap(err, "canonicalize request target"))
	}
	request = request.Clone(request.Context())
	request.URL, request.Host = canonical, ""
	response, err := t.transport.RoundTrip(request)
	if err != nil {
		return nil, errors.Wrap(err, "exchange through proxy")
	}
	return response, nil
}

// NewProxiedHTTPClient returns a client for user-selected targets that sends
// every admitted hop, including redirects, through proxy. It follows at most
// the shared policy's default redirect budget and bounds each whole exchange
// by timeout. See ProxiedTransport for the rebinding trade-off.
func NewProxiedHTTPClient(timeout time.Duration, proxy *url.URL, deps Dependencies, opts ...ClientOption) (*http.Client, error) {
	transport, err := NewProxiedTransport(proxy, deps, opts...)
	if err != nil {
		return nil, errors.Wrap(err, "new proxied transport")
	}
	return &http.Client{Transport: transport, Timeout: timeout, CheckRedirect: checkRedirectBudget}, nil
}
