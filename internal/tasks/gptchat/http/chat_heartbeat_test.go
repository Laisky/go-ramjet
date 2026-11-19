package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const (
	heartbeatFrame  = "data: [HEARTBEAT]\n\n"
	safariUserAgent = "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1"
	chromeUserAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0 Safari/537.36"
)

// heartbeatTestStream is a streaming gin request whose response writer is shared with
// the heartbeat goroutine under the gin-middlewares context lock.
type heartbeatTestStream struct {
	gctx   *gin.Context
	rec    *httptest.ResponseRecorder
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
	gctx.Set(string(gmw.CtxKeyLock), &sync.RWMutex{})

	reqCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	gctx.Request = httptest.NewRequest(http.MethodPost, "/gptchat/api", nil).WithContext(reqCtx)
	gctx.Request.Header.Set("User-Agent", userAgent)

	return &heartbeatTestStream{gctx: gctx, rec: rec, cancel: cancel}
}

// body returns the response written so far, read under the writer lock so it does
// not race with heartbeat writes.
func (s *heartbeatTestStream) body(t *testing.T) string {
	t.Helper()
	ctx := gmw.Ctx(s.gctx)
	require.NoError(t, gmw.CtxLock(ctx))
	defer func() { require.NoError(t, gmw.CtxUnlock(ctx)) }()
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
// several heartbeat intervals.
func (s *heartbeatTestStream) requireBodyStable(t *testing.T) string {
	t.Helper()
	before := s.body(t)
	time.Sleep(10 * streamHeartbeatInterval)
	require.Equal(t, before, s.body(t), "heartbeat written after the stream ended")
	return before
}

// TestEnableHeartBeatForStreamReqSendsPeriodicHeartbeats verifies that heartbeats keep
// flowing while the stream is open, for both regular and Safari clients.
func TestEnableHeartBeatForStreamReqSendsPeriodicHeartbeats(t *testing.T) {
	for _, ua := range []string{chromeUserAgent, safariUserAgent} {
		t.Run(ua[:20], func(t *testing.T) {
			s := newHeartbeatTestStream(t, ua)
			stop := enableHeartBeatForStreamReq(s.gctx)
			t.Cleanup(stop)

			require.Eventually(t, func() bool {
				return strings.Count(s.body(t), heartbeatFrame) >= 3
			}, 5*time.Second, 10*time.Millisecond, "periodic heartbeats were not sent while the stream was open")
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

	require.Equal(t, ": connection established\n"+heartbeatFrame, s.body(t))
}

// TestEnableHeartBeatForStreamReqStopEndsWrites verifies that once stop returns no
// heartbeat is written any more, and that stop is safe to call twice.
func TestEnableHeartBeatForStreamReqStopEndsWrites(t *testing.T) {
	s := newHeartbeatTestStream(t, chromeUserAgent)
	stop := enableHeartBeatForStreamReq(s.gctx)

	require.Eventually(t, func() bool {
		return strings.Contains(s.body(t), heartbeatFrame)
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

// TestWriteFinalToUIStopsHeartbeatBeforeDone verifies the streamed final answer ends
// with the [DONE] frame and no heartbeat is written after it.
func TestWriteFinalToUIStopsHeartbeatBeforeDone(t *testing.T) {
	s := newHeartbeatTestStream(t, safariUserAgent)

	err := writeFinalToUI(s.gctx, &FrontendReq{Stream: true}, nil, "final answer", "", nil)
	require.NoError(t, err)

	body := s.requireBodyStable(t)
	require.True(t, strings.HasSuffix(body, "data: [DONE]\n\n"), "stream must end with [DONE]")
	require.Contains(t, body, "final answer")
}
