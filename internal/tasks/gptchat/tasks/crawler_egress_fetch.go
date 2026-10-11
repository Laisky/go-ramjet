package tasks

import (
	"compress/gzip"
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/laisky-blog-graphql/library/crawleregress"
)

const (
	rendererMaxRequests      = 128
	rendererMaxResponseBytes = crawleregress.MaxResponseBytes
	rendererMaxTotalBytes    = crawleregress.MaxTotalBytes
)

// rendererNetworkDependencies supplies controlled network seams for local behavioral tests.
type rendererNetworkDependencies struct {
	resolve func(context.Context, string) ([]net.IPAddr, error)
	dial    func(context.Context, string, string) (net.Conn, error)
	rootCAs *x509.CertPool
}

// rendererEgressFetcher admits each request, pins the connection, and records actual peers.
type rendererEgressFetcher struct {
	mu        sync.Mutex
	bodyMu    sync.Mutex
	policy    crawleregress.Policy
	target    string
	pins      map[string][]string
	depths    map[string]int
	documents int
	requests  int
	bytes     int64
	sealed    bool
	receipt   *crawleregress.Receipt
	deps      rendererNetworkDependencies
}

// newRendererEgressFetcher validates a task policy without resolving its original host again.
func newRendererEgressFetcher(ctx context.Context, taskID, target string, policy crawleregress.Policy, deps rendererNetworkDependencies) (*rendererEgressFetcher, error) {
	policy = crawleregress.ClonePolicy(policy)
	admission, err := crawleregress.ValidatePolicy(ctx, target, policy)
	if err != nil {
		return nil, errors.Wrap(err, "validate renderer target")
	}
	if taskID == "" || admission.Host != policy.Host || policy.MaxRedirects < 0 || policy.MaxRedirects > 20 {
		return nil, errors.Errorf("invalid renderer policy")
	}
	if deps.resolve == nil {
		deps.resolve = net.DefaultResolver.LookupIPAddr
	}
	if deps.dial == nil {
		dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: -1}
		deps.dial = dialer.DialContext
	}
	return &rendererEgressFetcher{
		policy: policy, target: target, pins: map[string][]string{admission.Host: slices.Clone(admission.Addresses)},
		depths: make(map[string]int), receipt: crawleregress.NewReceipt(taskID, target, policy), deps: deps,
	}, nil
}

