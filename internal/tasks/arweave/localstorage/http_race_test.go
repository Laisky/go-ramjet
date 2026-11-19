package localstorage

import (
	"context"
	"io"
	"net"
	"net/http"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/library/egress"
)

// proxyRaceIterations is how many requests each timing-sensitive regression issues.
// The coordination defects it guards against were timing dependent, so a single
// request could pass by luck; every iteration must succeed.
const proxyRaceIterations = 300

// trackedBody is an in-memory response body that records whether it was closed.
type trackedBody struct {
	*strings.Reader
	closed atomic.Bool
}

// Close records the close and always succeeds.
func (b *trackedBody) Close() error {
	b.closed.Store(true)
	return nil
}

// contentLoader matches the signature of the live and archived loaders.
type contentLoader = func(ctx context.Context, target string) (io.ReadCloser, error)

// bodyRecorder hands out tracked bodies and keeps them for later close assertions.
type bodyRecorder struct {
	bodies chan *trackedBody
}

// newBodyRecorder returns a recorder that can retain up to capacity bodies.
func newBodyRecorder(capacity int) *bodyRecorder {
	return &bodyRecorder{bodies: make(chan *trackedBody, capacity)}
}

// loader returns a loader that immediately succeeds with content.
func (r *bodyRecorder) loader(content string) contentLoader {
	return func(context.Context, string) (io.ReadCloser, error) {
		body := &trackedBody{Reader: strings.NewReader(content)}
		r.bodies <- body
		return body, nil
	}
}

// requireAllClosed waits until every body handed out so far has been closed.
func (r *bodyRecorder) requireAllClosed(t *testing.T) {
	t.Helper()
	for {
		select {
		case body := <-r.bodies:
			require.Eventually(t, body.closed.Load, 2*time.Second, time.Millisecond,
				"every body produced by a source must be closed, including the loser's")
		default:
			return
		}
	}
}

// failingLoader returns a loader that immediately fails with err.
func failingLoader(err error) contentLoader {
	return func(context.Context, string) (io.ReadCloser, error) {
		return nil, err
	}
}

// blockingLoader returns a loader that waits for its context to end, reporting the
// cancellation on canceled so tests can assert that losers are released.
func blockingLoader(canceled *atomic.Int32) contentLoader {
	return func(ctx context.Context, _ string) (io.ReadCloser, error) {
		<-ctx.Done()
		canceled.Add(1)
		return nil, errors.WithStack(ctx.Err())
	}
}

// useProxySources makes public.example admissible without any network access and
// replaces the live loader, the archive loader and the archive head start for one test.
func useProxySources(t *testing.T, origin, cache contentLoader, delay time.Duration) {
	t.Helper()
	useArchiveTestConfig()
	requireNoProxyGoroutines(t)
	oldNetwork := targetNetwork
	oldOrigin, oldCache, oldDelay := loadOriginContent, loadCachedContent, cacheFallbackDelay
	targetNetwork = egress.Dependencies{Resolve: func(_ context.Context, host string) ([]net.IPAddr, error) {
		if host != "public.example" {
			return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
		}
		return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
	}}
	loadOriginContent, loadCachedContent, cacheFallbackDelay = origin, cache, delay
	t.Cleanup(func() {
		requireNoProxyGoroutines(t)
		targetNetwork = oldNetwork
		loadOriginContent, loadCachedContent, cacheFallbackDelay = oldOrigin, oldCache, oldDelay
		drainArchiveTasks()
	})
}

// proxyGoroutines returns the stacks of goroutines, other than the caller, that are
// still running code from this package's non-test files.
func proxyGoroutines() []string {
	buf := make([]byte, 1<<20)
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			buf = buf[:n]
			break
		}
		buf = make([]byte, 2*len(buf))
	}
	var leaked []string
	for i, stack := range strings.Split(string(buf), "\n\n") {
		if i == 0 { // the calling goroutine
			continue
		}
		for line := range strings.Lines(stack) {
			if strings.Contains(line, "/internal/tasks/arweave/localstorage/") &&
				strings.Contains(line, ".go:") && !strings.Contains(line, "_test.go:") {
				leaked = append(leaked, stack)
				break
			}
		}
	}
	return leaked
}

