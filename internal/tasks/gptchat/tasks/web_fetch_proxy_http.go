package tasks

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"time"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/go-ramjet/library/egress"
)

// verifiedWebFetchClientKey identifies an explicitly scoped provider transport.
type verifiedWebFetchClientKey struct{}

// withVerifiedWebFetchClient returns ctx with public-endpoint transport enforcement
// and optional local test dependencies for existing provider adapters.
func withVerifiedWebFetchClient(ctx context.Context, deps rendererNetworkDependencies) context.Context {
	if _, verified := ctx.Value(verifiedWebFetchClientKey{}).(rendererNetworkDependencies); verified {
		return ctx
	}
	return context.WithValue(ctx, verifiedWebFetchClientKey{}, deps)
}

// newWebFetchProxyHTTPClient preserves legacy clients unless ctx explicitly opts
// into public provider endpoint admission, connection pinning and no redirects.
func newWebFetchProxyHTTPClient(ctx context.Context) (*http.Client, error) {
	deps, verified := ctx.Value(verifiedWebFetchClientKey{}).(rendererNetworkDependencies)
	if !verified {
		return newCrawlerHTTPClient()
	}
	return &http.Client{
		Transport: verifiedWebFetchTransport{deps: deps},
		Timeout:   30 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}, nil
}

// verifiedWebFetchTransport admits each configured provider endpoint and pins
// its exchange; it does not attest the provider's own acquisition of the target.
type verifiedWebFetchTransport struct{ deps rendererNetworkDependencies }

// RoundTrip sends request only to admitted public provider peers and returns its
// response without automatic redirects, environment proxies or cookie storage.
// Configured provider API credentials require HTTPS before any network activity.
// Admission, pinning and peer checks are owned by egress.PinnedTransport.
func (t verifiedWebFetchTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.URL.Scheme != "https" &&
		(request.Header.Get("Authorization") != "" || request.Header.Get("X-Api-Token") != "") {
		err := errors.New("provider credentials require HTTPS")
		if request.Body != nil {
			err = errors.Join(err, request.Body.Close())
		}
		return nil, errors.WithStack(err)
	}
	request = request.Clone(request.Context())
	request.Header.Set("Accept-Encoding", egress.AcceptEncoding)
	transport := egress.PinnedTransport{Deps: egress.Dependencies{
		Resolve: t.deps.resolve, Dial: t.deps.dial, RootCAs: t.deps.rootCAs,
	}}
	response, err := transport.RoundTrip(request)
	if err != nil {
		return nil, errors.Wrap(err, "exchange with provider")
	}
	return response, nil
}

// readWebFetchProxyHTTPBody preserves legacy reads and bounds both wire and
// decoded provider content when ctx opts into verified endpoint enforcement.
// Content codings follow the shared egress policy (egress.DecodeContent).
func readWebFetchProxyHTTPBody(ctx context.Context, response *http.Response) ([]byte, error) {
	if _, verified := ctx.Value(verifiedWebFetchClientKey{}).(rendererNetworkDependencies); !verified {
		return io.ReadAll(response.Body)
	}
	if err := ctx.Err(); err != nil {
		return nil, errors.WithStack(err)
	}
	wire, err := readBoundedWebFetchBody(response.Body)
	if err != nil {
		return nil, err
	}
	decoded, err := egress.DecodeContent(response.Header, io.NopCloser(bytes.NewReader(wire)))
	if err != nil {
		return nil, errors.Wrap(err, "decode provider response")
	}
	body, readErr := readBoundedWebFetchBody(decoded)
	if err := errors.Join(readErr, decoded.Close()); err != nil {
		return nil, errors.Wrap(err, "read decoded provider response")
	}
	if err := ctx.Err(); err != nil {
		return nil, errors.WithStack(err)
	}
	return body, nil
}

// readBoundedWebFetchBody reads at most the existing renderer byte budget plus
// one byte from reader, returning an error when provider content exceeds it.
func readBoundedWebFetchBody(reader io.Reader) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(reader, rendererMaxResponseBytes+1))
	if err != nil {
		return nil, errors.Wrap(err, "read bounded provider response")
	}
	if len(body) > rendererMaxResponseBytes {
		return nil, errors.New("provider response budget exhausted")
	}
	return body, nil
}
