package http

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const (
	periodicHeartbeatFrame = ": heartbeat\n\n"
	legacyHeartbeatFrame   = "data: [HEARTBEAT]\n\n"
	safariUserAgent        = "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1"
	chromeUserAgent        = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0 Safari/537.36"
)

// heartbeatTestStream is a streaming gin request whose response writer is shared with
// the heartbeat goroutine under the gin-middlewares context lock.
type heartbeatTestStream struct {
	gctx   *gin.Context
	rec    *httptest.ResponseRecorder
	lock   *sync.RWMutex
	cancel context.CancelFunc
}

// newHeartbeatTestStream builds a gin context for a streaming request with the given
// user agent, installs the writer lock, and shortens heartbeat intervals.
func newHeartbeatTestStream(t *testing.T, userAgent string) *heartbeatTestStream {
	t.Helper()
	origInterval, origSafari := streamHeartbeatInterval, safariStreamHeartbeatInterval
	t.Cleanup(func() { streamHeartbeatInterval, safariStreamHeartbeatInterval = origInterval, origSafari })
	streamHeartbeatInterval = 20 * time.Millisecond
	safariStreamHeartbeatInterval = 20 * time.Millisecond

	rec := httptest.NewRecorder()
	gctx, _ := gin.CreateTestContext(rec)
	lock := &sync.RWMutex{}
	gctx.Set(string(gmw.CtxKeyLock), lock)

	reqCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	gctx.Request = httptest.NewRequest(http.MethodPost, "/gptchat/api", nil).WithContext(reqCtx)
	gctx.Request.Header.Set("User-Agent", userAgent)

	return &heartbeatTestStream{gctx: gctx, rec: rec, lock: lock, cancel: cancel}
}

// body returns the response written so far, read under the writer lock so it does
// not race with heartbeat writes.
func (s *heartbeatTestStream) body() string {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.rec.Body.String()
}

// requireReturnsWithin fails the test when fn does not return within limit.
func requireReturnsWithin(t *testing.T, limit time.Duration, what string, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()

	select {
	case <-done:
	case <-time.After(limit):
		require.FailNowf(t, "blocked", "%s did not return within %s", what, limit)
	}
}

// requireBodyStable verifies that nothing more is written to the response over
// several heartbeat intervals and returns the final body.
func (s *heartbeatTestStream) requireBodyStable(t *testing.T) string {
	t.Helper()
	before := s.body()
	time.Sleep(10 * streamHeartbeatInterval)
	require.Equal(t, before, s.body(), "heartbeat written after the stream ended")
	return before
}

// liveGoroutines counts the live goroutines whose stack contains frame, such as
// "enableHeartBeatForStreamReq.func"; it is a dependency-free goleak-style probe.
func liveGoroutines(frame string) int {
	buf := make([]byte, 1<<20)
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			return strings.Count(string(buf[:n]), frame)
		}
		buf = make([]byte, 2*len(buf))
	}
}

// heartbeatGoroutines counts live goroutines started by enableHeartBeatForStreamReq.
func heartbeatGoroutines() int {
	return liveGoroutines("enableHeartBeatForStreamReq.func")
}

// TestEnableHeartBeatForStreamReqGoroutineExitsWithRequest verifies that the
// heartbeat goroutine does not leak when the request ends but the handler never
// calls stop, for example because it is still blocked elsewhere.
func TestEnableHeartBeatForStreamReqGoroutineExitsWithRequest(t *testing.T) {
	before := heartbeatGoroutines()
	s := newHeartbeatTestStream(t, chromeUserAgent)
	stop := enableHeartBeatForStreamReq(s.gctx)
	t.Cleanup(stop)
	require.Greater(t, heartbeatGoroutines(), before, "the heartbeat goroutine never started")

	s.cancel()
	require.Eventually(t, func() bool { return heartbeatGoroutines() <= before },
		5*time.Second, 10*time.Millisecond, "the heartbeat goroutine outlived its request")
}