// requireNoProxyGoroutines waits until no goroutine is left running handler or
// archive code, so a request that already answered cannot leave work behind.
func requireNoProxyGoroutines(t *testing.T) {
	t.Helper()
	// Waiting on the tracked work orders its reads of package-level test seams
	// before the caller's writes; the stack scan then catches untracked leaks.
	tracked := make(chan struct{})
	go func() {
		requestBackgroundWork.Wait()
		close(tracked)
	}()
	select {
	case <-tracked:
	case <-time.After(3 * time.Second):
		require.Fail(t, "request background work did not finish")
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		leaked := proxyGoroutines()
		if len(leaked) == 0 {
			return
		}
		if time.Now().After(deadline) {
			require.Failf(t, "goroutines outlived their request",
				"%d goroutine(s) still run proxy code; first:\n%s", len(leaked), leaked[0])
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestCacheHandlerDeliversImmediateLiveSuccess guards against dropping a live
// result that is ready before the handler starts waiting: the old non-blocking
// send to an unbuffered channel discarded it and answered with the archive miss.
func TestCacheHandlerDeliversImmediateLiveSuccess(t *testing.T) {
	bodies := newBodyRecorder(proxyRaceIterations)
	useProxySources(t, bodies.loader("live-content"), failingLoader(errors.New("archive miss")), 0)

	for i := range proxyRaceIterations {
		recorder := serveCacheHandler("http://public.example/page")
		require.Equal(t, http.StatusOK, recorder.Code, "iteration %d: %s", i, recorder.Body.String())
		require.Equal(t, "DYNAMIC", recorder.Header().Get("X-Laisky-Cache-Status"), "iteration %d", i)
		require.Equal(t, "live-content", recorder.Body.String(), "iteration %d", i)
	}
	bodies.requireAllClosed(t)
}

// TestCacheHandlerAnswersWhenBothSourcesSucceedAtOnce guards against dropping both
// results: the old handler then received a nil error and answered an empty 200
// without a cache status. Exactly one body is served and the other is closed.
func TestCacheHandlerAnswersWhenBothSourcesSucceedAtOnce(t *testing.T) {
	bodies := newBodyRecorder(2 * proxyRaceIterations)
	useProxySources(t, bodies.loader("live-content"), bodies.loader("cached-content"), 0)

	for i := range proxyRaceIterations {
		recorder := serveCacheHandler("http://public.example/page")
		require.Equal(t, http.StatusOK, recorder.Code, "iteration %d: %s", i, recorder.Body.String())
		switch status := recorder.Header().Get("X-Laisky-Cache-Status"); status {
		case "DYNAMIC":
			require.Equal(t, "live-content", recorder.Body.String(), "iteration %d", i)
		case "CACHE":
			require.Equal(t, "cached-content", recorder.Body.String(), "iteration %d", i)
		default:
			require.Failf(t, "missing cache status", "iteration %d: status %q body %q", i, status, recorder.Body.String())
		}
	}
	bodies.requireAllClosed(t)
}

// TestCacheHandlerLeavesNoGoroutinesAfterSuccess guards against the completion
// goroutine blocking forever on an unbuffered channel once the handler has
// already answered from a successful source.
func TestCacheHandlerLeavesNoGoroutinesAfterSuccess(t *testing.T) {
	var canceled atomic.Int32
	bodies := newBodyRecorder(20)
	useProxySources(t, bodies.loader("live-content"), blockingLoader(&canceled), 0)

	for range 20 {
		recorder := serveCacheHandler("http://public.example/page")
		require.Equal(t, http.StatusOK, recorder.Code)
		require.Equal(t, "live-content", recorder.Body.String())
	}
	requireNoProxyGoroutines(t)
	bodies.requireAllClosed(t)
}

// TestCacheHandlerSkipsArchiveAfterLiveSuccess requires the archive head start to
// end with the request: once the live fetch has answered, the archive lookup must
// neither run nor leave a goroutine sleeping out its delay.
func TestCacheHandlerSkipsArchiveAfterLiveSuccess(t *testing.T) {
	var archiveCalls atomic.Int32
	bodies := newBodyRecorder(1)
	useProxySources(t, bodies.loader("live-content"), func(context.Context, string) (io.ReadCloser, error) {
		archiveCalls.Add(1)
		return nil, errors.New("archive must not be consulted")
	}, time.Hour)

	recorder := serveCacheHandler("http://public.example/page")
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "DYNAMIC", recorder.Header().Get("X-Laisky-Cache-Status"))
	requireNoProxyGoroutines(t)
	require.Zero(t, archiveCalls.Load())
	bodies.requireAllClosed(t)
}

// TestCacheHandlerFallsBackToArchive preserves the archive fallback when the live fetch fails.
func TestCacheHandlerFallsBackToArchive(t *testing.T) {
	bodies := newBodyRecorder(1)
	useProxySources(t, failingLoader(errors.New("origin unavailable")), bodies.loader("cached-content"), 10*time.Millisecond)

	recorder := serveCacheHandler("http://public.example/page")
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Equal(t, "CACHE", recorder.Header().Get("X-Laisky-Cache-Status"))
	require.Equal(t, "cached-content", recorder.Body.String())
	bodies.requireAllClosed(t)
}

// TestCacheHandlerArchiveWinsOverSlowOrigin serves the archive when the live fetch
// is still pending, and releases the pending live fetch instead of leaving it running.
func TestCacheHandlerArchiveWinsOverSlowOrigin(t *testing.T) {
	var canceled atomic.Int32
	bodies := newBodyRecorder(1)
	useProxySources(t, blockingLoader(&canceled), bodies.loader("cached-content"), 0)

	recorder := serveCacheHandler("http://public.example/page")
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Equal(t, "CACHE", recorder.Header().Get("X-Laisky-Cache-Status"))
	require.Equal(t, "cached-content", recorder.Body.String())
	requireNoProxyGoroutines(t)
	require.EqualValues(t, 1, canceled.Load(), "the losing live fetch must be canceled")
	bodies.requireAllClosed(t)
}

// TestCacheHandlerReportsLiveRefusalWhenBothFail keeps the existing answer when no
// source succeeds: a destination-policy refusal on the live fetch (here, a hop
// into a private address) is a 400 input error carrying its typed reason.
func TestCacheHandlerReportsLiveRefusalWhenBothFail(t *testing.T) {
	_, refusal := egress.Admit(context.Background(), "http://127.0.0.1/metadata", nil)
	require.True(t, egress.IsExpectedRefusal(refusal))
	useProxySources(t,
		failingLoader(errors.Wrap(refusal, "follow redirect")),
		failingLoader(errors.New("archive miss")),
		0)

	recorder := serveCacheHandler("http://public.example/page")
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), string(egress.ReasonDestinationDenied))
	require.NotContains(t, recorder.Body.String(), "archive miss", "only the live failure is answered")
	require.Empty(t, recorder.Header().Get("X-Laisky-Cache-Status"))
}

