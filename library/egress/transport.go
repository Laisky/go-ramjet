package egress

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/laisky-blog-graphql/library/crawleregress"
)

// Dependencies supplies optional network seams for deterministic tests.
// Zero values use the system resolver, a ten-second dialer and the system roots.
type Dependencies struct {
	Resolve Resolver
	Dial    func(ctx context.Context, network, address string) (net.Conn, error)
	RootCAs *x509.CertPool
}

// PinnedTransport is an http.RoundTripper for user-selected targets. Every
// request is admitted with Admit and connects only to the addresses that
// admission returned, so DNS rebinding cannot steer the connection elsewhere.
// It never uses environment proxies and keeps no idle connections. Redirects
// are left to the http.Client, so every hop is admitted again.
//
// It never negotiates or removes content codings, so callers see the wire body
// and can bound it before decoding. A caller that wants compression sets
// Accept-Encoding to AcceptEncoding and reads the body through DecodeContent.
type PinnedTransport struct{ Deps Dependencies }

// RoundTrip admits request's target, pins its connection and performs one exchange.
// Admission refusals are returned as *AdmissionError in the error chain.
func (t PinnedTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	admission, err := Admit(request.Context(), request.URL.String(), t.Deps.Resolve)
	if err != nil {
		return nil, closeRequestBody(request, errors.Wrap(err, "admit request target"))
	}
	canonical, err := url.Parse(crawleregress.RequestURL(request.URL.String()))
	if err != nil {
		return nil, closeRequestBody(request, errors.Wrap(err, "canonicalize request target"))
	}
	request = request.Clone(request.Context())
	request.URL, request.Host = canonical, ""
	dial := t.Deps.Dial
	if dial == nil {
		dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: -1}
		dial = dialer.DialContext
	}
	transport := &http.Transport{
		Proxy: nil, DisableKeepAlives: true, ForceAttemptHTTP2: false, DisableCompression: true,
		TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12, ServerName: admission.Host, RootCAs: t.Deps.RootCAs},
		TLSNextProto:        make(map[string]func(string, *tls.Conn) http.RoundTripper),
		TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 10 * time.Second,
		MaxResponseHeaderBytes: 1 << 20,
	}
	defer transport.CloseIdleConnections()
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		port := strconv.Itoa(admission.Port)
		if network != "tcp" || address != net.JoinHostPort(admission.Host, port) {
			return nil, errors.New("dial target mismatch")
		}
		var lastErr error
		for _, pinned := range admission.Addresses {
			conn, dialErr := dial(ctx, "tcp", net.JoinHostPort(pinned, port))
			if dialErr != nil {
				lastErr = dialErr
				continue
			}
			actual, actualPort, peerErr := net.SplitHostPort(conn.RemoteAddr().String())
			if peerErr != nil || actualPort != port || !containsAddress(admission.Addresses, actual) {
				return nil, errors.Wrap(errors.Join(errors.New("peer outside admitted pins"), peerErr, conn.Close()), "reject peer")
			}
			return conn, nil
		}
		return nil, errors.Wrap(lastErr, "dial admitted pins")
	}
	response, err := transport.RoundTrip(request)
	if err != nil {
		return nil, errors.Wrap(err, "exchange with admitted target")
	}
	return response, nil
}

// NewHTTPClient returns a client for user-selected targets whose every hop,
// including redirects, is admitted and pinned. It follows at most the shared
// policy's default redirect budget and bounds each whole exchange by timeout.
func NewHTTPClient(timeout time.Duration, deps Dependencies) *http.Client {
	return &http.Client{
		Transport: PinnedTransport{Deps: deps},
		Timeout:   timeout,
		CheckRedirect: func(_ *http.Request, via []*http.Request) error {
			if len(via) > crawleregress.DefaultMaxRedirects {
				return errors.Errorf("stopped after %d redirects", crawleregress.DefaultMaxRedirects)
			}
			return nil
		},
	}
}

// closeRequestBody closes request's body, as RoundTripper requires on every
// error, and returns cause joined with any close failure.
func closeRequestBody(request *http.Request, cause error) error {
	if request.Body == nil {
		return cause
	}
	if closeErr := request.Body.Close(); closeErr != nil {
		return errors.Join(cause, closeErr)
	}
	return cause
}

// containsAddress compares canonical IP values, including IPv4-mapped IPv6 peers.
func containsAddress(addresses []string, raw string) bool {
	ip := net.ParseIP(raw)
	if ip == nil {
		return false
	}
	for _, address := range addresses {
		if admitted := net.ParseIP(address); admitted != nil && admitted.Equal(ip) {
			return true
		}
	}
	return false
}
