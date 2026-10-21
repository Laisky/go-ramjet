package tasks

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/laisky-blog-graphql/library/crawleregress"
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
func (t verifiedWebFetchTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	lookup := t.deps.resolve
	if lookup == nil {
		lookup = net.DefaultResolver.LookupIPAddr
	}
	admission, err := crawleregress.AdmitURLWithResolver(request.Context(), request.URL.String(), lookup)
	if err != nil {
		if request.Body != nil {
			err = errors.Join(err, request.Body.Close())
		}
		return nil, errors.Wrap(err, "admit provider endpoint")
	}
	canonical, err := url.Parse(crawleregress.RequestURL(request.URL.String()))
	if err != nil {
		if request.Body != nil {
			err = errors.Join(err, request.Body.Close())
		}
		return nil, errors.Wrap(err, "canonicalize provider endpoint")
	}
	request = request.Clone(request.Context())
	request.URL, request.Host = canonical, ""
	request.Header.Set("Accept-Encoding", "gzip")
	dial := t.deps.dial
	if dial == nil {
		dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: -1}
		dial = dialer.DialContext
	}
	transport := &http.Transport{
		Proxy: nil, DisableKeepAlives: true, ForceAttemptHTTP2: false, DisableCompression: true,
		TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12, ServerName: admission.Host, RootCAs: t.deps.rootCAs},
		TLSNextProto:        make(map[string]func(string, *tls.Conn) http.RoundTripper),
		TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 10 * time.Second,
		MaxResponseHeaderBytes: 1 << 20,
	}
	defer transport.CloseIdleConnections()
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		port := strconv.Itoa(admission.Port)
		if network != "tcp" || address != net.JoinHostPort(admission.Host, port) {
			return nil, errors.New("provider dial target mismatch")
		}
		var lastErr error
		for _, pinned := range admission.Addresses {
			conn, dialErr := dial(ctx, "tcp", net.JoinHostPort(pinned, port))
			if dialErr != nil {
				lastErr = dialErr
				continue
			}
			actual, actualPort, peerErr := net.SplitHostPort(conn.RemoteAddr().String())
			if peerErr != nil || actualPort != port || !containsRendererAddress(admission.Addresses, actual) {
				return nil, errors.Wrap(errors.Join(errors.New("provider peer outside admitted pins"), peerErr, conn.Close()), "reject provider peer")
			}
			return conn, nil
		}
		return nil, errors.Wrap(lastErr, "dial provider pins")
	}
	response, err := transport.RoundTrip(request)
	if err != nil {
		return nil, errors.Wrap(err, "exchange with provider")
	}
	return response, nil
}

// readWebFetchProxyHTTPBody preserves legacy reads and bounds both wire and
// decoded provider content when ctx opts into verified endpoint enforcement.
func readWebFetchProxyHTTPBody(ctx context.Context, response *http.Response) ([]byte, error) {
	if _, verified := ctx.Value(verifiedWebFetchClientKey{}).(rendererNetworkDependencies); !verified {
		return io.ReadAll(response.Body)
	}
	if err := ctx.Err(); err != nil {
		return nil, errors.WithStack(err)
	}
	body, err := readBoundedWebFetchBody(response.Body)
	if err != nil {
		return nil, err
	}
	switch strings.ToLower(strings.TrimSpace(response.Header.Get("Content-Encoding"))) {
	case "", "identity":
	case "gzip":
		compressed, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			return nil, errors.Wrap(err, "decode provider gzip")
		}
		decoded, readErr := readBoundedWebFetchBody(compressed)
		if err := errors.Join(readErr, compressed.Close()); err != nil {
			return nil, errors.Wrap(err, "read decoded provider response")
		}
		body = decoded
	default:
		return nil, errors.New("provider content encoding unsupported")
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
