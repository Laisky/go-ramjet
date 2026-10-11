package tasks

import (
	"context"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/laisky-blog-graphql/library/crawleregress"
	rlibs "github.com/Laisky/laisky-blog-graphql/library/db/redis"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

// rendererDenyProxy blocks browser network paths not fulfilled by the pinned Go transport.
type rendererDenyProxy struct {
	server   *http.Server
	listener net.Listener
	done     chan struct{}
	mu       sync.Mutex
	serveErr error
}

// startRendererDenyProxy binds an ephemeral loopback proxy that rejects HTTP and CONNECT.
func startRendererDenyProxy(ctx context.Context, denied func(), failed func(error)) (*rendererDenyProxy, error) {
	listener, err := new(net.ListenConfig).Listen(ctx, "tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, errors.Wrap(err, "listen renderer deny proxy")
	}
	p := &rendererDenyProxy{listener: listener, done: make(chan struct{})}
	p.server = &http.Server{ReadHeaderTimeout: 2 * time.Second, IdleTimeout: time.Second, MaxHeaderBytes: 8192, Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		denied()
		w.Header().Set("Connection", "close")
		http.Error(w, "renderer route denied", http.StatusForbidden)
	})}
	go func() {
		defer close(p.done)
		if err := p.server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			p.mu.Lock()
			p.serveErr = errors.Wrap(err, "serve renderer deny proxy")
			p.mu.Unlock()
			failed(errors.Wrap(err, "renderer deny proxy unavailable"))
		}
	}()
	return p, nil
}

// close stops the task-local listener and reports unexpected serving or cleanup failures.
func (p *rendererDenyProxy) close() error {
	closeErr := p.server.Close()
	<-p.done
	p.mu.Lock()
	defer p.mu.Unlock()
	return errors.Wrap(errors.Join(closeErr, p.serveErr), "close renderer deny proxy")
}

// rendererBrowserOptions forces unsupported networking through the task-local deny proxy.
func rendererBrowserOptions(proxy string) []chromedp.ExecAllocatorOption {
	return append(slicesOfRendererDefaults(),
		chromedp.Headless,
		chromedp.ExecPath("crawler-browser-launcher"),
		chromedp.Flag("remote-debugging-pipe", true),
		chromedp.Flag("proxy-server", "http://"+proxy),
		chromedp.Flag("proxy-bypass-list", "<-loopback>"),
		chromedp.Flag("host-resolver-rules", "MAP * ~NOTFOUND, EXCLUDE 127.0.0.1"),
		chromedp.Flag("disable-quic", true),
		chromedp.Flag("force-webrtc-ip-handling-policy", "disable_non_proxied_udp"),
		chromedp.Flag("disable-background-networking", true),
		chromedp.Flag("disable-component-update", true),
		chromedp.Flag("disable-domain-reliability", true),
		chromedp.Flag("disable-sync", true),
	)
}

// slicesOfRendererDefaults copies allocator defaults so per-task flags never mutate shared configuration.
func slicesOfRendererDefaults() []chromedp.ExecAllocatorOption {
	return append([]chromedp.ExecAllocatorOption(nil), chromedp.DefaultExecAllocatorOptions[:]...)
}

// renderVerifiedHTML renders a task using independently pinned exchanges and returns bound peer evidence.
func renderVerifiedHTML(ctx context.Context, task *rlibs.HTMLCrawlerTask) (string, *crawleregress.Receipt, error) {
	return renderVerifiedHTMLWithDependencies(ctx, task, rendererNetworkDependencies{}, nil)
}