// TestCacheHandlerStopsWhenClientLeaves preserves the client-disconnect answer:
// a canceled request is answered with 499 as soon as it ends, and both pending
// sources are released instead of being waited for in the background.
func TestCacheHandlerStopsWhenClientLeaves(t *testing.T) {
	var canceled atomic.Int32
	useProxySources(t, blockingLoader(&canceled), blockingLoader(&canceled), 0)

	ctx, cancel := context.WithCancel(context.Background())
	canceledAt := make(chan time.Time, 1)
	time.AfterFunc(50*time.Millisecond, func() {
		canceledAt <- time.Now()
		cancel()
	})
	recorder := serveCacheHandlerWithContext(ctx, "http://public.example/page")
	answeredAt := time.Now()
	require.Equal(t, 499, recorder.Code, "client-closed-request status")
	require.Less(t, answeredAt.Sub(<-canceledAt), time.Second, "the handler must stop when the client leaves")
	requireNoProxyGoroutines(t)
	require.EqualValues(t, 2, canceled.Load())
}

// TestSaveUrlContentSkipsURLArchivedDuringLookup closes the window between the
// archive lookup and taking the pending mark: a task that archived the url in
// between has already recorded it, so the url is not queued a second time.
func TestSaveUrlContentSkipsURLArchivedDuringLookup(t *testing.T) {
	const target = "https://public.example/archived-meanwhile"
	isUrlFinishedCache.Delete(target)
	t.Cleanup(func() { isUrlFinishedCache.Delete(target) })
	useArchiveSeams(t, func(context.Context, string) (bool, error) {
		// The lookup misses, then a concurrent task finishes and records the archive.
		isUrlFinishedCache.Store(target, true)
		return false, nil
	}, nil, nil)

	require.NoError(t, SaveUrlContent(context.Background(), taskItem{url: target}))
	require.Zero(t, drainArchiveTasks(), "an archived url must not be queued again")
	_, pending := runningUrls.Load(target)
	require.False(t, pending, "a skipped url must not stay marked pending")
}

// TestFirstAvailableContentRejectsMissingBody treats a loader that returns neither
// a body nor an error as a failure, so the caller never streams a nil body, and
// still answers from a source that does produce content.
func TestFirstAvailableContentRejectsMissingBody(t *testing.T) {
	logger := glog.Shared.Named("test")
	empty := func(context.Context, string) (io.ReadCloser, error) { return nil, nil }

	body, _, err := firstAvailableContent(context.Background(), logger, "http://public.example/", []contentSource{
		{name: "empty", status: "DYNAMIC", load: empty},
	})
	require.ErrorContains(t, err, "no body")
	require.Nil(t, body)

	bodies := newBodyRecorder(1)
	body, status, err := firstAvailableContent(context.Background(), logger, "http://public.example/", []contentSource{
		{name: "empty", status: "DYNAMIC", load: empty},
		{name: "archive", status: "CACHE", load: bodies.loader("cached-content")},
	})
	require.NoError(t, err)
	require.Equal(t, "CACHE", status)
	content, err := io.ReadAll(body)
	require.NoError(t, err)
	require.Equal(t, "cached-content", string(content))
	require.NoError(t, body.Close())
	bodies.requireAllClosed(t)

	_, _, err = firstAvailableContent(context.Background(), logger, "http://public.example/", nil)
	require.Error(t, err)
}