// TestEnableHeartBeatForStreamReqSendsPeriodicHeartbeats verifies that heartbeats keep
// flowing while the stream is open, as SSE comment frames that spec-compliant
// clients ignore; only Safari's initial frame keeps the legacy data form.
func TestEnableHeartBeatForStreamReqSendsPeriodicHeartbeats(t *testing.T) {
	for _, tc := range []struct {
		name        string
		ua          string
		legacyCount int
	}{
		{name: "chrome", ua: chromeUserAgent, legacyCount: 0},
		{name: "safari", ua: safariUserAgent, legacyCount: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newHeartbeatTestStream(t, tc.ua)
			stop := enableHeartBeatForStreamReq(s.gctx)
			t.Cleanup(stop)

			require.Eventually(t, func() bool {
				return strings.Count(s.body(), periodicHeartbeatFrame) >= 3
			}, 5*time.Second, 10*time.Millisecond, "periodic heartbeats were not sent while the stream was open")
			stop()
			require.Equal(t, tc.legacyCount, strings.Count(s.body(), legacyHeartbeatFrame),
				"periodic heartbeats must not be data frames")
		})
	}
}

// TestEnableHeartBeatForStreamReqSafariInitialFrame verifies Safari receives the
// connection-established heartbeat before any periodic one.
func TestEnableHeartBeatForStreamReqSafariInitialFrame(t *testing.T) {
	s := newHeartbeatTestStream(t, safariUserAgent)
	safariStreamHeartbeatInterval = time.Hour
	stop := enableHeartBeatForStreamReq(s.gctx)
	t.Cleanup(stop)

	require.Equal(t, ": connection established\n"+legacyHeartbeatFrame, s.body())
}

// TestEnableHeartBeatForStreamReqStopEndsWrites verifies that once stop returns no
// heartbeat is written any more, and that stop is safe to call twice.
func TestEnableHeartBeatForStreamReqStopEndsWrites(t *testing.T) {
	s := newHeartbeatTestStream(t, chromeUserAgent)
	stop := enableHeartBeatForStreamReq(s.gctx)

	require.Eventually(t, func() bool {
		return strings.Contains(s.body(), periodicHeartbeatFrame)
	}, 5*time.Second, 10*time.Millisecond)

	requireReturnsWithin(t, 5*time.Second, "stop", stop)
	s.requireBodyStable(t)
	requireReturnsWithin(t, 5*time.Second, "second stop", stop)
}

// TestEnableHeartBeatForStreamReqEndsWithRequest verifies the heartbeat goroutine
// exits on its own when the request context ends.
func TestEnableHeartBeatForStreamReqEndsWithRequest(t *testing.T) {
	s := newHeartbeatTestStream(t, chromeUserAgent)
	stop := enableHeartBeatForStreamReq(s.gctx)

	s.cancel()
	// stop only waits here; it returns once the goroutine has observed the cancellation.
	requireReturnsWithin(t, 5*time.Second, "stop after request cancellation", stop)
	s.requireBodyStable(t)
}

// TestEnableHeartBeatForStreamReqStopSurvivesPanickedWriter verifies that stop cannot
// hang when the handler panicked while holding the writer lock, so gin's recovery
// still runs: the deferred stop must return although the lock is never released.
func TestEnableHeartBeatForStreamReqStopSurvivesPanickedWriter(t *testing.T) {
	s := newHeartbeatTestStream(t, chromeUserAgent)

	handler := func() {
		stop := enableHeartBeatForStreamReq(s.gctx)
		defer stop()

		require.NoError(t, gmw.CtxLock(gmw.Ctx(s.gctx)))
		time.Sleep(5 * streamHeartbeatInterval) // let heartbeat ticks contend for the lock
		panic("stream writer failed while holding the lock")
	}

	var recovered any
	requireReturnsWithin(t, 5*time.Second, "panicking handler with deferred stop", func() {
		defer func() { recovered = recover() }()
		handler()
	})
	require.NotNil(t, recovered)
}

// TestEnableHeartBeatForStreamReqWithoutLock verifies that a context without the
// writer lock disables heartbeats instead of writing unsynchronized frames.
func TestEnableHeartBeatForStreamReqWithoutLock(t *testing.T) {
	s := newHeartbeatTestStream(t, safariUserAgent)
	s.gctx.Keys = nil

	stop := enableHeartBeatForStreamReq(s.gctx)
	time.Sleep(10 * safariStreamHeartbeatInterval)
	requireReturnsWithin(t, 5*time.Second, "stop", stop)
	require.Empty(t, s.rec.Body.String())
}

