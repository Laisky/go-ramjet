package localstorage

import (
	"context"
	"expvar"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	gutils "github.com/Laisky/go-utils/v6"
	"github.com/Laisky/zap/zapcore"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/library/log"
)

// scheduleCount returns how often the named scheduling outcome was counted.
func scheduleCount(outcome string) int64 {
	counter, ok := archiveScheduleOutcomes.Get(outcome).(*expvar.Int)
	if !ok {
		return 0
	}
	return counter.Value()
}

// TestCacheHandlerBoundsConcurrentArchiveLookups guards against an unauthenticated
// caller turning every proxy request into one more detached archive lookup: the
// lookups that may run at once are bounded, and requests beyond the bound are
// still answered.
func TestCacheHandlerBoundsConcurrentArchiveLookups(t *testing.T) {
	release := make(chan struct{})
	var releaseOnce sync.Once
	var inFlight, peak atomic.Int32
	bodies := newBodyRecorder(64)
	useProxySources(t, bodies.loader("live-content"), failingLoader(errors.New("archive miss")), 0)
	useArchiveSeams(t, func(ctx context.Context, _ string) (bool, error) {
		current := inFlight.Add(1)
		defer inFlight.Add(-1)
		for {
			observed := peak.Load()
			if current <= observed || peak.CompareAndSwap(observed, current) {
				break
			}
		}
		select {
		case <-release:
		case <-ctx.Done():
		}
		return false, nil
	}, nil, nil)
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })

	saturatedBefore := scheduleCount(scheduleSaturated)
	const requests = 4 * maxArchiveSchedulers
	for i := range requests {
		recorder := serveCacheHandler(fmt.Sprintf("http://public.example/page-%d", i))
		require.Equal(t, 200, recorder.Code, "request %d must be answered while lookups are pending", i)
	}
	// Every admitted scheduler blocks in its lookup, so exactly the bound is in flight.
	require.Eventually(t, func() bool { return inFlight.Load() == maxArchiveSchedulers }, 2*time.Second, time.Millisecond)
	require.LessOrEqual(t, int(peak.Load()), maxArchiveSchedulers, "concurrent archive lookups must be bounded")
	require.EqualValues(t, requests-maxArchiveSchedulers, scheduleCount(scheduleSaturated)-saturatedBefore,
		"every request beyond the bound is counted as skipped")
	releaseOnce.Do(func() { close(release) })
	requireNoProxyGoroutines(t)
	bodies.requireAllClosed(t)
}

// TestSaveUrlContentLooksUpEachURLOnce guards against redundant archive lookups:
// concurrent requests for one url must not each run their own lookup before one
// of them takes the pending mark. Only the first caller looks the url up; the
// others return at once.
func TestSaveUrlContentLooksUpEachURLOnce(t *testing.T) {
	release := make(chan struct{})
	var releaseOnce sync.Once
	var lookups, returned atomic.Int32
	useArchiveSeams(t, func(context.Context, string) (bool, error) {
		lookups.Add(1)
		<-release
		return false, nil
	}, nil, nil)
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })

	const callers = 32
	var wg sync.WaitGroup
	for range callers {
		wg.Go(func() {
			if err := SaveUrlContent(context.Background(), taskItem{url: "https://public.example/page"}); err != nil {
				t.Error(err)
			}
			returned.Add(1)
		})
	}
	require.Eventually(t, func() bool { return returned.Load() == callers-1 }, 2*time.Second, time.Millisecond,
		"callers other than the first must not wait on a lookup of the same url")
	releaseOnce.Do(func() { close(release) })
	wg.Wait()
	require.EqualValues(t, 1, lookups.Load(), "one url must be looked up once")
	require.Equal(t, 1, drainArchiveTasks())
}

// archiveLookupContext is what the archive lookup observed about its context.
type archiveLookupContext struct {
	deadline     time.Duration
	hasDeadline  bool
	ginReachable bool
	traceID      any
	err          error
}

// TestCacheHandlerDetachesArchiveScheduling requires the archive lookup to outlive
// the request without holding on to it: it is bounded by its own deadline, it is not
// canceled when the request ends, and the recycled gin context is not reachable.
func TestCacheHandlerDetachesArchiveScheduling(t *testing.T) {
	observed := make(chan archiveLookupContext, 1)
	bodies := newBodyRecorder(1)
	useProxySources(t, bodies.loader("live-content"), failingLoader(errors.New("archive miss")), 0)
	useArchiveSeams(t, func(ctx context.Context, _ string) (bool, error) {
		deadline, ok := ctx.Deadline()
		ginReachable := ctx.Value(gmw.CtxKeyGin) != nil
		observed <- archiveLookupContext{
			deadline: time.Until(deadline), hasDeadline: ok,
			ginReachable: ginReachable, traceID: ctx.Value(gutils.TracingKey), err: ctx.Err(),
		}
		return true, nil
	}, nil, nil)

	recorder := serveCacheHandler("http://public.example/page")
	require.Equal(t, 200, recorder.Code)
	lookup := <-observed
	require.True(t, lookup.hasDeadline, "archive scheduling must be bounded in time")
	require.LessOrEqual(t, lookup.deadline, archiveScheduleTimeout)
	require.False(t, lookup.ginReachable, "the detached scheduler must not retain the recycled gin context")
	require.NotNil(t, lookup.traceID, "the request's trace id is kept for correlation")
	require.NoError(t, lookup.err)
	requireNoProxyGoroutines(t)
	bodies.requireAllClosed(t)
}

