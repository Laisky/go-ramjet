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

// rendererResolutionError preserves a resolver failure separately from URL or
// public-address policy rejection, without exposing the resolver's raw message.
type rendererResolutionError struct{ cause error }

// Error returns a safe description of the failed renderer hostname lookup.
func (e *rendererResolutionError) Error() string { return "renderer hostname resolution failed" }

// Unwrap retains the resolver cause for timeout and cancellation classification.
func (e *rendererResolutionError) Unwrap() error { return e.cause }

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
		return nil, crawlerEnforcementFailure(errors.Wrap(err, "validate renderer target"))
	}
	if taskID == "" || admission.Host != policy.Host || policy.MaxRedirects < 0 || policy.MaxRedirects > 20 {
		return nil, crawlerEnforcementFailure(errors.Errorf("invalid renderer policy"))
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
		// Shared admission sanitizes resolver errors into input errors. Preserve
		// the cause at the lookup boundary so transport failures are not mistaken
		// for rejected destinations. An empty or private answer stays terminal.
		var resolutionErr error
		admission, err := crawleregress.AdmitURLWithResolver(ctx, raw, func(lookupCtx context.Context, host string) ([]net.IPAddr, error) {
			var resolved []net.IPAddr
			resolved, resolutionErr = f.deps.resolve(lookupCtx, host)
			return resolved, resolutionErr
		})
		if err != nil {
			if resolutionErr != nil {
				return crawleregress.Admission{}, errors.WithStack(&rendererResolutionError{cause: resolutionErr})
			}
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
func (f *rendererEgressFetcher) fetch(ctx context.Context, id, parent, raw, kind, method string, headers http.Header) (response *http.Response, body []byte, resultErr error) {
	admission, err := f.prepare(ctx, id, parent, raw, kind, method)
	if err != nil {
		var resolution *rendererResolutionError
		if !errors.As(err, &resolution) {
			err = crawlerEnforcementFailure(err)
		}
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
			return nil, crawlerEnforcementFailure(errors.Errorf("renderer dial target mismatch"))
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
				return nil, crawlerEnforcementFailure(errors.Wrap(errors.Join(err, closeErr), "invalid renderer peer"))
			}
			if !containsRendererAddress(admission.Addresses, actual) || port != strconv.Itoa(admission.Port) {
				closeErr := conn.Close()
				return nil, crawlerEnforcementFailure(errors.Wrap(errors.Join(errors.New("renderer peer outside admitted pins"), closeErr), "reject renderer peer"))
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
	response, err = client.Do(req)
	if err != nil {
		var verification *tls.CertificateVerificationError
		if errors.As(err, &verification) {
			err = crawlerEnforcementFailure(err)
		}
		return nil, nil, errors.Wrap(err, "renderer exchange")
	}
	// Preserve selected upstream facts before browser fulfillment removes framing headers.
	countedBody := &rendererCountedBody{ReadCloser: response.Body}
	response.Body = countedBody
	observedResponse := &rendererFailureResponse{
		RequestID: id, ParentRequestID: parent, Kind: kind, Method: method,
		URL: raw, UpstreamStatus: response.StatusCode,
		PeerAddress: peer, Protocol: response.Proto,
		DeclaredContentLength: response.ContentLength, ContentEncoding: response.Header.Get("Content-Encoding"),
		TransferEncoding: append([]string(nil), response.TransferEncoding...),
	}
	var decodedBody []byte
	var decodedObserved bool
	defer func() {
		if resultErr != nil {
			if decodedObserved {
				observedResponse.DecodedBody = decodedBody
				observedResponse.DecodedBodyObserved = true
			}
			observedResponse.EncodedBodyBytes = countedBody.bytes
			observeRendererFailure(ctx, observedResponse)
		}
	}()
	// A response status is evidence only; HTTP acceptance and body handling remain unchanged.
	observedStatus := response.StatusCode
	// Headers are observed even when decoding or reading this response later fails.
	observeCrawlerDocumentResponse(ctx, kind, id, parent, raw, response.StatusCode, response.Header.Get("Content-Type"))
	defer func() {
		if resultErr != nil && kind == crawleregress.OriginDocument && observedStatus >= 100 && observedStatus <= 599 {
			resultErr = errors.WithStack(&crawlerHTTPStatusError{status: observedStatus, cause: resultErr})
		}
	}()
	var reader io.Reader = response.Body
	var compressed *gzip.Reader
	switch strings.ToLower(strings.TrimSpace(response.Header.Get("Content-Encoding"))) {
	case "", "identity":
	case "gzip":
		compressed, err = gzip.NewReader(response.Body)
		if err != nil {
			observedResponse.FailurePhase = "decode"
			observedResponse.DecodeError = err.Error()
			closeErr := response.Body.Close()
			if closeErr != nil {
				observedResponse.CloseError = closeErr.Error()
			}
			return nil, nil, errors.Wrap(errors.Join(err, closeErr), "decode renderer gzip")
		}
		reader = compressed
	default:
		observedResponse.FailurePhase = "decode"
		observedResponse.DecodeError = "renderer content encoding denied"
		closeErr := response.Body.Close()
		if closeErr != nil {
			observedResponse.CloseError = closeErr.Error()
		}
		return nil, nil, errors.Wrap(errors.Join(errors.New("renderer content encoding denied"), closeErr), "reject renderer encoding")
	}
	body, readErr := io.ReadAll(io.LimitReader(&rendererBudgetReader{fetcher: f, reader: reader}, rendererMaxResponseBytes+1))
	decodedBody = body
	decodedObserved = true
	observedResponse.DecodedBodyComplete = true
	if readErr != nil {
		observedResponse.DecodedBodyComplete = false
		observedResponse.ReadError = readErr.Error()
		observedResponse.FailurePhase = "read"
		f.bodyMu.Lock()
		totalBudgetExceeded := f.bytes > rendererMaxTotalBytes
		f.bodyMu.Unlock()
		if totalBudgetExceeded {
			observedResponse.FailurePhase = "budget"
		}
	}
	if compressed != nil {
		decodeCloseErr := compressed.Close()
		if decodeCloseErr != nil {
			observedResponse.DecodeCloseError = decodeCloseErr.Error()
			if observedResponse.FailurePhase == "" {
				observedResponse.FailurePhase = "decode"
			}
		}
		readErr = errors.Join(readErr, decodeCloseErr)
	}
	closeErr := response.Body.Close()
	if closeErr != nil {
		observedResponse.CloseError = closeErr.Error()
		if observedResponse.FailurePhase == "" {
			observedResponse.FailurePhase = "close"
		}
	}
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
		observedResponse.FailurePhase = "budget"
		observedResponse.DecodedBodyComplete = false
		return nil, nil, crawlerEnforcementFailure(errors.Errorf("renderer response budget exhausted"))
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.sealed {
		observedResponse.FailurePhase = "post_read"
		return nil, nil, crawlerEnforcementFailure(errors.Errorf("renderer evidence sealed during request"))
	}
	if peer == "" {
		observedResponse.FailurePhase = "post_read"
		return nil, nil, crawlerEnforcementFailure(errors.Errorf("renderer connection evidence missing"))
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
		return 0, crawlerEnforcementFailure(errors.Errorf("renderer total response budget exhausted"))
	}
	if int64(len(buffer)) > remaining+1 {
		buffer = buffer[:remaining+1]
	}
	n, err := r.reader.Read(buffer)
	r.fetcher.bytes += int64(n)
	if r.fetcher.bytes > rendererMaxTotalBytes {
		return n, crawlerEnforcementFailure(errors.Wrap(errors.Join(errors.New("renderer total response budget exhausted"), err), "read renderer body"))
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
		return nil, crawlerEnforcementFailure(errors.Wrap(err, "verify renderer receipt"))
	}
	copyReceipt := *f.receipt
	copyReceipt.Origins = slices.Clone(f.receipt.Origins)
	for i := range copyReceipt.Origins {
		copyReceipt.Origins[i].Addresses = slices.Clone(copyReceipt.Origins[i].Addresses)
	}
	return &copyReceipt, nil
}