// renderVerifiedHTMLWithDependencies supplies explicit test seams without reading settings or credentials.
func renderVerifiedHTMLWithDependencies(ctx context.Context, task *rlibs.HTMLCrawlerTask, deps rendererNetworkDependencies, extra []chromedp.ExecAllocatorOption) (html string, receipt *crawleregress.Receipt, resultErr error) {
	if task == nil || task.Egress == nil {
		return "", nil, errors.Errorf("renderer policy missing")
	}
	policy := crawleregress.Policy{Host: task.Egress.Host, Addresses: task.Egress.Addresses, MaxRedirects: task.Egress.MaxRedirects, AllowSubresources: task.Egress.AllowSubresources}
	f, err := newRendererEgressFetcher(ctx, task.TaskID, task.Url, policy, deps)
	if err != nil {
		return "", nil, errors.Wrap(err, "initialize renderer enforcement")
	}
	if err := chromedpSema.Acquire(ctx, 1); err != nil {
		return "", nil, errors.Wrap(err, "acquire renderer capacity")
	}
	defer chromedpSema.Release(1)
	renderCtx, stopRender := context.WithTimeout(ctx, 60*time.Second)
	defer stopRender()
	var failureMu sync.Mutex
	var failure error
	var snapshotComplete bool
	var activeRequest bool
	lastActivity := time.Now()
	var blockedRequests atomic.Int64
	fail := func(err error) {
		failureMu.Lock()
		defer failureMu.Unlock()
		if failure != nil || snapshotComplete {
			return
		}
		failure = err
		stopRender()
	}
	blocked := func() {
		if blockedRequests.Add(1) > rendererMaxRequests {
			fail(errors.New("renderer blocked request budget exhausted"))
		}
	}
	p, err := startRendererDenyProxy(renderCtx, blocked, fail)
	if err != nil {
		return "", nil, errors.Wrap(err, "initialize renderer browser gate")
	}
	defer func() {
		if err := p.close(); err != nil {
			html = ""
			receipt = nil
			resultErr = errors.Wrap(errors.Join(resultErr, err), "renderer gate cleanup")
		}
	}()
	opts := append(rendererBrowserOptions(p.listener.Addr().String()), extra...)
	allocatorCtx, stopAllocator := chromedp.NewExecAllocator(renderCtx, opts...)
	defer stopAllocator()
	browserCtx, stopBrowser := chromedp.NewContext(allocatorCtx)
	defer stopBrowser()
	if err := chromedp.Do(browserCtx); err != nil {
		return "", nil, errors.Wrap(err, "initialize isolated renderer browser")
	}
	tree, err := chromedp.Call(browserCtx, page.GetFrameTree, cdp.Empty{})
	if err != nil {
		return "", nil, errors.Wrap(err, "identify renderer main frame")
	}
	rootFrame := tree.FrameTree.Frame.ID
	paused := chromedp.Events(browserCtx, fetch.RequestPaused)
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		observedRequests := 0
		for event, err := range paused {
			if err != nil {
				fail(errors.Wrap(err, "observe renderer requests"))
				return
			}
			observedRequests++
			if observedRequests > rendererMaxRequests || event.Request == nil {
				fail(errors.New("renderer intercepted request budget exhausted"))
				return
			}
			failureMu.Lock()
			if snapshotComplete {
				failureMu.Unlock()
				return
			}
			activeRequest = true
			lastActivity = time.Now()
			failureMu.Unlock()
			kind := crawleregress.OriginSubresource
			if event.ResourceType == network.ResourceTypeDocument && event.FrameID == rootFrame {
				kind = crawleregress.OriginDocument
			}
			if kind == crawleregress.OriginSubresource && !policy.AllowSubresources {
				blocked()
				if _, err := chromedp.Call(browserCtx, fetch.FailRequest, fetch.FailRequestParams{RequestID: event.RequestID, ErrorReason: network.ErrorReasonBlockedByClient}); err != nil {
					fail(errors.Wrap(err, "block renderer subresource"))
					return
				}
				failureMu.Lock()
				activeRequest = false
				lastActivity = time.Now()
				failureMu.Unlock()
				continue
			}
			headers := make(http.Header)
			for name, value := range event.Request.Headers {
				if text, ok := value.(string); ok {
					headers.Add(name, text)
				}
			}
			response, body, err := f.fetch(browserCtx, string(event.RequestID), string(event.RedirectedRequestID), event.Request.URL, kind, event.Request.Method, headers)
			if err != nil {
				fail(errors.Wrap(err, "fetch renderer request"))
				return
			}
			responseHeaders := make([]*fetch.HeaderEntry, 0, len(response.Header))
			for name, values := range response.Header {
				switch strings.ToLower(name) {
				case "connection", "proxy-connection", "transfer-encoding", "content-length", "keep-alive", "upgrade", "alt-svc":
					continue
				}
				for _, value := range values {
					responseHeaders = append(responseHeaders, &fetch.HeaderEntry{Name: name, Value: value})
				}
			}
			if _, err := chromedp.Call(browserCtx, fetch.FulfillRequest, fetch.FulfillRequestParams{RequestID: event.RequestID, ResponseCode: int64(response.StatusCode), ResponseHeaders: responseHeaders, Body: body}); err != nil {
				fail(errors.Wrap(err, "fulfill renderer request"))
				return
			}
			failureMu.Lock()
			activeRequest = false
			lastActivity = time.Now()
			failureMu.Unlock()
		}
	}()
	if _, err := chromedp.Call(browserCtx, network.SetCacheDisabled, network.SetCacheDisabledParams{CacheDisabled: true}); err != nil {
		fail(errors.Wrap(err, "disable renderer cache"))
	}
	if _, err := chromedp.Call(browserCtx, network.SetBypassServiceWorker, network.SetBypassServiceWorkerParams{Bypass: true}); err != nil {
		fail(errors.Wrap(err, "bypass renderer service workers"))
	}
	if _, err := chromedp.Call(browserCtx, fetch.Enable, fetch.EnableParams{Patterns: []*fetch.RequestPattern{{URLPattern: "*", RequestStage: fetch.RequestStageRequest}}}); err != nil {
		fail(errors.Wrap(err, "enable renderer interception"))
	}
	navigationErr := chromedp.Do(browserCtx, chromedp.Navigate(task.Url))
	var captureErr error
	if navigationErr != nil {
		captureErr = navigationErr
	} else {
		stabilize := time.NewTimer(2 * time.Second)
		select {
		case <-renderCtx.Done():
			captureErr = renderCtx.Err()
		case <-stabilize.C:
		}
		if !stabilize.Stop() {
			select {
			case <-stabilize.C:
			default:
			}
		}
		for {
			if captureErr != nil {
				break
			}
			for {
				failureMu.Lock()
				quiet := !activeRequest && time.Since(lastActivity) >= 300*time.Millisecond
				failureMu.Unlock()
				if quiet {
					break
				}
				timer := time.NewTimer(25 * time.Millisecond)
				select {
				case <-renderCtx.Done():
					captureErr = renderCtx.Err()
				case <-timer.C:
				}
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				if captureErr != nil {
					break
				}
			}
			if captureErr != nil {
				break
			}
			html, captureErr = chromedp.Run(browserCtx, chromedp.OuterHTML(chromedp.CSS("html")))
			failureMu.Lock()
			if !activeRequest || captureErr != nil {
				snapshotComplete = true
				failureMu.Unlock()
				break
			}
			failureMu.Unlock()
		}
	}
	// No browser can keep making requests after the snapshot or mutate returned evidence.
	failureMu.Lock()
	snapshotComplete = true
	failureMu.Unlock()
	stopRender()
	stopBrowser()
	<-workerDone
	failureMu.Lock()
	observedFailure := failure
	failureMu.Unlock()
	if err := errors.Join(observedFailure, captureErr); err != nil {
		return "", nil, errors.Wrap(err, "verified renderer failed")
	}
	if len(html) > crawleregress.MaxResponseBytes {
		return "", nil, errors.Errorf("renderer DOM response budget exhausted")
	}
	receipt, err = f.seal(ctx)
	if err != nil {
		return "", nil, errors.Wrap(err, "seal renderer evidence")
	}
	receipt.BlockedRequests = blockedRequests.Load()
	return html, receipt, nil
}