// prepare reserves bounded request work and returns independently validated pinned admission.
func (f *rendererEgressFetcher) prepare(ctx context.Context, id, parent, raw, kind, method string) (crawleregress.Admission, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.sealed {
		return crawleregress.Admission{}, errors.Errorf("renderer evidence sealed")
	}
	if method != http.MethodGet && method != http.MethodHead {
		return crawleregress.Admission{}, errors.Errorf("renderer method denied")
	}
	if f.requests >= rendererMaxRequests {
		return crawleregress.Admission{}, errors.Errorf("renderer request budget exhausted")
	}
	if _, exists := f.depths[id]; exists || id == "" {
		return crawleregress.Admission{}, errors.Errorf("renderer request identity invalid")
	}
	depth := 0
	if parent != "" {
		previous, exists := f.depths[parent]
		if !exists {
			return crawleregress.Admission{}, errors.Errorf("renderer redirect parent missing")
		}
		depth = previous + 1
		if depth > f.policy.MaxRedirects {
			return crawleregress.Admission{}, errors.Errorf("renderer redirect budget exhausted")
		}
	}
	switch kind {
	case crawleregress.OriginDocument:
		if f.documents == 0 && crawleregress.RequestURL(raw) != crawleregress.RequestURL(f.target) {
			return crawleregress.Admission{}, errors.Errorf("renderer initial document mismatch")
		}
		if f.documents > f.policy.MaxRedirects {
			return crawleregress.Admission{}, errors.Errorf("renderer document budget exhausted")
		}
	case crawleregress.OriginSubresource:
		if !f.policy.AllowSubresources {
			return crawleregress.Admission{}, errors.Errorf("renderer subresource denied")
		}
	default:
		return crawleregress.Admission{}, errors.Errorf("renderer request kind denied")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return crawleregress.Admission{}, errors.Wrap(err, "parse renderer request")
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	addresses, exists := f.pins[host]
	if !exists {
		admission, err := crawleregress.AdmitURLWithResolver(ctx, raw, f.deps.resolve)
		if err != nil {
			return crawleregress.Admission{}, errors.Wrap(err, "admit renderer origin")
		}
		addresses = slices.Clone(admission.Addresses)
		f.pins[admission.Host] = addresses
	}
	admission, err := crawleregress.ValidateDestination(ctx, raw, addresses)
	if err != nil {
		return crawleregress.Admission{}, errors.Wrap(err, "validate renderer request")
	}
	f.depths[id] = depth
	f.requests++
	if kind == crawleregress.OriginDocument {
		f.documents++
	}
	return admission, nil
}

// fetch performs one admitted HTTP exchange without following redirects or using environment proxies.
func (f *rendererEgressFetcher) fetch(ctx context.Context, id, parent, raw, kind, method string, headers http.Header) (*http.Response, []byte, error) {
	admission, err := f.prepare(ctx, id, parent, raw, kind, method)
	if err != nil {
		return nil, nil, errors.Wrap(err, "prepare renderer request")
	}
	var peer string
	originIndex := -1
	transport := &http.Transport{
		Proxy: nil, DisableKeepAlives: true, ForceAttemptHTTP2: false, DisableCompression: true,
		TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12, ServerName: admission.Host, RootCAs: f.deps.rootCAs},
		TLSNextProto:        make(map[string]func(string, *tls.Conn) http.RoundTripper),
		TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 10 * time.Second,
		MaxResponseHeaderBytes: 1 << 20,
	}
	defer transport.CloseIdleConnections()
	transport.DialContext = func(dialCtx context.Context, network, address string) (net.Conn, error) {
		expected := net.JoinHostPort(admission.Host, strconv.Itoa(admission.Port))
		if address != expected || network != "tcp" {
			return nil, errors.Errorf("renderer dial target mismatch")
		}
		var lastErr error
		for _, pinned := range admission.Addresses {
			conn, err := f.deps.dial(dialCtx, "tcp", net.JoinHostPort(pinned, strconv.Itoa(admission.Port)))
			if err != nil {
				lastErr = err
				continue
			}
			actual, port, err := net.SplitHostPort(conn.RemoteAddr().String())
			if err != nil {
				closeErr := conn.Close()
				return nil, errors.Wrap(errors.Join(err, closeErr), "invalid renderer peer")
			}
			if !containsRendererAddress(admission.Addresses, actual) || port != strconv.Itoa(admission.Port) {
				closeErr := conn.Close()
				return nil, errors.Wrap(errors.Join(errors.New("renderer peer outside admitted pins"), closeErr), "reject renderer peer")
			}
			peer = net.ParseIP(actual).String()
			f.mu.Lock()
			originIndex = len(f.receipt.Origins)
			f.receipt.Origins = append(f.receipt.Origins, crawleregress.Origin{URL: raw, Addresses: slices.Clone(admission.Addresses), PeerAddress: peer, Kind: kind})
			f.mu.Unlock()
			return conn, nil
		}
		return nil, errors.Wrap(lastErr, "dial renderer pins")
	}
	req, err := http.NewRequestWithContext(ctx, method, crawleregress.RequestURL(raw), nil)
	if err != nil {
		return nil, nil, errors.Wrap(err, "build renderer request")
	}
	// Fresh renders carry no API keys, cookies, authentication, or page-generated bodies upstream.
	for _, name := range []string{"Accept", "Accept-Language", "User-Agent", "Range"} {
		for _, value := range headers.Values(name) {
			req.Header.Add(name, value)
		}
	}
	req.Header.Set("Accept-Encoding", "gzip")
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(req)
	if err != nil {
		return nil, nil, errors.Wrap(err, "renderer exchange")
	}
	var reader io.Reader = response.Body
	var compressed *gzip.Reader
	switch strings.ToLower(strings.TrimSpace(response.Header.Get("Content-Encoding"))) {
	case "", "identity":
	case "gzip":
		compressed, err = gzip.NewReader(response.Body)
		if err != nil {
			return nil, nil, errors.Wrap(errors.Join(err, response.Body.Close()), "decode renderer gzip")
		}
		reader = compressed
	default:
		return nil, nil, errors.Wrap(errors.Join(errors.New("renderer content encoding denied"), response.Body.Close()), "reject renderer encoding")
	}
	body, readErr := io.ReadAll(io.LimitReader(&rendererBudgetReader{fetcher: f, reader: reader}, rendererMaxResponseBytes+1))
	if compressed != nil {
		readErr = errors.Join(readErr, compressed.Close())
	}
	closeErr := response.Body.Close()
	response.Header.Del("Content-Encoding")
	response.Header.Del("Content-Length")
	f.mu.Lock()
	if originIndex >= 0 {
		f.receipt.Origins[originIndex].BodyBytes = int64(len(body))
	}
	f.mu.Unlock()
	if err := errors.Join(readErr, closeErr); err != nil {
		return nil, nil, errors.Wrap(err, "read renderer response")
	}
	if len(body) > rendererMaxResponseBytes {
		return nil, nil, errors.Errorf("renderer response budget exhausted")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.sealed {
		return nil, nil, errors.Errorf("renderer evidence sealed during request")
	}
	if peer == "" {
		return nil, nil, errors.Errorf("renderer connection evidence missing")
	}
	return response, body, nil
}

// rendererBudgetReader enforces the shared decoded-byte budget even when a response later fails.
type rendererBudgetReader struct {
	fetcher *rendererEgressFetcher
	reader  io.Reader
}

// Read serializes body budget consumption and bounds each read by the remaining total capacity.
func (r *rendererBudgetReader) Read(buffer []byte) (int, error) {
	r.fetcher.bodyMu.Lock()
	defer r.fetcher.bodyMu.Unlock()
	remaining := int64(rendererMaxTotalBytes) - r.fetcher.bytes
	if remaining < 0 {
		return 0, errors.Errorf("renderer total response budget exhausted")
	}
	if int64(len(buffer)) > remaining+1 {
		buffer = buffer[:remaining+1]
	}
	n, err := r.reader.Read(buffer)
	r.fetcher.bytes += int64(n)
	if r.fetcher.bytes > rendererMaxTotalBytes {
		return n, errors.Wrap(errors.Join(errors.New("renderer total response budget exhausted"), err), "read renderer body")
	}
	return n, err
}

// containsRendererAddress compares canonical IP values, including IPv4-mapped IPv6 peers.
func containsRendererAddress(addresses []string, raw string) bool {
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

// seal returns an immutable receipt after all rendering work has stopped and validates its binding.
func (f *rendererEgressFetcher) seal(ctx context.Context) (*crawleregress.Receipt, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sealed = true
	if err := crawleregress.VerifyReceipt(ctx, f.receipt.TaskID, f.target, f.policy, f.receipt); err != nil {
		return nil, errors.Wrap(err, "verify renderer receipt")
	}
	copyReceipt := *f.receipt
	copyReceipt.Origins = slices.Clone(f.receipt.Origins)
	for i := range copyReceipt.Origins {
		copyReceipt.Origins[i].Addresses = slices.Clone(copyReceipt.Origins[i].Addresses)
	}
	return &copyReceipt, nil
}