// TestWriteFinalToUILeavesNoHeartbeatRunning verifies the streamed final answer ends
// with [DONE] and that no heartbeat goroutine outlives writeFinalToUI while the
// request is still open, so nothing can be written after [DONE] or after the
// handler returns.
func TestWriteFinalToUILeavesNoHeartbeatRunning(t *testing.T) {
	s := newHeartbeatTestStream(t, safariUserAgent)
	require.Eventually(t, func() bool { return heartbeatGoroutines() == 0 },
		5*time.Second, 10*time.Millisecond, "heartbeat goroutines leaked by an earlier test")

	err := writeFinalToUI(s.gctx, &FrontendReq{Stream: true}, nil, "final answer", "", nil)
	require.NoError(t, err)
	require.Zero(t, heartbeatGoroutines(), "heartbeat goroutine still running after writeFinalToUI returned")

	body := s.requireBodyStable(t)
	require.True(t, strings.HasSuffix(body, "data: [DONE]\n\n"), "stream must end with [DONE]")
	require.Contains(t, body, "final answer")
}

// slowFlushRecorder is a response writer whose every write and flush takes a while,
// widening the window in which a heartbeat could interleave with a stream chunk.
type slowFlushRecorder struct {
	*httptest.ResponseRecorder
	delay time.Duration
}

// Write records p after the configured delay.
func (w *slowFlushRecorder) Write(p []byte) (int, error) {
	time.Sleep(w.delay)
	return w.ResponseRecorder.Write(p)
}

// WriteString records s after the configured delay.
func (w *slowFlushRecorder) WriteString(s string) (int, error) {
	time.Sleep(w.delay)
	return w.ResponseRecorder.WriteString(s)
}

// Flush flushes the recorder after the configured delay.
func (w *slowFlushRecorder) Flush() {
	time.Sleep(w.delay)
	w.ResponseRecorder.Flush()
}

// TestHeartbeatNeverInterleavesWithStreamChunks verifies that heartbeats contending
// with stream chunks on a slow connection never split a frame: every SSE frame on
// the wire is either a whole heartbeat comment or a whole JSON data frame, and the
// stream ends with [DONE]. Run with -race it also proves the heartbeat goroutine and
// the handler never touch the response writer concurrently.
func TestHeartbeatNeverInterleavesWithStreamChunks(t *testing.T) {
	origInterval := streamHeartbeatInterval
	t.Cleanup(func() { streamHeartbeatInterval = origInterval })
	streamHeartbeatInterval = time.Millisecond

	rec := &slowFlushRecorder{ResponseRecorder: httptest.NewRecorder(), delay: 100 * time.Microsecond}
	gctx, _ := gin.CreateTestContext(rec)
	gctx.Set(string(gmw.CtxKeyLock), &sync.RWMutex{})
	gctx.Request = httptest.NewRequest(http.MethodPost, "/gptchat/api", nil)
	gctx.Request.Header.Set("User-Agent", chromeUserAgent)

	stop := enableHeartBeatForStreamReq(gctx)
	t.Cleanup(stop)
	for i := range 50 {
		require.NoError(t, writeChatCompletionChunk(gctx, OpenaiCompletionStreamResp{
			ID: "req-1",
			Choices: []OpenaiCompletionStreamRespChoice{{
				Delta: OpenaiCompletionStreamRespDelta{Role: OpenaiMessageRoleAI, Content: strings.Repeat("x", i%7)},
			}},
		}))
		time.Sleep(100 * time.Microsecond) // idle gap between chunks, as between upstream deltas
	}
	stop()
	_, err := io.WriteString(gctx.Writer, "data: [DONE]\n\n")
	require.NoError(t, err)

	body := rec.Body.String()
	require.True(t, strings.HasSuffix(body, "\n\n"))
	frames := strings.Split(strings.TrimSuffix(body, "\n\n"), "\n\n")
	var heartbeats, chunks int
	for _, frame := range frames {
		switch {
		case frame == ": heartbeat":
			heartbeats++
		case frame == "data: [DONE]":
		case strings.HasPrefix(frame, "data: "):
			var chunk OpenaiCompletionStreamResp
			require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(frame, "data: ")), &chunk), "corrupted frame %q", frame)
			chunks++
		default:
			require.FailNowf(t, "corrupted frame", "%q", frame)
		}
	}
	require.Equal(t, 50, chunks)
	require.Positive(t, heartbeats, "no heartbeat was written between chunks")
	require.Equal(t, "data: [DONE]", frames[len(frames)-1])
}