// TestCacheHandlerAlertsArchiveLookupFailure makes an archive lookup failure
// visible: it is an infrastructure fault that silently stops archiving, so it is
// an ERROR on the archive logger with a safe, attributed alert.
func TestCacheHandlerAlertsArchiveLookupFailure(t *testing.T) {
	bodies := newBodyRecorder(1)
	useProxySources(t, bodies.loader("live-content"), failingLoader(errors.New("archive miss")), 0)
	entries, messages := installArchiveTestLogger(t)
	runningUrls.Clear()
	old := isArchived
	isArchived = func(context.Context, string) (bool, error) {
		return false, errors.WithStack(&net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")})
	}
	t.Cleanup(func() { isArchived = old })
	raw := "http://public.example/private-path?token=private-query"
	failedBefore := scheduleCount(scheduleLookupFailed)

	recorder := serveCacheHandler(raw)
	require.Equal(t, 200, recorder.Code)
	var message string
	select {
	case message = <-messages:
	case <-time.After(3 * time.Second):
		t.Fatal("an archive lookup failure must alert")
	}
	require.Contains(t, message, `"stage":"schedule"`)
	require.Contains(t, message, `"category":"network_error"`)
	require.Contains(t, message, `"target_ref":"`+log.ArchiveTargetRef(raw)+`"`)
	for _, secret := range []string{"public.example", "private-path", "private-query", "connection refused"} {
		require.NotContains(t, message, secret)
	}
	requireNoProxyGoroutines(t)
	logged := entries.FilterLoggerName(log.ArchiveLoggerName).All()
	require.Len(t, logged, 1)
	require.Equal(t, zapcore.ErrorLevel, logged[0].Level)
	require.Equal(t, raw, logged[0].ContextMap()["url"], "local logs keep the raw target for operators")
	require.Equal(t, log.ArchiveTargetRef(raw), logged[0].ContextMap()["target_ref"])
	require.EqualValues(t, 1, scheduleCount(scheduleLookupFailed)-failedBefore)
	_, pending := runningUrls.Load(raw)
	require.False(t, pending, "a url whose lookup failed must not stay pending")
	bodies.requireAllClosed(t)
}

// TestCacheHandlerReportsFullArchiveQueue keeps a full queue below the alert level,
// because bursts fill it by design, while recording it on the archive logger
// with its category so a persistently full queue is visible.
func TestCacheHandlerReportsFullArchiveQueue(t *testing.T) {
	bodies := newBodyRecorder(1)
	useProxySources(t, bodies.loader("live-content"), failingLoader(errors.New("archive miss")), 0)
	entries, messages := installArchiveTestLogger(t)
	runningUrls.Clear()
	old := isArchived
	isArchived = notArchived
	t.Cleanup(func() { isArchived = old })
	drainArchiveTasks()
	for i := range cap(taskChan) {
		taskChan <- taskItem{url: fmt.Sprintf("https://public.example/filler-%d", i)}
	}

	fullBefore := scheduleCount(scheduleQueueFull)
	recorder := serveCacheHandler("http://public.example/page")
	require.Equal(t, 200, recorder.Code)
	requireNoProxyGoroutines(t)
	require.EqualValues(t, 1, scheduleCount(scheduleQueueFull)-fullBefore)
	select {
	case message := <-messages:
		t.Fatalf("a full queue must not alert: %s", message)
	case <-time.After(300 * time.Millisecond):
	}
	logged := entries.FilterLoggerName(log.ArchiveLoggerName).All()
	require.Len(t, logged, 1)
	require.Equal(t, zapcore.WarnLevel, logged[0].Level)
	require.Equal(t, "queue_full", logged[0].ContextMap()["category"])
	_, pending := runningUrls.Load("http://public.example/page")
	require.False(t, pending, "a url that could not be queued must not stay pending")
	drainArchiveTasks()
	bodies.requireAllClosed(t)
}

// TestCacheHandlerEndsHungArchiveLookup proves the scheduling deadline is enforced:
// a lookup that would otherwise hang (a stalled S3 HEAD) is ended by
// archiveScheduleTimeout, which releases its slot and pending mark and records an
// attributed ERROR with the timeout category, so the scheduler cannot pile up.
func TestCacheHandlerEndsHungArchiveLookup(t *testing.T) {
	bodies := newBodyRecorder(1)
	useProxySources(t, bodies.loader("live-content"), failingLoader(errors.New("archive miss")), 0)
	entries := useArchiveSeams(t, func(ctx context.Context, _ string) (bool, error) {
		<-ctx.Done()
		return false, errors.WithStack(ctx.Err())
	}, nil, nil)
	oldTimeout := archiveScheduleTimeout
	archiveScheduleTimeout = 50 * time.Millisecond
	t.Cleanup(func() { archiveScheduleTimeout = oldTimeout })
	const raw = "http://public.example/stalled"

	startedAt := time.Now()
	recorder := serveCacheHandler(raw)
	require.Equal(t, 200, recorder.Code)
	requireNoProxyGoroutines(t)
	require.Less(t, time.Since(startedAt), 2*time.Second, "the lookup must end at its deadline")
	require.Empty(t, archiveSchedulerSlots, "the scheduler slot must be released")
	_, pending := runningUrls.Load(raw)
	require.False(t, pending, "a url whose lookup timed out must not stay pending")
	logged := entries.FilterLoggerName(log.ArchiveLoggerName).All()
	require.Len(t, logged, 1)
	require.Equal(t, zapcore.ErrorLevel, logged[0].Level)
	require.Equal(t, "schedule", logged[0].ContextMap()["stage"])
	var cause error
	for _, field := range logged[0].Context {
		if field.Key == "error" {
			cause, _ = field.Interface.(error)
		}
	}
	require.ErrorIs(t, cause, context.DeadlineExceeded, "the failure is attributed to the deadline")
	bodies.requireAllClosed(t)
}
